# URL Shortener

Small Go URL shortener built for an interview exercise. It exposes a REST API for creating shortened URLs, supports custom aliases, generates readable four-word aliases by default, handles alias collisions safely with SQLite uniqueness constraints, redirects short links, tracks access counts, caches hot redirects in memory, and enforces optional TTL expiration.

## API

`POST /api/urls`

Request:

```json
{
  "url": "https://example.com/some/long/path",
  "alias": "launch-2026",
  "ttl_seconds": 3600
}
```

Response:

```json
{
  "alias": "launch-2026",
  "original_url": "https://example.com/some/long/path",
  "short_url": "http://localhost:8080/launch-2026",
  "created_at": "2026-06-15T12:00:00Z",
  "expires_at": "2026-06-15T13:00:00Z",
  "access_count": 0
}
```

`GET /api/urls/{alias}` returns metadata.

`GET /{alias}` redirects to the original URL.

`GET /healthz` returns service health.

## Run locally

Requirements:

- Go 1.26+

Run:

```bash
go run ./cmd/api
```

Environment variables:

- `ADDR` default: `:8080`
- `BASE_URL` default: `http://localhost:8080`
- `DATABASE_PATH` default: `data/shortener.db`

Example create call:

```bash
curl -X POST http://localhost:8080/api/urls \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/docs","alias":"docs","ttl_seconds":3600}'
```

## Test

```bash
go test ./...
./scripts/e2e.sh
```

Or run both with:

```bash
./scripts/test.sh
```

## Design notes

- `net/http` only for a minimal standard-library stack.
- `SQLite` gives persistence, uniqueness constraints, and simple deployment.
- Custom alias race conditions are handled by the database primary key.
- Redirect caching is in-memory and only used for alias-to-URL lookups.
- TTL is enforced on metadata reads and redirects. Expired links return `410 Gone`.
- Expired aliases remain reserved and are not automatically recycled.

## Deployment to DigitalOcean

This repo includes:

- `Dockerfile`
- `compose.yaml`
- `Caddyfile`
- `.do/droplet.md`
- `.github/workflows/ci.yml`

Schema changes are managed with `goose` migrations stored in `internal/store/migrations/`.

On every push to `main`, GitHub Actions runs tests and publishes a multi-arch image to DigitalOcean Container Registry as:

- `registry.digitalocean.com/do-interview/url-shortener:latest`
- `registry.digitalocean.com/do-interview/url-shortener:<git-sha>`

To enable image publishing, add the GitHub Actions secret `DIGITALOCEAN_ACCESS_TOKEN` with permission to push to the registry.

For this SQLite-backed build, the simplest durable DO deployment is a single Droplet with a mounted host directory for `/app/data` and a small TLS reverse proxy in front.

Why not App Platform? Its container filesystem is ephemeral, which would wipe SQLite data on restart or redeploy.

Basic flow:

```bash
gh repo create YOUR_GITHUB_USERNAME/url-shortener --public
git remote add origin git@github.com:YOUR_GITHUB_USERNAME/url-shortener.git
git push -u origin main
```

Then follow `.do/droplet.md` on the target Droplet.

If you prefer Compose on the Droplet, update `compose.yaml` and `Caddyfile`, then run:

```bash
mkdir -p data
docker compose pull
docker compose up -d
```

The first production hardening step after the interview would be moving persistence to a managed database and then reconsidering App Platform.

## Architecture diagram

See `ARCHITECTURE.md` for the request/data-flow diagram.
