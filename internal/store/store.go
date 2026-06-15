package store

import (
	"context"
	"errors"
	"time"
)

var ErrAliasConflict = errors.New("alias already exists")

type Link struct {
	Alias       string
	OriginalURL string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	AccessCount int64
}

type Store interface {
	CreateLink(ctx context.Context, link Link) (Link, error)
	GetLink(ctx context.Context, alias string) (Link, error)
	IncrementAccessCount(ctx context.Context, alias string) error
	HealthCheck(ctx context.Context) error
	Close() error
}
