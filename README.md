# PDC Archive

An independent mirror of Washington State Public Disclosure Commission (PDC) campaign finance data from [data.wa.gov](https://data.wa.gov). Data is stored in PostgreSQL and served through a **SODA 2-compatible API** — swap the hostname in existing queries and they keep working.

## Features

- **Drop-in SODA 2 API** — `GET /resource/{id}.json` with `$select`, `$where`, `$order`, `$limit`, `$offset`
- **Public homepage** — dataset catalog and API documentation
- **Unauthenticated API** — no API keys required for data access
- **Admin dashboard** — login-protected sync controls at `/admin`
- **Background scheduler** — automatic sync on a configurable interval
- **Railway-ready** — Dockerfile, health check, `PORT` binding

## Quick start (local)

```bash
cp .env.example .env          # edit ADMIN_PASSWORD and SESSION_SECRET
docker compose up -d          # start Postgres
go run ./cmd/pdcarchive       # start server, scheduler, and admin UI
```

Trigger the initial data load from the admin dashboard (**Sync All**), or wait for the scheduled sync on `SYNC_INTERVAL`.

- Public site: http://localhost:8080/
- SODA API: http://localhost:8080/resource/kv7h-kjye.json?$limit=1
- Admin: http://localhost:8080/admin/login

## API examples

```bash
curl "http://localhost:8080/resource/kv7h-kjye.json?\$limit=1"
curl "http://localhost:8080/resource/kv7h-kjye.json?\$select=count(*)"
curl "http://localhost:8080/resource/kv7h-kjye.json?\$where=amount>500&\$limit=2"
curl "http://localhost:8080/api/views/kv7h-kjye/columns.json"
```

Replace `localhost:8080` with your deployment host for production use.

## Configuration

See [.env.example](.env.example). Key variables:

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `PORT` | HTTP port (Railway sets automatically) |
| `SOURCE_BASE_URL` | Upstream Socrata host (default `https://data.wa.gov`) |
| `DATASETS` | Comma-separated resource IDs to mirror |
| `SYNC_INTERVAL` | Background sync interval (default `24h`) |
| `SYNC_PAGE_SIZE` | Records fetched per upstream request (default `1000`) |
| `SYNC_PAGE_INTERVAL` | Delay between upstream requests (default `1s`) |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | Admin dashboard credentials |
| `SESSION_SECRET` | Session cookie signing key |
| `SOCRATA_APP_TOKEN` | Optional upstream app token for higher rate limits |

Admin credentials are read once at startup. After editing `.env`, **save the file and restart** the application for changes to take effect. On startup, the server logs whether the password is `custom` or `DEFAULT`.

## Deploy to Railway

1. Create a Railway project and add **PostgreSQL**
2. Deploy from this GitHub repo (uses `Dockerfile`)
3. Set variables on the web service:
   - `DATABASE_URL` = `${{Postgres.DATABASE_URL}}`
   - `ADMIN_PASSWORD`, `SESSION_SECRET`, `GIN_MODE=release`
4. Health check path: `/health`
5. After deploy, log in to `/admin` and trigger **Sync All** for the initial data load

## Datasets (default)

| ID | Name |
|----|------|
| `kv7h-kjye` | Contributions to Candidates and Political Committees |
| `tijg-9zyp` | Expenditures by Candidates and Political Committees |
| `7qr9-q2c9` | Campaign Finance Reporting History |
| `3h9x-7bvm` | Campaign Finance Summary |
| `3r6b-hsaa` | Debt Reported by Candidates and Political Committees |
| `d2ig-r3q4` | Loans to Candidates and Political Committees |

## Attribution

Data sourced from the Washington State Public Disclosure Commission via [data.wa.gov](https://data.wa.gov). This project is an independent archive and is not affiliated with the PDC or the State of Washington. Mirrored data is dedicated to the public domain under [CC0 1.0 Universal](https://creativecommons.org/publicdomain/zero/1.0/).

## License

- **Data**: [CC0 1.0 Universal](https://creativecommons.org/publicdomain/zero/1.0/) (Public Domain Dedication)
- **Software**: GNU GPL v3 — see [LICENSE](LICENSE). Source code: [github.com/ihamburglar/pdcarchive](https://github.com/ihamburglar/pdcarchive)
