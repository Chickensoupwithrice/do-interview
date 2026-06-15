package shortener

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/example/url-shortener/internal/store"
)

var (
	ErrInvalidURL        = errors.New("invalid url")
	ErrInvalidAlias      = errors.New("invalid alias")
	ErrAliasConflict     = store.ErrAliasConflict
	ErrNotFound          = errors.New("link not found")
	ErrExpired           = errors.New("link expired")
	ErrInvalidTTL        = errors.New("ttl must be positive")
	ErrInvalidBaseURL    = errors.New("invalid base url")
	ErrInvalidDependency = errors.New("invalid dependency")
)

const maxTTLSeconds = int64(math.MaxInt64 / int64(time.Second))

var reservedAliases = map[string]struct{}{
	"healthz": {},
}

type Service struct {
	mu      sync.RWMutex
	store   store.Store
	cache   *Cache
	baseURL string
	now     func() time.Time
}

type CreateInput struct {
	URL        string
	Alias      string
	TTLSeconds *int64
}

type Link struct {
	Alias       string     `json:"alias"`
	OriginalURL string     `json:"original_url"`
	ShortURL    string     `json:"short_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	AccessCount int64      `json:"access_count"`
}

func NewService(store store.Store, cache *Cache, baseURL string) (*Service, error) {
	if store == nil || cache == nil {
		return nil, ErrInvalidDependency
	}
	normalizedBaseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &Service{
		store:   store,
		cache:   cache,
		baseURL: normalizedBaseURL,
		now:     func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *Service) SetNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (s *Service) Health(ctx context.Context) error {
	return s.store.HealthCheck(ctx)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Link, error) {
	parsedURL, err := normalizeURL(input.URL)
	if err != nil {
		return Link{}, err
	}
	now := s.currentTime()

	var expiresAt *time.Time
	if input.TTLSeconds != nil {
		if *input.TTLSeconds <= 0 || *input.TTLSeconds > maxTTLSeconds {
			return Link{}, ErrInvalidTTL
		}
		value := now.Add(time.Duration(*input.TTLSeconds) * time.Second)
		expiresAt = &value
	}

	if input.Alias != "" {
		if err := validateAlias(input.Alias); err != nil {
			return Link{}, err
		}
		return s.createWithAlias(ctx, input.Alias, parsedURL, expiresAt, now)
	}

	for range 10 {
		alias, err := generateAlias()
		if err != nil {
			return Link{}, fmt.Errorf("generate alias: %w", err)
		}
		if _, reserved := reservedAliases[alias]; reserved {
			continue
		}
		link, err := s.createWithAlias(ctx, alias, parsedURL, expiresAt, now)
		if errors.Is(err, ErrAliasConflict) {
			continue
		}
		return link, err
	}

	return Link{}, ErrAliasConflict
}

func (s *Service) Get(ctx context.Context, alias string) (Link, error) {
	if err := validateAlias(alias); err != nil {
		return Link{}, err
	}

	link, err := s.store.GetLink(ctx, alias)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Link{}, ErrNotFound
		}
		return Link{}, err
	}
	if isExpired(link.ExpiresAt, s.currentTime()) {
		s.cache.Delete(alias)
		return Link{}, ErrExpired
	}

	return toOutput(link, s.baseURL), nil
}

func (s *Service) Resolve(ctx context.Context, alias string) (string, error) {
	if err := validateAlias(alias); err != nil {
		return "", err
	}

	now := s.currentTime()
	if cachedURL, ok := s.cache.Get(alias, now); ok {
		s.incrementAccessCount(alias)
		return cachedURL, nil
	}

	link, err := s.store.GetLink(ctx, alias)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if isExpired(link.ExpiresAt, now) {
		s.cache.Delete(alias)
		return "", ErrExpired
	}

	s.incrementAccessCount(alias)
	s.cache.Set(alias, link.OriginalURL, link.ExpiresAt, now)
	return link.OriginalURL, nil
}

func (s *Service) createWithAlias(ctx context.Context, alias, originalURL string, expiresAt *time.Time, createdAt time.Time) (Link, error) {
	stored, err := s.store.CreateLink(ctx, store.Link{
		Alias:       alias,
		OriginalURL: originalURL,
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return Link{}, err
	}

	return toOutput(stored, s.baseURL), nil
}

func toOutput(link store.Link, baseURL string) Link {
	return Link{
		Alias:       link.Alias,
		OriginalURL: link.OriginalURL,
		ShortURL:    baseURL + "/" + link.Alias,
		CreatedAt:   link.CreatedAt,
		ExpiresAt:   link.ExpiresAt,
		AccessCount: link.AccessCount,
	}
}

func normalizeURL(raw string) (string, error) {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidURL
	}
	return parsed.String(), nil
}

func normalizeBaseURL(raw string) (string, error) {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidBaseURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidBaseURL
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", ErrInvalidBaseURL
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateAlias(alias string) error {
	if len(alias) < 3 || len(alias) > 32 {
		return ErrInvalidAlias
	}
	for _, r := range alias {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return ErrInvalidAlias
	}
	if _, reserved := reservedAliases[alias]; reserved {
		return ErrInvalidAlias
	}
	return nil
}

func isExpired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !now.Before(*expiresAt)
}

func (s *Service) currentTime() time.Time {
	s.mu.RLock()
	now := s.now
	s.mu.RUnlock()
	return now().UTC()
}

func (s *Service) incrementAccessCount(alias string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.store.IncrementAccessCount(ctx, alias)
	}()
}
