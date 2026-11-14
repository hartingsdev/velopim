// Command velopim runs the PIM server and its maintenance commands.
//
//	velopim serve                    start the web server (default)
//	velopim import <source> <file>   import one supplier delivery
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/api"
	"velopim/internal/connector"
	"velopim/internal/connector/csvfile"
	"velopim/internal/db"
	"velopim/internal/importer"
	"velopim/web"
)

func main() {
	log.SetFlags(0)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(ctx)
	case "import":
		if len(args) != 2 {
			err = errors.New("usage: velopim import <source> <file>")
			break
		}
		err = runImport(ctx, args[0], args[1])
	default:
		err = fmt.Errorf("unknown command %q (serve, import)", cmd)
	}
	if err != nil {
		log.Fatalf("velopim: %v", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openDB(ctx context.Context) (*pgxpool.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, errors.New("DATABASE_URL is not set (see .env.example)")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: %w", err)
	}
	return pool, nil
}

func serve(ctx context.Context) error {
	pool, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	mux := http.NewServeMux()
	mux.Handle("/api/", api.New(pool, env("VELOPIM_ACTOR", "betreiber")))
	mux.Handle("/", web.Handler())

	// There is no login: keep the default on localhost.
	addr := env("VELOPIM_ADDR", "127.0.0.1:8080")
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("velopim listening on http://%s", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runImport(ctx context.Context, sourceID, path string) error {
	pool, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	conn, err := loadConnector(ctx, pool, sourceID)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	res, err := importer.Run(ctx, pool, conn, f, importer.Options{
		RawDir: filepath.Join(env("VELOPIM_DATA_DIR", "data"), "raw")})
	if err != nil {
		return err
	}
	fmt.Printf("Importlauf %d (%s): %d Artikel, %d neu, %d geändert, %d entfallen, %d Mengenänderungen\n",
		res.RunID, res.Status, res.Records, res.New, res.Changed, res.Removed, res.QuantityChanges)
	fmt.Printf("%d automatisch übernommen, %d zur Freigabe, %d Datenprobleme\n",
		res.AutoApplied, res.PendingApproval, res.Issues)
	return nil
}

// loadConnector builds the connector a source is configured with.
func loadConnector(ctx context.Context, pool *pgxpool.Pool, sourceID string) (connector.Connector, error) {
	src, err := db.New(pool).GetSource(ctx, sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("source %q is not configured", sourceID)
	}
	if err != nil {
		return nil, err
	}
	switch src.ConnectorKind {
	case "csvfile":
		var cfg csvfile.Config
		if err := json.Unmarshal(src.ConnectorConfig, &cfg); err != nil {
			return nil, fmt.Errorf("connector config of %q: %w", sourceID, err)
		}
		cfg.SourceID = src.ID
		return csvfile.New(cfg)
	}
	return nil, fmt.Errorf("source %q: unknown connector kind %q", sourceID, src.ConnectorKind)
}
