package shortener

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/url-shortener/internal/store"
)

func TestCreateRejectsConflictingCustomAlias(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	input := CreateInput{URL: "https://example.com/a", Alias: "custom123"}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Create(context.Background(), input)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	var conflicts, successes int
	for err := range results {
		switch err {
		case nil:
			successes++
		case ErrAliasConflict:
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("got %d successes and %d conflicts", successes, conflicts)
	}
}

func TestGetReturnsExpiredForExpiredLink(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	service.SetNow(func() time.Time { return now })
	ttl := int64(1)

	_, err := service.Create(context.Background(), CreateInput{
		URL:        "https://example.com/a",
		Alias:      "expiresoon",
		TTLSeconds: &ttl,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	service.SetNow(func() time.Time { return now.Add(2 * time.Second) })
	_, err = service.Get(context.Background(), "expiresoon")
	if err != ErrExpired {
		t.Fatalf("expected expired, got %v", err)
	}
}

func TestResolveExpiresAtBoundaryAfterCacheWarmup(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	service.SetNow(func() time.Time { return now })
	ttl := int64(1)

	_, err := service.Create(context.Background(), CreateInput{
		URL:        "https://example.com/a",
		Alias:      "boundary1",
		TTLSeconds: &ttl,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	url, err := service.Resolve(context.Background(), "boundary1")
	if err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	if url != "https://example.com/a" {
		t.Fatalf("unexpected url: %q", url)
	}

	service.SetNow(func() time.Time { return now.Add(time.Second) })
	_, err = service.Resolve(context.Background(), "boundary1")
	if err != ErrExpired {
		t.Fatalf("expected expired, got %v", err)
	}

	link, err := service.Get(context.Background(), "boundary1")
	if err != ErrExpired {
		t.Fatalf("expected expired metadata, got %v", err)
	}
	if link.AccessCount != 0 {
		t.Fatalf("expected no metadata payload on expired link, got %+v", link)
	}
	if err := waitForAccessCount(service, "boundary1", 1); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.cache.Get("boundary1", now.Add(time.Second)); ok {
		t.Fatal("expected expired cache entry to be evicted")
	}
}

func TestCreateRejectsOversizedTTL(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	ttl := maxTTLSeconds + 1

	_, err := service.Create(context.Background(), CreateInput{
		URL:        "https://example.com/a",
		TTLSeconds: &ttl,
	})
	if err != ErrInvalidTTL {
		t.Fatalf("expected invalid ttl, got %v", err)
	}
}

func TestResolveIncrementsAccessCountOnCacheHit(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	_, err := service.Create(context.Background(), CreateInput{
		URL:   "https://example.com/a",
		Alias: "cached12",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for i := 0; i < 2; i++ {
		url, err := service.Resolve(context.Background(), "cached12")
		if err != nil {
			t.Fatalf("resolve %d: %v", i, err)
		}
		if url != "https://example.com/a" {
			t.Fatalf("unexpected url: %q", url)
		}
	}

	if err := waitForAccessCount(service, "cached12", 2); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRejectsReservedAlias(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	_, err := service.Create(context.Background(), CreateInput{
		URL:   "https://example.com/a",
		Alias: "healthz",
	})
	if err != ErrInvalidAlias {
		t.Fatalf("expected invalid alias, got %v", err)
	}
}

func TestCreateGeneratesReadableAlias(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	link, err := service.Create(context.Background(), CreateInput{URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	parts := strings.Split(link.Alias, "-")
	if len(parts) != 4 {
		t.Fatalf("expected 4 alias words, got %q", link.Alias)
	}
	for _, part := range parts {
		if part == "" {
			t.Fatalf("expected non-empty alias parts, got %q", link.Alias)
		}
	}
}

func TestCreateUsesSingleTimestampForCreatedAtAndExpiry(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	service.SetNow(func() time.Time {
		if calls.Add(1) == 1 {
			return base
		}
		return base.Add(10 * time.Second)
	})
	ttl := int64(5)

	link, err := service.Create(context.Background(), CreateInput{
		URL:        "https://example.com/a",
		Alias:      "oneclock",
		TTLSeconds: &ttl,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !link.CreatedAt.Equal(base) {
		t.Fatalf("expected created_at %v, got %v", base, link.CreatedAt)
	}
	if link.ExpiresAt == nil || !link.ExpiresAt.Equal(base.Add(5*time.Second)) {
		t.Fatalf("expected expires_at %v, got %v", base.Add(5*time.Second), link.ExpiresAt)
	}
}

func TestResolveReturnsCachedURLWhenAccessCountUpdateFails(t *testing.T) {
	t.Parallel()

	service, err := NewService(&failingCountStore{
		link: store.Link{
			Alias:       "cached12",
			OriginalURL: "https://example.com/a",
			CreatedAt:   time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}, NewCache(), "http://localhost:8080")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	url, err := service.Resolve(context.Background(), "cached12")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if url != "https://example.com/a" {
		t.Fatalf("unexpected url: %q", url)
	}

	url, err = service.Resolve(context.Background(), "cached12")
	if err != nil {
		t.Fatalf("cached resolve: %v", err)
	}
	if url != "https://example.com/a" {
		t.Fatalf("unexpected cached url: %q", url)
	}
}

func TestNewServiceRejectsInvalidBaseURL(t *testing.T) {
	t.Parallel()

	for _, baseURL := range []string{"foo", "mailto:test@example.com", "https://example.com/base"} {
		_, err := NewService(&failingCountStore{}, NewCache(), baseURL)
		if err != ErrInvalidBaseURL {
			t.Fatalf("base_url %q: expected invalid base url, got %v", baseURL, err)
		}
	}
}

func TestNewServiceRejectsNilDependencies(t *testing.T) {
	t.Parallel()

	if _, err := NewService(nil, NewCache(), "http://localhost:8080"); err != ErrInvalidDependency {
		t.Fatalf("expected invalid dependency, got %v", err)
	}
	if _, err := NewService(&failingCountStore{}, nil, "http://localhost:8080"); err != ErrInvalidDependency {
		t.Fatalf("expected invalid dependency, got %v", err)
	}
}

func TestCreateRejectsReusingExpiredAlias(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	service.SetNow(func() time.Time { return base })
	ttl := int64(1)

	_, err := service.Create(context.Background(), CreateInput{
		URL:        "https://example.com/a",
		Alias:      "reusable",
		TTLSeconds: &ttl,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	service.SetNow(func() time.Time { return base.Add(2 * time.Second) })
	_, err = service.Create(context.Background(), CreateInput{
		URL:   "https://example.com/b",
		Alias: "reusable",
	})
	if err != ErrAliasConflict {
		t.Fatalf("expected alias conflict, got %v", err)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	store, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.RemoveAll(tempDir)
	})

	service, err := NewService(store, NewCache(), "http://localhost:8080")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func waitForAccessCount(service *Service, alias string, want int64) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := service.store.GetLink(context.Background(), alias)
		if err == nil && stored.AccessCount == want {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	stored, err := service.store.GetLink(context.Background(), alias)
	if err != nil {
		return err
	}
	return fmt.Errorf("access count did not reach expected value: got %d want %d", stored.AccessCount, want)
}

type failingCountStore struct {
	link store.Link
}

func (s *failingCountStore) CreateLink(_ context.Context, link store.Link) (store.Link, error) {
	s.link = link
	return link, nil
}

func (s *failingCountStore) GetLink(_ context.Context, alias string) (store.Link, error) {
	if alias != s.link.Alias {
		return store.Link{}, errors.New("not found")
	}
	return s.link, nil
}

func (s *failingCountStore) IncrementAccessCount(context.Context, string) error {
	return errors.New("write failed")
}

func (s *failingCountStore) HealthCheck(context.Context) error {
	return nil
}

func (s *failingCountStore) Close() error {
	return nil
}
