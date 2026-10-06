# coffee (g)rate

A personal coffee journal: log the coffees you try, rate them across a 7-axis flavour radar, set up your grinder and machine, save portafilter recipes (basket, dose → yield ratio, time, temperature, grind, pre-infusion, pressure), and log every shot against them to dial in.

Every coffee gets its own generated "bag label". The colours come from its roast level and process, and the pattern is seeded by the coffee itself.

## Why Go

The app is a single static Go binary (~9 MB) with all templates, CSS, JS and SQL migrations built in, running in a distroless container (~12 MB). It idles at a few MB of RAM and starts in milliseconds, so the smallest Deploio size is enough. Pages are rendered on the server, and about 6 KB of plain JS adds the live bits: the radar, ratio and flow readouts, the shot timer and filling in a recipe. The only dependency is `lib/pq`.

```
main.go        server, graceful shutdown
db.go          Postgres connect (DATABASE_URL or Nine service DSN), embedded migrations
auth.go        single-password login, HMAC-signed session cookie, same-origin check, CSP
models.go      beans, grinders, brewers, recipes, shots
handlers.go    routes + handlers
render.go      templates & helpers
art.go         generative labels, flavour radar, shot rhythm chart (SVG)
web/           templates + static assets (embedded)
migrations/    numbered .sql files, applied once each on boot
```

## Deploy on Deploio

```bash
# 1. a Postgres database (Economy tier is plenty: CHF 5.50/mo)
nctl create postgresdatabase coffee-db

# 2. the app, built from the Dockerfile, wired to the database
nctl create app coffee-grate \
  --git-url=https://github.com/<you>/coffee-grate \
  --dockerfile \
  --size=micro --replicas=1 \
  --service db=postgresdatabase/coffee-db \
  --sensitive-env APP_PASSWORD='pick-something-long' \
  --env TZ=Europe/Zurich \
  --health-probe-path=/healthz
```

The app finds the injected `NINE_PG*_DSN` variable by itself and verifies TLS against the injected CA certificate (Nine's CA doesn't match the hostname, so it uses `verify-ca`). If you'd rather set the connection yourself, set `DATABASE_URL` (plus `DATABASE_CA_CERT` if you have one) instead of `--service`.

If the repo is private, add `--git-username/--git-password` or `--git-ssh-private-key-from-file`. Push to `main` and Deploio rebuilds. Migrations run on boot behind a Postgres advisory lock, so running several replicas is safe.

## Configuration

| Variable | Required | |
|---|---|---|
| `APP_PASSWORD` | yes | The one password that unlocks the app. Changing it logs out every session. |
| `DATABASE_URL` | one of | `postgres://user:pass@host:5432/db?sslmode=require` |
| `NINE_PG*_DSN` | one of | Injected automatically by `--service` |
| `TZ` | no | Time zone for showing times, e.g. `Europe/Zurich` (tzdata is built in) |
| `SESSION_SECRET` | no | Defaults to a value derived from `APP_PASSWORD` |
| `PORT` | no | Injected by Deploio, default `8080` |
| `DB_MAX_CONNS` | no | Default `5` (Economy DBs allow 20) |
| `DEV=1` | no | Run without a password. Local use only. |

## Local development

```bash
docker compose up --build          # → http://localhost:8080, password "espresso"
```

Or with Go installed and a Postgres running:

```bash
DATABASE_URL=postgres://localhost/coffee?sslmode=disable DEV=1 go run .
```

## Adding a field

Add `migrations/002_whatever.sql`, then update the struct, its column list and its save query in `models.go`, plus the form template. Each migration is applied once and recorded in `schema_migrations`.
