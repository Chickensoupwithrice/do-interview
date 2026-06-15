# Architecture

```mermaid
flowchart LR
    Client -->|POST /api/urls| API[Go HTTP service]
    Client -->|GET /api/urls/:alias| API
    Client -->|GET /:alias| API

    API --> Validate[Validate input and alias]
    Validate --> Service[Shortener service]
    Service --> Cache{Redirect cache hit?}
    Cache -->|Yes| Count[Increment access count]
    Cache -->|No| DB[(SQLite)]
    DB --> Count
    Count --> Redirect[302 redirect response]

    Service -->|Create| DB
    DB -->|Unique alias enforced| Service
    Service --> JSON[JSON response]
    JSON --> Client

    DB --> Metadata[Stored URL, TTL, access count, timestamps]
    Metadata --> API
```

## Request lifecycle

- Create requests validate the URL and optional alias/TTL, then insert into SQLite.
- SQLite enforces alias uniqueness, which makes simultaneous custom-alias requests deterministic.
- Redirects check the in-memory cache first. On miss, the service loads from SQLite, verifies TTL, increments access count, then redirects.
- Metadata reads load the record from SQLite and also enforce TTL.
