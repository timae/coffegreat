package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
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
		if pool, perr := parseCA(ca); perr == nil {
			// Nine's CA does not match the hostname → verify the chain, not the name.
			// (We register our own tls.Config instead of pq's sslinline mode, which
			// would also demand a client certificate.)
			if err := pq.RegisterTLSConfig("nineca", verifyChainOnly(pool)); err != nil {
				return nil, err
			}
			cfg.SSLMode = "pqgo-nineca"
			log.Println("database: TLS, verifying server certificate against provided CA")
		} else {
			log.Printf("WARNING: could not parse database CA certificate (%v) — falling back to sslmode=require", perr)
			cfg.SSLMode = pq.SSLModeRequire
		}
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

// parseCA accepts the CA as plain PEM, PEM with escaped "\n", base64-encoded
// PEM, or base64-encoded DER — whatever form the platform hands us.
func parseCA(s string) (*x509.CertPool, error) {
	s = strings.TrimSpace(s)
	candidates := [][]byte{[]byte(s), []byte(strings.ReplaceAll(s, `\n`, "\n"))}
	if b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), "")); err == nil {
		candidates = append(candidates, b)
	}
	pool := x509.NewCertPool()
	for _, c := range candidates {
		if pool.AppendCertsFromPEM(c) {
			return pool, nil
		}
		if cert, err := x509.ParseCertificate(c); err == nil {
			pool.AddCert(cert)
			return pool, nil
		}
	}
	return nil, errors.New("no PEM or DER certificate found")
}

// verifyChainOnly checks the server certificate chain against pool but skips
// hostname matching (equivalent to libpq's sslmode=verify-ca).
func verifyChainOnly(pool *x509.CertPool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // hostname check off; chain verified below
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("database presented no certificate")
			}
			certs := make([]*x509.Certificate, 0, len(raw))
			for _, r := range raw {
				c, err := x509.ParseCertificate(r)
				if err != nil {
					return err
				}
				certs = append(certs, c)
			}
			opts := x509.VerifyOptions{Roots: pool, Intermediates: x509.NewCertPool()}
			for _, c := range certs[1:] {
				opts.Intermediates.AddCert(c)
			}
			_, err := certs[0].Verify(opts)
			return err
		},
	}
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
