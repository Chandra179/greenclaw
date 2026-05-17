package router

import (
	"greenclaw/internal/service"
	"net/http"

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
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

// handleExtractYoutube extracts a transcript from a YouTube video and stores it.
//
// @Summary     Extract YouTube transcript
// @Description Given a YouTube URL, fetches (or transcribes) the transcript and stores it in SQLite.
// @Tags        extract
// @Accept      json
// @Produce     json
// @Param       body body service.ExtractYoutubeReq true "YouTube URLs"
// @Success     200 {array} service.ExtractYoutubeResp
// @Failure     400 {object} map[string]string
// @Failure     500 {object} map[string]string
// @Router      /extract/youtube [post]
func (h *Handler) handleExtractYoutube(c *gin.Context) {
	var req service.ExtractYoutubeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.YoutubeURLs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "youtube_urls is required"})
		return
	}

	resp, err := h.deps.OrchDeps.ExtractYoutube(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
