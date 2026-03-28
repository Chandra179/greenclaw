package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"greenclaw/internal/llm"
	"greenclaw/internal/pipeline"
)

type extractRequest struct {
	URL string `json:"url" binding:"required" example:"https://www.youtube.com/watch?v=dQw4w9WgXcQ"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// handleExtract godoc
// @Summary      Extract content from a URL
// @Description  Scrapes the given URL and returns structured content. For YouTube URLs, also returns transcript and LLM processing results.
// @Tags         extract
// @Accept       json
// @Produce      json
// @Param        body  body      extractRequest  true  "URL to extract"
// @Success      200   {object}  store.Result
// @Failure      400   {object}  errorResponse
// @Router       /extract [post]
func handleExtract(s *pipeline.Pipeline) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req extractRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "url is required"})
			return
		}

		rs := s.Run(c.Request.Context(), []string{req.URL})
		results := rs.All()

		if len(results) == 1 {
			c.JSON(http.StatusOK, results[0])
		} else {
			c.JSON(http.StatusOK, results)
		}
	}
}

// handleExtractStream godoc
// @Summary      Extract content from a URL with SSE progress streaming
// @Description  Same as /extract but streams chunk-level progress events via Server-Sent Events while the LLM processes the transcript. Final result is sent as an "result" event.
// @Tags         extract
// @Accept       json
// @Produce      text/event-stream
// @Param        body  body      extractRequest  true  "URL to extract"
// @Success      200   {object}  store.Result
// @Failure      400   {object}  errorResponse
// @Router       /extract/stream [post]
func handleExtractStream(s *pipeline.Pipeline) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req extractRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "url is required"})
			return
		}

		progressCh := make(chan llm.ProgressEvent, 32)

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no") // disable nginx proxy buffering

		resultCh := make(chan json.RawMessage, 1)
		errCh := make(chan error, 1)

		go func() {
			defer close(progressCh)
			r, err := s.ProcessSingle(c.Request.Context(), req.URL, progressCh)
			if err != nil {
				errCh <- err
				return
			}
			data, _ := json.Marshal(r)
			resultCh <- data
		}()

		flusher, _ := c.Writer.(http.Flusher)

		for ev := range progressCh {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(c.Writer, "event: progress\ndata: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}

		select {
		case data := <-resultCh:
			fmt.Fprintf(c.Writer, "event: result\ndata: %s\n\n", data)
		case err := <-errCh:
			data, _ := json.Marshal(errorResponse{Error: err.Error()})
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", data)
		}

		if flusher != nil {
			flusher.Flush()
		}
	}
}
