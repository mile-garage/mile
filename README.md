# MILE

**M**aintenance, **I**nsurance, **L**ogs & **E**xpenses.

Self-hosted tracker for your vehicles: deadlines, expenses, fuel and documents, with the Italian rules built in.

*Scadenziario per i tuoi veicoli: revisione, bollo, assicurazione (anche sospesa), tagliando, spese con fatture e scontrini, consumi e foto. Da installare sul tuo server, anche su TrueNAS.*

- **Deadlines computed, not typed in**
  - **Inspection (revisione)**: the first one is due 4 years after registration, then every 2 years, by the end of the month. A yearly rule is available for taxis and heavy vehicles.
  - **Road tax (bollo)**: due by the last day of the month after it expires. Exemptions are supported (e.g. electric and historic vehicles).
  - **Insurance with suspensions**: the expiry moves forward by the suspended days. The 15-day grace period is shown.
  - **Service (tagliando)**: due every N km or M months, whichever comes first. The km limit is turned into a date using your average daily distance.
  - **Oil change**: its own km/months interval, handy for motorcycles and scooters. A service also resets it.
  - **Custom reminders** (driving licence, tyres…), optionally repeating.
- **Expenses** by category, with PDF invoices and photos of receipts attached.
- **Fuel**: consumption with the full-to-full method (l/100 km and km/l, kWh for EVs), cost per km, average price.
- **Photos** of your vehicles, resized in the browser (HEIC from iPhones included).
- **Calendar feed** (iCal): subscribe from your phone and get deadlines with alerts 7 days and 1 day before.
- **Multi-user**: share a vehicle with your family as owner, editor or read-only.
- **Mobile first**: installable as an app from the browser (PWA). Italian and English, light and dark theme.

One ~15 MB container, one SQLite file and a folder of attachments, nothing else to run.

## Install with Docker

```yaml
services:
  mile:
    image: ghcr.io/mile-garage/mile:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      TZ: Europe/Rome
    volumes:
      - ./data:/data
```

```bash
docker compose up -d
```

Open `http://<server>:8080`: the first account you create is the administrator. Images are published for `linux/amd64` and `linux/arm64`.

### TrueNAS SCALE

**Apps › Discover Apps › Custom App** (or *Install via YAML* with the compose above):

- **Image**: `ghcr.io/mile-garage/mile`, tag `latest`
- **Port**: container `8080`
- **Storage**: a host path dataset (e.g. `/mnt/tank/apps/mile`) mounted on `/data`
- **User**: run the container as the dataset's owner (usually `568:568`, the `apps` user), or give that user write access to the dataset

Back up the dataset with periodic snapshots: it contains the database, its daily copies and all the attachments.

### Behind a reverse proxy

MILE works behind Caddy, Traefik or Nginx Proxy Manager. Serve it over **HTTPS** if it is reachable from outside your network: the session cookie is marked `Secure` when the proxy sends `X-Forwarded-Proto: https`.

## Configuration

| Variable | Default | |
|---|---|---|
| `MILE_DATA_DIR` | `/data` (in Docker) | database, backups, attachments |
| `MILE_ADDR` | `:8080` | listen address |
| `MILE_MAX_UPLOAD_MB` | `25` | maximum attachment size |
| `MILE_BACKUP_KEEP` | `14` | daily database backups to keep |
| `TZ` | `Europe/Rome` | decides when a day starts for the deadlines |

**Locked out?** Reset a password from the command line:

```bash
docker compose exec mile /mile reset-password <username> <new-password>
```

**Backups**: every day `/data/backups/mile-YYYY-MM-DD.db` is created. To restore, stop the container, replace `/data/mile.db` with a backup, and start it again. Attachments live in `/data/files`.

## Development

Requirements: Go 1.27+, Node 22+.

```bash
cd web && npm install && npm run build && cd ..
go run ./cmd/mile                  # http://localhost:8080, data in ./data
```

For the frontend with hot reload, with the Go server running:

```bash
npm --prefix web run dev           # http://localhost:5173, API proxied to :8080
```

Tests: `go test ./...` · Type check: `npm --prefix web run typecheck`

**Stack**: Go standard library + SQLite (`modernc.org/sqlite`, no CGO), React + Vite embedded in the binary, distroless image.

```
cmd/mile/            entry point (serve, healthcheck, reset-password)
internal/deadlines/  Italian deadline rules (pure functions, tested)
internal/fuel/       consumption, full-to-full method
internal/store/      data access and validation
internal/server/     JSON API, attachments, calendar feed
internal/ical/       iCalendar rendering
web/                 React frontend
```

## Roadmap

- Notifications: email, ntfy / Apprise, web push
- Login with OpenID Connect (Authentik, Authelia, Keycloak…)
- Import from Fuelio and LubeLogger, CSV export
- Receipt scanning

## License

Copyright (C) 2026 Gabriele Menghi.

MILE is free software: you can use, study, modify and redistribute it under the terms of the [GNU Affero General Public License v3.0 only](LICENSE). If you run a modified version as a service for other people, you must share its source code with them. See [NOTICE](NOTICE).

The **MILE name and logo are not covered by the license**: forks must use a different name and logo. See the [trademark policy](TRADEMARKS.md).

Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md).
