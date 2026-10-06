package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

// openDB connects to Postgres. The connection string is taken from
// DATABASE_URL, or — when the app is wired to a Nine on-demand database via
// `nctl ... --service db=postgresdatabase/<name>` — from the injected
// NINE_PG*_DSN variable. If Nine also injects a CA certificate, the TLS
// connection is verified against it.
func openDB() (*sql.DB, error) {
	dsn, ca := databaseConfigFromEnv()
	if dsn == "" {
		return nil, errors.New("no DATABASE_URL (or NINE_PG*_DSN) set")
	}

	cfg, err := pq.NewConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if ca != "" {
		// Nine's CA does not match the hostname → verify the chain, not the name.
		cfg.SSLMode = pq.SSLModeVerifyCA
		cfg.SSLRootCert = ca
		cfg.SSLInline = true
	} else if !strings.Contains(dsn, "sslmode") && !isLocalHost(cfg.Host) {
		cfg.SSLMode = pq.SSLModeRequire
	}

	conn, err := pq.NewConnectorConfig(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(conn)
	// Economy databases allow 20 connections — stay well below.
	db.SetMaxOpenConns(maxConns())
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// The database may still be waking up during a deploy; retry briefly.
	var pingErr error
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pingErr = db.PingContext(ctx)
		cancel()
		if pingErr == nil {
			return db, nil
		}
		log.Printf("waiting for database (%d/10): %v", i+1, pingErr)
		time.Sleep(2 * time.Second)
	}
	return nil, pingErr
}

func maxConns() int {
	if n, err := strconv.Atoi(os.Getenv("DB_MAX_CONNS")); err == nil && n > 0 {
		return n
	}
	return 5
}

func databaseConfigFromEnv() (dsn, ca string) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v, os.Getenv("DATABASE_CA_CERT")
	}
	// Look for Nine service-connection variables, e.g. NINE_PGDB_DB_DSN.
	var keys []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "NINE_PG") && strings.HasSuffix(k, "_DSN") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "", ""
	}
	prefix := strings.TrimSuffix(keys[0], "_DSN")
	return os.Getenv(keys[0]), os.Getenv(prefix + "_CA_CERT")
}

func isLocalHost(h string) bool {
	return h == "" || h == "localhost" || h == "127.0.0.1" || h == "::1" ||
		strings.HasPrefix(h, "/") || h == "db" || h == "postgres"
}

// migrate applies embedded migrations/NNN_*.sql in order, exactly once each.
// An advisory lock keeps concurrent replicas from racing each other.
func migrate(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	const lockID = 727274 // "coffee"
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}

	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, f := range files {
		base := strings.TrimPrefix(f, "migrations/")
		numStr, _, _ := strings.Cut(base, "_")
		version, err := strconv.Atoi(numStr)
		if err != nil {
			return fmt.Errorf("bad migration name %q", base)
		}
		var exists bool
		if err := conn.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrationsFS.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", base, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("applied migration %s", base)
	}
	return nil
}
