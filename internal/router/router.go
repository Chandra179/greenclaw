package router

import (
	"greenclaw/internal/service"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type Dependencies struct {
	OrchDeps service.Dependencies
}

type Handler struct {
	deps Dependencies
}

func Router(deps Dependencies) *gin.Engine {
	r := gin.Default()

	h := &Handler{deps: deps}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: false,
	}))

	r.POST("/extract/youtube", h.handleExtractYoutube)
	r.POST("/extract/graph", h.handleExtractGraph)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

func (h *Handler) handleExtractYoutube(c *gin.Context) {
	// Example: h.deps.OrchDeps.SomeService.DoWork()
	c.JSON(200, gin.H{"status": "ok", "message": "youtube extraction logic"})
}

func (h *Handler) handleExtractGraph(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok", "message": "graph extraction logic"})
}
