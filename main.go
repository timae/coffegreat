// coffee (g)rate — a tiny, fast coffee journal.
//
// One static Go binary: server-rendered HTML, embedded assets & migrations,
// Postgres for storage. Built to idle at a few MB of RAM on Deploio.
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // so TZ=Europe/Zurich works in a scratch/distroless image
)

//go:embed web
var webFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

type App struct {
	db    *sql.DB
	views *Views
	auth  *Auth
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	db, err := openDB()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := migrate(context.Background(), db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	views, err := loadViews()
	if err != nil {
		log.Fatalf("templates: %v", err)
	}

	auth, err := newAuth()
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	app := &App{db: db, views: views, auth: auth}

	addr := ":" + getenv("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("coffee (g)rate listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
