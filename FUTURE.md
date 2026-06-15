# Future Work

## Deployment and platform

- Build and publish multi-arch container images so the image runs cleanly on both `amd64` and `arm64` Droplets.
- Update the Dockerfile to use `TARGETOS` and `TARGETARCH`, or switch to `docker buildx` in CI.
- Push images from CI or a stronger build machine instead of compiling on a tiny Droplet.
- Revisit DigitalOcean App Platform once persistence is moved off SQLite.
- Add a DigitalOcean Container Registry deployment path to the docs.

## Persistence and schema management

- Consider switching from SQLite to Postgres for easier managed deployment and a better scaling story.
- Add more explicit migration workflow docs around `goose` usage.
- Decide whether expired aliases should remain permanently reserved or become reusable.
- Add a cleanup strategy for expired links so storage does not grow forever.

## Alias generation

- Expand the word lists substantially to improve uniqueness at larger scale.
- Consider adding an optional short numeric suffix or larger word pools to reduce collision probability.
- Consider curated memorable alias generation rules if human readability becomes a product goal.

## Redirect and analytics behavior

- Revisit access-count updates so redirect latency and goroutine growth do not depend on per-request async writes.
- Consider a bounded worker queue or batched counter flush strategy.
- Add clearer metrics around cache hit rate and redirect throughput.

## Testing

- Add explicit process-level end-to-end tests beyond `httptest` plus the bash smoke test.
- Add build/deployment tests for image architecture compatibility.
- Add heavier concurrency/load tests around generated alias creation and redirect traffic.
- Add tests for SQLite lock contention behavior under load.

## Operations and docs

- Add a ready-to-use raw-IP HTTP Caddy example to the docs.
- Add a ready-to-use domain + HTTPS Caddy example to the docs.
- Document recommended production environment variables and rollout steps more explicitly.
