package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

const sqliteBusyTimeout = time.Second

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dsn := (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: fmt.Sprintf("_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)", sqliteBusyTimeout/time.Millisecond),
	}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	if err := runMigrations(context.Background(), db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) CreateLink(ctx context.Context, link Link) (Link, error) {
	err := withBusyRetry(ctx, func() error {
		_, err := s.db.ExecContext(
			ctx,
			`INSERT INTO links (alias, original_url, created_at, expires_at, access_count)
			 VALUES (?, ?, ?, ?, ?);`,
			link.Alias,
			link.OriginalURL,
			link.CreatedAt.UTC(),
			link.ExpiresAt,
			link.AccessCount,
		)
		return err
	})
	if err != nil {
		if isUniqueConstraint(err) {
			return Link{}, ErrAliasConflict
		}
		return Link{}, err
	}

	return link, nil
}

func (s *SQLiteStore) GetLink(ctx context.Context, alias string) (Link, error) {
	var link Link
	var expiresAt sql.NullTime

	err := withBusyRetry(ctx, func() error {
		return s.db.QueryRowContext(
			ctx,
			`SELECT alias, original_url, created_at, expires_at, access_count
			 FROM links WHERE alias = ?;`,
			alias,
		).Scan(&link.Alias, &link.OriginalURL, &link.CreatedAt, &expiresAt, &link.AccessCount)
	})
	if err != nil {
		return Link{}, err
	}

	if expiresAt.Valid {
		value := expiresAt.Time.UTC()
		link.ExpiresAt = &value
	}
	link.CreatedAt = link.CreatedAt.UTC()

	return link, nil
}

func (s *SQLiteStore) IncrementAccessCount(ctx context.Context, alias string) error {
	return withBusyRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `UPDATE links SET access_count = access_count + 1 WHERE alias = ?;`, alias)
		return err
	})
}

func (s *SQLiteStore) HealthCheck(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique") || errors.Is(err, ErrAliasConflict)
}

func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "sqlite_busy") || strings.Contains(strings.ToLower(err.Error()), "database is locked")
}

func withBusyRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := range 5 {
		err = fn()
		if !isBusyError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
		}
	}
	return err
}
