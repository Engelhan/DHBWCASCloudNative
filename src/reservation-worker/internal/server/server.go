// Package server exposes the /metrics, /healthz and /readyz HTTP endpoints.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Pinger is satisfied by both *db.DB and *producer.Producer.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	httpServer *http.Server
}

// New builds the server. /readyz actively checks DB and Kafka reachability
// on every request (mirrors catalog-api's readyz, see
// src/catalog-api/internal/api ## handleReadyz) -- there's no cheap
// always-true default here since, unlike notification-worker, this service
// has no long stretches where "no traffic yet" is expected/normal.
func New(addr string, db, kafka Pinger) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		handleReadyz(w, r, db, kafka)
	})

	return &Server{httpServer: &http.Server{Addr: addr, Handler: mux}}
}

func handleReadyz(w http.ResponseWriter, r *http.Request, db, kafka Pinger) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbErr := db.Ping(ctx)
	kafkaErr := kafka.Ping(ctx)

	ready := dbErr == nil && kafkaErr == nil
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, map[string]any{
		"status": readyString(ready),
		"checks": map[string]bool{
			"database": dbErr == nil,
			"kafka":    kafkaErr == nil,
		},
	})
}

func readyString(ready bool) string {
	if ready {
		return "ready"
	}
	return "not ready"
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) Run() error {
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
