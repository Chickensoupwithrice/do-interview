package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/example/url-shortener/internal/shortener"
)

type Handler struct {
	service *shortener.Service
}

type createRequest struct {
	URL        string `json:"url"`
	Alias      string `json:"alias,omitempty"`
	TTLSeconds *int64 `json:"ttl_seconds,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

const maxCreateBodyBytes = 1 << 20

func NewHandler(service *shortener.Service) http.Handler {
	h := &Handler{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealth)
	mux.HandleFunc("POST /api/urls", h.handleCreate)
	mux.HandleFunc("GET /api/urls/{$}", http.NotFound)
	mux.HandleFunc("GET /api/urls/{alias}", h.handleMetadata)
	mux.HandleFunc("GET /{$}", h.handleRoot)
	mux.HandleFunc("GET /{alias}", h.handleRedirect)
	return mux
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Health(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "unhealthy"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateBodyBytes)
	defer r.Body.Close()

	var req createRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json body"})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json body"})
		return
	}

	link, err := h.service.Create(r.Context(), shortener.CreateInput{
		URL:        req.URL,
		Alias:      req.Alias,
		TTLSeconds: req.TTLSeconds,
	})
	if err != nil {
		h.writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, link)
}

func (h *Handler) handleMetadata(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")
	link, err := h.service.Get(r.Context(), alias)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, link)
}

func (h *Handler) handleRedirect(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")
	url, err := h.service.Resolve(r.Context(), alias)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"name": "url-shortener",
		"docs": "POST /api/urls, GET /api/urls/{alias}, GET /{alias}",
	})
}

func (h *Handler) writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shortener.ErrInvalidURL), errors.Is(err, shortener.ErrInvalidAlias), errors.Is(err, shortener.ErrInvalidTTL):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, shortener.ErrAliasConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, shortener.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, shortener.ErrExpired):
		writeJSON(w, http.StatusGone, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
