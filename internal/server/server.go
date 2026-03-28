package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"greenclaw/internal/config"
	"greenclaw/internal/pipeline"
)

// Server holds the HTTP server and its dependencies.
type Server struct {
	port     int
	pipeline *pipeline.Pipeline
	router   *gin.Engine
}

func New(cfg config.Config) *Server {
	s := pipeline.New(cfg)

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: false,
	}))

	r.POST("/extract", handleExtract(s, cfg.LLM.Model, cfg.LLM.NumCtx))
	r.POST("/extract/stream", handleExtractStream(s, cfg.LLM.Model, cfg.LLM.NumCtx))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return &Server{
		port:     cfg.Port,
		pipeline: s,
		router:   r,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) Run() error {
	defer s.pipeline.Close()

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("listening on %s", addr)
	log.Printf("swagger docs at http://localhost%s/swagger/index.html", addr)
	return s.router.Run(addr)
}
