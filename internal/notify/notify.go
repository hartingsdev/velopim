// Package notify sends the daily mail about open tasks.
package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/db"
)

// Config holds the mail settings. All values come from the environment.
type Config struct {
	Host     string // SMTP server; empty: no mail is sent
	Port     int    // 587 (STARTTLS) or 465 (TLS from the start)
	User     string // empty: no login
	Password string
	From     string
	To       []string
	// BaseURL is where velopim is reachable, for the link in the mail.
	BaseURL string
	// Hour and Minute say when the daily mail is sent, in local time.
	Hour, Minute int
}

// FromEnv reads the configuration. ok is false when no mail server is
// configured; then nothing is sent and err is nil.
func FromEnv(getenv func(string) string) (cfg Config, ok bool, err error) {
	cfg.Host = strings.TrimSpace(getenv("VELOPIM_SMTP_HOST"))
	if cfg.Host == "" {
		return cfg, false, nil
	}
	cfg.Port = 587
	if v := getenv("VELOPIM_SMTP_PORT"); v != "" {
		if cfg.Port, err = strconv.Atoi(v); err != nil || cfg.Port < 1 || cfg.Port > 65535 {
			return cfg, false, fmt.Errorf("notify: VELOPIM_SMTP_PORT %q is not a port", v)
		}
	}
	cfg.User, cfg.Password = getenv("VELOPIM_SMTP_USER"), getenv("VELOPIM_SMTP_PASSWORD")
	cfg.From = strings.TrimSpace(getenv("VELOPIM_MAIL_FROM"))
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return cfg, false, fmt.Errorf("notify: VELOPIM_MAIL_FROM must be a mail address")
	}
	for _, to := range strings.Split(getenv("VELOPIM_MAIL_TO"), ",") {
		if to = strings.TrimSpace(to); to == "" {
			continue
		}
		if _, err := mail.ParseAddress(to); err != nil {
			return cfg, false, fmt.Errorf("notify: VELOPIM_MAIL_TO contains %q, which is not a mail address", to)
		}
		cfg.To = append(cfg.To, to)
	}
	if len(cfg.To) == 0 {
		return cfg, false, errors.New("notify: VELOPIM_MAIL_TO is required when VELOPIM_SMTP_HOST is set")
	}
	cfg.BaseURL = strings.TrimRight(getenv("VELOPIM_BASE_URL"), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}
	cfg.Hour, cfg.Minute = 7, 0
	if v := getenv("VELOPIM_MAIL_TIME"); v != "" {
		t, err := time.Parse("15:04", v)
		if err != nil {
			return cfg, false, fmt.Errorf("notify: VELOPIM_MAIL_TIME %q must look like 07:00", v)
		}
		cfg.Hour, cfg.Minute = t.Hour(), t.Minute()
	}
	return cfg, true, nil
}

// Next returns the next time the daily mail is due after now. It implements
// the schedule of the periodic job.
func (c Config) Next(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), c.Hour, c.Minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// Counts is what the mail reports.
type Counts struct {
	Open int64 // open tasks that are not put aside
	High int64 // of those, with high priority
	// Resubmitted counts tasks whose Wiedervorlage date has come.
	Resubmitted int64
	// ByCategory holds the open tasks per category label, in display order.
	ByCategory []CategoryCount
}

// CategoryCount is the number of open tasks of one category.
type CategoryCount struct {
	Label string
	Count int64
}

var categoryLabels = []struct{ key, label string }{
	{"price", "Preise"}, {"new_model", "Neue Modelle"}, {"availability", "Verfügbarkeit"},
	{"removed", "Entfallene Artikel"}, {"content", "Texte, Bilder, Merkmale"}, {"import", "Import"},
	{"shopify", "Shopify"}, {"other", "Sonstiges"},
}

// Load reads the counts.
func Load(ctx context.Context, pool *pgxpool.Pool) (Counts, error) {
	q := db.New(pool)
	row, err := q.DigestCounts(ctx)
	if err != nil {
		return Counts{}, err
	}
	c := Counts{Open: row.Open, High: row.High, Resubmitted: row.Resubmitted}
	byCategory, err := q.CountTasksByCategory(ctx, db.CountTasksByCategoryParams{View: "open", Q: ""})
	if err != nil {
		return Counts{}, err
	}
	counts := map[string]int64{}
	for _, r := range byCategory {
		counts[r.Category] = r.Count
	}
	for _, cl := range categoryLabels {
		if n := counts[cl.key]; n > 0 {
			c.ByCategory = append(c.ByCategory, CategoryCount{Label: cl.label, Count: n})
		}
	}
	return c, nil
}

// Digest builds the mail. ok is false when there is nothing to report: no
// mail is sent on a day without open tasks.
func Digest(c Counts, baseURL string) (subject, body string, ok bool) {
	if c.Open == 0 {
		return "", "", false
	}
	subject = fmt.Sprintf("velopim: %d offene Aufgaben", c.Open)
	if c.Open == 1 {
		subject = "velopim: 1 offene Aufgabe"
	}
	var b strings.Builder
	if c.Open == 1 {
		b.WriteString("In velopim wartet 1 Aufgabe auf dich.\n")
	} else {
		fmt.Fprintf(&b, "In velopim warten %d Aufgaben auf dich.\n", c.Open)
	}
	if c.High > 0 {
		fmt.Fprintf(&b, "Davon mit hoher Priorität: %d\n", c.High)
	}
	if c.Resubmitted > 0 {
		fmt.Fprintf(&b, "Davon aus der Wiedervorlage zurück: %d\n", c.Resubmitted)
	}
	b.WriteString("\n")
	for _, cc := range c.ByCategory {
		fmt.Fprintf(&b, "%4d  %s\n", cc.Count, cc.Label)
	}
	fmt.Fprintf(&b, "\nZu den Aufgaben: %s/#tasks\n", baseURL)
	return subject, b.String(), true
}

// Message renders a plain-text mail.
func Message(cfg Config, subject, body string, now time.Time) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", cfg.From)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(cfg.To, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", now.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return []byte(b.String())
}

// Send delivers a mail. Port 465 uses TLS from the start; on any other port
// STARTTLS is used when the server offers it. A password is only ever sent
// over an encrypted connection (or to localhost).
func Send(ctx context.Context, cfg Config, subject, body string) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	var conn net.Conn
	var err error
	if cfg.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: cfg.Host}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("notify: connecting to the mail server: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("notify: %w", err)
	}
	defer client.Close()

	if cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
				return fmt.Errorf("notify: starting TLS: %w", err)
			}
		}
	}
	if cfg.User != "" {
		// PlainAuth refuses to send the password over an unencrypted
		// connection unless the server is localhost.
		if err := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("notify: login at the mail server: %w", err)
		}
	}
	from, _ := mail.ParseAddress(cfg.From)
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	for _, to := range cfg.To {
		addr, _ := mail.ParseAddress(to)
		if err := client.Rcpt(addr.Address); err != nil {
			return fmt.Errorf("notify: recipient %s: %w", addr.Address, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	if _, err := w.Write(Message(cfg, subject, body, time.Now())); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return client.Quit()
}

// SendDigest loads the counts and sends the mail. sent is false when there
// was nothing to report.
func SendDigest(ctx context.Context, pool *pgxpool.Pool, cfg Config) (sent bool, err error) {
	counts, err := Load(ctx, pool)
	if err != nil {
		return false, err
	}
	subject, body, ok := Digest(counts, cfg.BaseURL)
	if !ok {
		return false, nil
	}
	return true, Send(ctx, cfg, subject, body)
}
