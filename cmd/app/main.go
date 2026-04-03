// @title           Greenclaw API
// @version         1.0
// @description     Detect-first, escalate-later web scraper with LLM processing.
// @host            localhost:8080
// @BasePath        /

package main

import (
	"fmt"
	"log"

	"greenclaw/cmd/app/docs"
	"greenclaw/internal"
	"greenclaw/internal/config"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("error loading config: %v", err)
	}

	docs.SwaggerInfo.Host = fmt.Sprintf("localhost:%d", cfg.Port)

	if err := internal.NewServer(cfg).Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
