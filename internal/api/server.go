package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/simonbalfe/k3s-hetzner-adk-research/internal/jobs"
)

const maxRequestBytes = 32 << 10

type jobStore interface {
	Enqueue(context.Context, string) (jobs.Job, error)
	Find(context.Context, string) (jobs.Job, error)
}

type Server struct {
	jobs   jobStore
	token  string
	logger *slog.Logger
}

func New(store jobStore, token string, logger *slog.Logger) (*Server, error) {
	if store == nil {
		return nil, errors.New("job store is required")
	}
	if token == "" {
		return nil, errors.New("API token is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	return &Server{jobs: store, token: token, logger: logger}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("POST /v1/research", s.authenticate(http.HandlerFunc(s.create)))
	mux.Handle("GET /v1/research/{id}", s.authenticate(http.HandlerFunc(s.find)))
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	defer r.Body.Close()

	var input struct {
		Query string `json:"query"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	input.Query = strings.TrimSpace(input.Query)
	if len(input.Query) < 3 || len(input.Query) > 2000 {
		writeError(w, http.StatusBadRequest, "query must contain between 3 and 2000 characters")
		return
	}

	job, err := s.jobs.Enqueue(r.Context(), input.Query)
	if err != nil {
		s.logger.Error("enqueue research job", "error", err)
		writeError(w, http.StatusServiceUnavailable, "research queue is unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) find(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if len(id) != 32 {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	job, err := s.jobs.Find(r.Context(), id)
	if errors.Is(err, jobs.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		s.logger.Error("read research job", "job_id", id, "error", err)
		writeError(w, http.StatusServiceUnavailable, "job store is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) || header[len(prefix):] != s.token {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
