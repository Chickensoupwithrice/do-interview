-- +goose Up
CREATE TABLE IF NOT EXISTS links (
    alias TEXT PRIMARY KEY,
    original_url TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NULL,
    access_count INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS links;
