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
  - **Tyres**: summer, winter and all-season sets. With summer tyres fitted, winter ones are due by 15 November (required until 15 April); with winter tyres, summer ones by 15 May. Front/rear rotation every N km, and the km driven on each set.
  - **Custom reminders** (driving licence…), optionally repeating.
- **Expenses** by category, with PDF invoices and photos of receipts attached.
- **Fuel**: consumption with the full-to-full method (l/100 km and km/l, kWh for EVs), cost per km, average price.
- **Photos** of your vehicles, resized in the browser (HEIC from iPhones included).
- **Calendar feed** (iCal): subscribe from your phone and get deadlines with alerts 7 days and 1 day before.
- **Notifications** by email and [ntfy](https://ntfy.sh): a daily digest N days before each deadline (30, 7 and 1 by default), and when it has passed. Each reminder is sent only once.
- **Multi-user**: share a vehicle with your family as owner, editor or read-only. Single sign-on with Authentik, Authelia, Keycloak or any OpenID Connect provider.
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
| `MILE_BASE_URL` | | public address of MILE (e.g. `https://mile.example.com`), for links in notifications |
| `MILE_NOTIFY_HOUR` | `9` | local hour from which the daily notifications are sent |
| `TZ` | `Europe/Rome` | decides when a day starts for the deadlines |

### Email notifications

Each user enables email and/or ntfy in **Settings › Notifications**. ntfy needs nothing on the server; email needs an SMTP account:

| Variable | Default | |
|---|---|---|
| `MILE_SMTP_HOST` | | SMTP server, e.g. `smtp.gmail.com` |
| `MILE_SMTP_PORT` | `587` (`465` with `tls`) | |
| `MILE_SMTP_USERNAME` / `MILE_SMTP_PASSWORD` | | credentials (for Gmail, an app password) |
| `MILE_SMTP_FROM` | | sender, e.g. `MILE <mile@example.com>` |
| `MILE_SMTP_TLS` | `starttls` | `starttls`, `tls` (implicit, port 465) or `none` |

### Login with Authentik (OpenID Connect)

Users can log in with Authentik, or with any other OpenID Connect provider (Authelia, Keycloak, Pocket ID…). In Authentik:

1. **Applications › Providers › Create › OAuth2/OpenID Provider**: client type *Confidential*, redirect URI `https://mile.example.com/auth/oidc/callback` (your `MILE_BASE_URL` followed by `/auth/oidc/callback`). Pick a **signing key** (e.g. *authentik Self-signed Certificate*): without it, ID tokens are signed with the client secret and MILE rejects them.
2. **Applications › Applications › Create**: slug `mile`, with the provider above. Bind a group or policy to it to choose who can log in.
3. Set the variables below. The issuer is `https://auth.example.com/application/o/mile/`, trailing slash included.

| Variable | Default | |
|---|---|---|
| `MILE_OIDC_ISSUER` | | provider URL; needs `MILE_BASE_URL` too |
| `MILE_OIDC_CLIENT_ID` / `MILE_OIDC_CLIENT_SECRET` | | from the provider page (no secret for public clients) |
| `MILE_OIDC_NAME` | `SSO` | shown on the button: *Log in with Authentik* |
| `MILE_OIDC_AUTO_REGISTER` | `true` | create a MILE user at the first login; with `false` only linked users can log in |
| `MILE_OIDC_ADMIN_GROUP` | | members of this group are administrators, checked at every login (needs the `groups` claim) |

At the first login MILE creates the user with the Authentik username, and on a new installation the first user becomes the administrator. **Existing users** log in with their password once and link their account in **Settings › Login with Authentik**: a user with the same name is never linked automatically. Login with a password keeps working, and users created by Authentik can set one in Settings.

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

**Stack**: Go standard library + SQLite (`modernc.org/sqlite`, no CGO) + `go-oidc` for single sign-on, React + Vite embedded in the binary, distroless image.

```
cmd/mile/            entry point (serve, healthcheck, reset-password)
internal/deadlines/  Italian deadline rules (pure functions, tested)
internal/fuel/       consumption, full-to-full method
internal/store/      data access and validation
internal/server/     JSON API, attachments, calendar feed, OpenID Connect login
internal/ical/       iCalendar rendering
web/                 React frontend
```

## Roadmap

- More notification channels: Apprise, web push
- Import from Fuelio and LubeLogger, CSV export
- Receipt scanning

## License

Copyright (C) 2026 Gabriele Menghi.

MILE is free software: you can use, study, modify and redistribute it under the terms of the [GNU Affero General Public License v3.0 only](LICENSE). If you run a modified version as a service for other people, you must share its source code with them. See [NOTICE](NOTICE).

The **MILE name and logo are not covered by the license**: forks must use a different name and logo. See the [trademark policy](TRADEMARKS.md).

Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md).
