package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"greenclaw/internal/config"
	"greenclaw/scraper"
)

type extractRequest struct {
	URL string `json:"url"`
}

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	s := scraper.New(cfg)
	defer s.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /extract", handleExtract(s))

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %v, shutting down...", sig)
		srv.Shutdown(context.Background())
	}()

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func handleExtract(s *scraper.Scraper) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req extractRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
			return
		}
		if req.URL == "" {
			http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
			return
		}

		log.Printf("[extract] %s", req.URL)
		rs := s.Run(r.Context(), []string{req.URL})

		results := rs.All()
		w.Header().Set("Content-Type", "application/json")
		if len(results) == 1 {
			json.NewEncoder(w).Encode(results[0])
		} else {
			json.NewEncoder(w).Encode(results)
		}
	}
}
