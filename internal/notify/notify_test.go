package notify

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestFromEnv(t *testing.T) {
	base := map[string]string{"VELOPIM_SMTP_HOST": "mail.example.test", "VELOPIM_MAIL_FROM": "velopim@example.test",
		"VELOPIM_MAIL_TO": "a@example.test, Einkauf <b@example.test>"}
	with := func(change map[string]string) func(string) string {
		env := map[string]string{}
		for k, v := range base {
			env[k] = v
		}
		for k, v := range change {
			env[k] = v
		}
		return func(k string) string { return env[k] }
	}

	cfg, ok, err := FromEnv(with(nil))
	if err != nil || !ok || cfg.Port != 587 || len(cfg.To) != 2 || cfg.Hour != 7 || cfg.Minute != 0 || cfg.BaseURL != "http://localhost:8080" {
		t.Fatalf("defaults: %+v ok=%v err=%v", cfg, ok, err)
	}
	cfg, _, err = FromEnv(with(map[string]string{"VELOPIM_SMTP_PORT": "465", "VELOPIM_MAIL_TIME": "18:30", "VELOPIM_BASE_URL": "https://pim.example.test/"}))
	if err != nil || cfg.Port != 465 || cfg.Hour != 18 || cfg.Minute != 30 || cfg.BaseURL != "https://pim.example.test" {
		t.Errorf("custom: %+v %v", cfg, err)
	}
	// No server configured: no mail, no error.
	if _, ok, err := FromEnv(func(string) string { return "" }); ok || err != nil {
		t.Errorf("unconfigured: ok=%v err=%v", ok, err)
	}
	for name, change := range map[string]map[string]string{
		"bad port":      {"VELOPIM_SMTP_PORT": "mail"},
		"no sender":     {"VELOPIM_MAIL_FROM": ""},
		"no recipient":  {"VELOPIM_MAIL_TO": " , "},
		"bad recipient": {"VELOPIM_MAIL_TO": "a@example.test, nobody"},
		"bad time":      {"VELOPIM_MAIL_TIME": "7 Uhr"},
	} {
		if _, ok, err := FromEnv(with(change)); err == nil || ok {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNext(t *testing.T) {
	cfg := Config{Hour: 7, Minute: 0}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for now, want := range map[string]string{
		"2027-01-10 06:59": "2027-01-10 07:00",
		"2027-01-10 07:00": "2027-01-11 07:00", // exactly due: the next one is tomorrow
		"2027-01-10 12:00": "2027-01-11 07:00",
		"2027-12-31 23:59": "2028-01-01 07:00",
	} {
		if got := cfg.Next(at(now)); !got.Equal(at(want)) {
			t.Errorf("Next(%s) = %s, want %s", now, got.Format("2006-01-02 15:04"), want)
		}
	}
}

func TestDigest(t *testing.T) {
	if _, _, ok := Digest(Counts{}, "http://x"); ok {
		t.Error("no open tasks must not produce a mail")
	}
	subject, body, ok := Digest(Counts{Open: 1, ByCategory: []CategoryCount{{"Preise", 1}}}, "http://x")
	if !ok || subject != "velopim: 1 offene Aufgabe" || !strings.Contains(body, "wartet 1 Aufgabe") || strings.Contains(body, "hoher Priorität") {
		t.Errorf("one task: %q\n%s", subject, body)
	}
	subject, body, ok = Digest(Counts{Open: 12, High: 3, Resubmitted: 2,
		ByCategory: []CategoryCount{{"Preise", 7}, {"Neue Modelle", 5}}}, "https://pim.example.test")
	want := "In velopim warten 12 Aufgaben auf dich.\nDavon mit hoher Priorität: 3\nDavon aus der Wiedervorlage zurück: 2\n\n" +
		"   7  Preise\n   5  Neue Modelle\n\nZu den Aufgaben: https://pim.example.test/#tasks\n"
	if !ok || subject != "velopim: 12 offene Aufgaben" || body != want {
		t.Errorf("subject %q, body:\n%s", subject, body)
	}
}

// fakeSMTP accepts one mail without TLS or login and returns what it got.
func fakeSMTP(t *testing.T) (host string, port int, received chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	received = make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r, w := bufio.NewReader(conn), conn
		say := func(s string) { _, _ = w.Write([]byte(s + "\r\n")) }
		say("220 fake ESMTP")
		var log strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			log.WriteString(line)
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case inData:
				if strings.TrimSpace(line) == "." {
					inData = false
					say("250 queued")
				}
			case strings.HasPrefix(cmd, "EHLO"):
				say("250-fake")
				say("250 8BITMIME")
			case strings.HasPrefix(cmd, "DATA"):
				inData = true
				say("354 go ahead")
			case strings.HasPrefix(cmd, "QUIT"):
				say("221 bye")
				received <- log.String()
				return
			default:
				say("250 ok")
			}
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, received
}

func TestSend(t *testing.T) {
	host, port, received := fakeSMTP(t)
	cfg := Config{Host: host, Port: port, From: "velopim <velopim@example.test>", To: []string{"a@example.test", "Einkauf <b@example.test>"}}
	if err := Send(context.Background(), cfg, "velopim: 2 offene Aufgaben", "Zeile mit Umlaut: Verfügbarkeit\nZweite Zeile\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-received:
		for _, want := range []string{
			"MAIL FROM:<velopim@example.test>", "RCPT TO:<a@example.test>", "RCPT TO:<b@example.test>",
			"Subject: velopim: 2 offene Aufgaben", "Content-Type: text/plain; charset=utf-8",
			"Zeile mit Umlaut: Verfügbarkeit\r\nZweite Zeile\r\n",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("mail lacks %q:\n%s", want, got)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the fake server received nothing")
	}

	// A password is never sent over an unencrypted connection to a remote
	// host; nothing listens here, so it fails either way, but with a login
	// configured against a plain remote server Send must not succeed.
	if err := Send(context.Background(), Config{Host: "127.0.0.1", Port: 1, From: "a@example.test", To: []string{"b@example.test"}}, "x", "y"); err == nil {
		t.Error("sending to a closed port succeeded")
	}
}
