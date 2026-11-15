// Command velopim runs the PIM server and its maintenance commands.
//
//	velopim serve                    start the web server (default)
//	velopim import <source> <file>   import one supplier delivery
//	velopim sync                     send pending changes to Shopify
//	velopim shopify-check            test the Shopify connection (read only)
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/api"
	"velopim/internal/db"
	"velopim/internal/importer"
	"velopim/internal/shopify"
	"velopim/internal/shopsync"
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
	case "shopify-check":
		err = shopifyCheck(ctx)
	case "sync":
		err = runSync(ctx)
	default:
		err = fmt.Errorf("unknown command %q (serve, import, sync, shopify-check)", cmd)
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

// periodicSync runs the Shopify sync at a fixed interval until ctx ends.
// Failures of single products become tasks; anything else is logged and
// tried again at the next tick.
func periodicSync(ctx context.Context, pool *pgxpool.Pool, store shopsync.Store, interval time.Duration) {
	log.Printf("periodic Shopify sync every %s", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			res, err := shopsync.Run(ctx, pool, store, time.Now())
			if err != nil {
				log.Printf("periodic sync: %v", err)
				continue
			}
			if res.Synced > 0 || res.Failed > 0 {
				log.Printf("periodic sync: %d synced, %d failed", res.Synced, res.Failed)
			}
		}
	}
}

// rawDir is where original supplier files are kept.
func rawDir() string {
	return filepath.Join(env("VELOPIM_DATA_DIR", "data"), "raw")
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
	// Without Shopify settings the server runs without a store; the sync
	// preview still works.
	var store shopsync.Store
	if cfg := shopifyConfig(); cfg.StoreDomain != "" {
		client, err := shopify.NewClient(cfg)
		if err != nil {
			return err
		}
		store = client
	}
	mux.Handle("/api/", api.New(pool, env("VELOPIM_ACTOR", "betreiber"), store, api.WithRawDir(rawDir())))

	// Optional: send pending changes to Shopify at a fixed interval, e.g.
	// VELOPIM_SYNC_INTERVAL=15m. Off by default; it also needs writing to be
	// enabled.
	if v := os.Getenv("VELOPIM_SYNC_INTERVAL"); v != "" && store != nil {
		interval, err := time.ParseDuration(v)
		if err != nil || interval < time.Minute {
			return fmt.Errorf("VELOPIM_SYNC_INTERVAL %q must be a duration of at least 1m, e.g. 15m", v)
		}
		if !shopifyConfig().WriteEnabled {
			log.Printf("periodic sync is configured but writing to Shopify is switched off; not started")
		} else {
			go periodicSync(ctx, pool, store, interval)
		}
	}
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

// shopifyConfig reads the Shopify settings from the environment.
func shopifyConfig() shopify.Config {
	return shopify.Config{
		StoreDomain:  os.Getenv("SHOPIFY_STORE_DOMAIN"),
		ClientID:     os.Getenv("SHOPIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SHOPIFY_CLIENT_SECRET"),
		WriteEnabled: os.Getenv("SHOPIFY_WRITE_ENABLED") == "true",
	}
}

// shopifyCheck reads the shop and the token's scopes. It writes nothing.
func shopifyCheck(ctx context.Context) error {
	cfg := shopifyConfig()
	client, err := shopify.NewClient(cfg)
	if err != nil {
		return err
	}
	shop, err := client.GetShop(ctx)
	if err != nil {
		return err
	}
	scopes, err := client.Scopes(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Store: %s (%s)\nPlan: %s\nDevelopment-Store: %v\nRechte: %s\nSchreiben eingeschaltet: %v\n",
		shop.Name, shop.Domain, shop.PlanName, shop.DevStore, strings.Join(scopes, ", "), cfg.WriteEnabled)
	if !shop.DevStore {
		fmt.Println("Achtung: Shopify meldet keinen Development-Store. velopim schreibt nicht in diesen Store.")
	}
	return nil
}

// runSync sends pending changes of the products selected for the store.
func runSync(ctx context.Context) error {
	pool, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	client, err := shopify.NewClient(shopifyConfig())
	if err != nil {
		return err
	}
	res, err := shopsync.Run(ctx, pool, client, time.Now())
	for _, p := range res.Products {
		line := p.Name + ": " + map[string]string{"synced": "übertragen", "unchanged": "unverändert", "failed": "fehlgeschlagen"}[p.Status]
		if p.Created {
			line += " (neu in Shopify, Entwurf)"
		}
		if p.Error != "" {
			line += " – " + p.Error
		}
		fmt.Println(line)
	}
	if err != nil {
		return err
	}
	fmt.Printf("%d übertragen, %d unverändert, %d fehlgeschlagen\n", res.Synced, res.Unchanged, res.Failed)
	return nil
}

func runImport(ctx context.Context, sourceID, path string) error {
	pool, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	conn, err := importer.ConnectorFor(ctx, db.New(pool), sourceID)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	res, err := importer.Run(ctx, pool, conn, f, importer.Options{
		RawDir: rawDir()})
	if err != nil {
		return err
	}
	fmt.Printf("Importlauf %d (%s): %d Artikel, %d neu, %d geändert, %d entfallen, %d Mengenänderungen\n",
		res.RunID, res.Status, res.Records, res.New, res.Changed, res.Removed, res.QuantityChanges)
	fmt.Printf("%d automatisch übernommen, %d zur Freigabe, %d Datenprobleme\n",
		res.AutoApplied, res.PendingApproval, res.Issues)
	return nil
}
