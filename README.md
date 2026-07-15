# PDC Archive

An independent mirror of Washington State Public Disclosure Commission (PDC) campaign finance data from [data.wa.gov](https://data.wa.gov). Data is stored in PostgreSQL and served through a **SODA 2-compatible API** — swap the hostname in existing queries and they keep working.

## Features

- **Drop-in SODA 2 API** — SoQL core (`$select`, `$where`, `$group`, `$having`, `$order`, `$limit`, `$offset`, `$q`, `$query`, `$distinct`) plus JSON/CSV
- **Public homepage** — dataset catalog and API documentation
- **Unauthenticated API** — no API keys required for data access
- **Admin dashboard** — login-protected sync controls at `/admin`
- **Background scheduler** — automatic daily sync at a configurable time
- **Railway-ready** — Dockerfile, health check, `PORT` binding

## Quick start (local)

```bash
cp .env.example .env          # edit ADMIN_PASSWORD and SESSION_SECRET
docker run -d --name pdcarchive-pg \
  -e POSTGRES_USER=pdcarchive -e POSTGRES_PASSWORD=pdcarchive -e POSTGRES_DB=pdcarchive \
  -p 5432:5432 postgres:16-alpine
go run ./cmd/pdcarchive       # start server, scheduler, and admin UI
```

Trigger the initial data load from the admin dashboard (**Sync All**), or wait for the daily scheduled sync (`SYNC_TIME`, default 2:00 AM Pacific).

- Public site: http://localhost:8080/
- SODA API: http://localhost:8080/resource/kv7h-kjye.json?$limit=1
- Admin: http://localhost:8080/admin/login

## API examples

```bash
curl "http://localhost:8080/resource/kv7h-kjye.json?\$limit=1"
curl "http://localhost:8080/resource/kv7h-kjye.json?\$select=count(*)"
curl "http://localhost:8080/resource/kv7h-kjye.json?\$where=amount>500&\$limit=2"
curl "http://localhost:8080/resource/kv7h-kjye.json?\$select=party,sum(amount)%20AS%20total&\$group=party&\$order=total%20DESC&\$limit=5"
curl "http://localhost:8080/resource/kv7h-kjye.csv?\$limit=100"
curl "http://localhost:8080/api/views/kv7h-kjye.json"
curl "http://localhost:8080/api/views/kv7h-kjye/columns.json"
```

Supported SoQL: `$select` (including aggregates/aliases), `$where` (`AND`/`OR`/`NOT`, `IN`, `BETWEEN`, `LIKE`, `starts_with`, …), `$group`, `$having`, `$order`, `$limit`, `$offset`, `$q`, `$query`, `$distinct`. Geospatial functions are not supported.

Replace `localhost:8080` with your deployment host for production use.

## Configuration

See [.env.example](.env.example). Key variables:

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `DB_MAX_OPEN_CONNS` | Max open PostgreSQL connections (default `12`) |
| `DB_MAX_IDLE_CONNS` | Max idle PostgreSQL connections (default `5`) |
| `PORT` | HTTP port (Railway sets automatically) |
| `SOURCE_BASE_URL` | Upstream Socrata host (default `https://data.wa.gov`) |
| `SYNC_TIME` | Daily sync time in `HH:MM` (default `02:00`) |
| `SYNC_TIMEZONE` | Timezone for `SYNC_TIME` (default `America/Los_Angeles`) |
| `SYNC_PAGE_SIZE` | Records fetched per upstream request (default `1000`) |
| `SYNC_PAGE_INTERVAL_MIN` | Minimum delay between upstream requests (default `5s`) |
| `SYNC_PAGE_INTERVAL_MAX` | Maximum delay between upstream requests (default `15s`) |
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

## Mirrored datasets

26 PDC datasets are hardcoded in [`internal/datasets/datasets.go`](internal/datasets/datasets.go).

### Campaign finance

| ID | Name |
|----|------|
| `kv7h-kjye` | Contributions to Candidates and Political Committees |
| `tijg-9zyp` | Expenditures by Candidates and Political Committees |
| `7qr9-q2c9` | Campaign Finance Reporting History |
| `3h9x-7bvm` | Campaign Finance Summary |
| `3r6b-hsaa` | Debt Reported by Candidates and Political Committees |
| `d2ig-r3q4` | Loans to Candidates and Political Committees |
| `67cp-h962` | Independent Campaign Expenditures and Electioneering Communications |
| `mppc-zjn9` | Last Minute Contributions to Candidates and Political Committees |
| `8bva-rkeb` | Pledges Reporting History |
| `9kcu-2bem` | Candidate Surplus Funds Reports |
| `ti55-mvy5` | Surplus Funds Expenditures |
| `qdtg-6yir` | Contributions to out-of-state political committees |
| `mzg4-pm9n` | Expenditures by out-of-state political committees |

### Lobbying

| ID | Name |
|----|------|
| `9nnw-c693` | Lobbyist Compensation and Expenses by Source |
| `nuwx-ay5h` | Lobbyist Reporting History |
| `xhn7-64im` | Lobbyist Employment Registrations |
| `biux-xiwe` | Lobbyist Employers Summary |
| `c4ag-3cmj` | Lobbyist Summary |
| `e7sd-jbuy` | Lobbyist Agent Employers |
| `bp5b-jrti` | Lobbyist Agents |
| `mjwb-szba` | Public Agency Lobbying Totals |
| `ef7g-tyg8` | L7 - Employment of Legislators and State Officials |
| `3v2j-kqbi` | Pre-2016 Lobbyist Compensation and Expenses by Source |

### Financial affairs and enforcement

| ID | Name |
|----|------|
| `ehbc-shxw` | Financial Affairs Disclosures |
| `a4ma-dq6s` | PDC Enforcement Cases |
| `ub89-7wbv` | PDC Enforcement Case Attachments |

## Attribution

Data sourced from the Washington State Public Disclosure Commission via [data.wa.gov](https://data.wa.gov). This project is an independent archive and is not affiliated with the PDC or the State of Washington. Mirrored data is dedicated to the public domain under [CC0 1.0 Universal](https://creativecommons.org/publicdomain/zero/1.0/).

## License

- **Data**: [CC0 1.0 Universal](https://creativecommons.org/publicdomain/zero/1.0/) (Public Domain Dedication)
- **Software**: see [LICENSE](LICENSE)
