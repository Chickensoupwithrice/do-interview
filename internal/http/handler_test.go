package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	stdhttp "net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/example/url-shortener/internal/shortener"
	"github.com/example/url-shortener/internal/store"
)

func TestRedirectAndMetadata(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestService(t)
	defer cleanup()
	handler := NewHandler(service)

	body, _ := json.Marshal(map[string]any{
		"url":   "https://example.com/landing",
		"alias": "launchme",
	})
	req := httptest.NewRequest(stdhttp.MethodPost, "/api/urls", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}

	redirectReq := httptest.NewRequest(stdhttp.MethodGet, "/launchme", nil)
	redirectResp := httptest.NewRecorder()
	handler.ServeHTTP(redirectResp, redirectReq)

	if redirectResp.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", redirectResp.Code)
	}
	if got := redirectResp.Header().Get("Location"); got != "https://example.com/landing" {
		t.Fatalf("expected redirect location, got %q", got)
	}

	metadataReq := httptest.NewRequest(stdhttp.MethodGet, "/api/urls/launchme", nil)
	var payload shortener.Link
	deadline := time.Now().Add(2 * time.Second)
	for {
		metadataResp := httptest.NewRecorder()
		handler.ServeHTTP(metadataResp, metadataReq)
		if metadataResp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", metadataResp.Code)
		}
		if err := json.NewDecoder(metadataResp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode metadata: %v", err)
		}
		if payload.AccessCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected access count 1, got %d", payload.AccessCount)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestExpiredRedirectReturnsGone(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestService(t)
	defer cleanup()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	service.SetNow(func() time.Time { return now })
	handler := NewHandler(service)

	ttl := int64(1)
	_, err := service.Create(httptest.NewRequest(stdhttp.MethodGet, "/", nil).Context(), shortener.CreateInput{
		URL:        "https://example.com/soon-gone",
		Alias:      "gone123",
		TTLSeconds: &ttl,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	service.SetNow(func() time.Time { return now.Add(2 * time.Second) })
	req := httptest.NewRequest(stdhttp.MethodGet, "/gone123", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d", resp.Code)
	}
}

func TestCreateRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestService(t)
	defer cleanup()
	handler := NewHandler(service)

	req := httptest.NewRequest(stdhttp.MethodPost, "/api/urls", strings.NewReader(`{"url":"https://example.com","ttl_secondz":10}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestCreateRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestService(t)
	defer cleanup()
	handler := NewHandler(service)

	req := httptest.NewRequest(stdhttp.MethodPost, "/api/urls", strings.NewReader(`{"url":"https://example.com"} {"url":"https://evil.com"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestCreateRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestService(t)
	defer cleanup()
	handler := NewHandler(service)

	largeURL := "https://example.com/" + strings.Repeat("a", maxCreateBodyBytes)
	body := `{"url":"` + largeURL + `"}`
	req := httptest.NewRequest(stdhttp.MethodPost, "/api/urls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestHealthReturnsUnavailableWhenStoreIsClosed(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service, err := shortener.NewService(sqliteStore, shortener.NewCache(), "http://localhost:8080")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	handler := NewHandler(service)
	if err := sqliteStore.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	req := httptest.NewRequest(stdhttp.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestMetadataInvalidPathsReturnNotFound(t *testing.T) {
	t.Parallel()

	service, cleanup := newTestService(t)
	defer cleanup()
	handler := NewHandler(service)

	for _, path := range []string{"/api/urls/", "/api/urls/foo/bar"} {
		req := httptest.NewRequest(stdhttp.MethodGet, path, nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("path %q: expected 404, got %d", path, resp.Code)
		}
	}
}

func newTestService(t *testing.T) (*shortener.Service, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service, err := shortener.NewService(sqliteStore, shortener.NewCache(), "http://localhost:8080")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service, func() { _ = sqliteStore.Close() }
}
