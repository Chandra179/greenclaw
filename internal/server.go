package internal

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"greenclaw/internal/config"
	"greenclaw/internal/router"
	"greenclaw/internal/service"
	"greenclaw/pkg/storage"
	"greenclaw/pkg/transcribe"
	"greenclaw/pkg/youtube"
)

type Server struct {
	port   int
	router *gin.Engine
}

func NewServer(cfg config.Config) *Server {
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	ytClient := youtube.New(httpClient)

	orchDeps := service.Dependencies{
		YtClient: ytClient,
		Cfg:      cfg,
	}

	// Wire up SQLite storage.
	if cfg.Storage.DSN != "" {
		s, err := storage.NewClient(cfg.Storage.DSN)
		if err != nil {
			log.Printf("[server] warn: storage unavailable: %v", err)
		} else {
			orchDeps.Storage = s
		}
	}

	// Wire up optional transcribe client.
	if cfg.YouTube.DownloadAudio && cfg.YouTube.TranscribeAudio && cfg.Transcriber.Endpoint != "" {
		timeout, err := time.ParseDuration(cfg.Transcriber.Timeout)
		if err != nil {
			timeout = 5 * time.Minute
		}
		orchDeps.TranscribeClient = transcribe.NewHTTPClient(
			cfg.Transcriber.Endpoint,
			timeout,
			cfg.Transcriber.Language,
		)
	}

	routerDeps := router.Dependencies{
		OrchDeps: orchDeps,
	}

	r := router.Router(routerDeps)

	return &Server{
		port:   cfg.Port,
		router: r,
	}
}

func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("listening on %s", addr)
	log.Printf("swagger docs at http://localhost%s/swagger/index.html", addr)
	return s.router.Run(addr)
}
