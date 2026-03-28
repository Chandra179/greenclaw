package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"greenclaw/internal/llm"
	"greenclaw/internal/pipeline"
	"greenclaw/internal/router"
)

var validStyles = map[string]bool{
	string(llm.StyleSummary):   true,
	string(llm.StyleTakeaways): true,
}

const resultsDir = "results"

// randomHex returns a short random hex string (4 bytes = 8 hex chars).
func randomHex() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

// saveResult writes data as indented JSON to results/{videoID}_{model}_{numCtx}_{rand}.json.
// Only saves for YouTube URLs; logs and skips silently for others.
func saveResult(url, model string, numCtx int, data json.RawMessage) {
	_, id, ok := router.IsYouTube(url)
	if !ok {
		return
	}
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		log.Printf("[results] mkdir: %v", err)
		return
	}
	safe := strings.ReplaceAll(model, ":", "-")
	name := fmt.Sprintf("%s_%s_%d_%s.json", id, safe, numCtx, randomHex())
	out, _ := json.MarshalIndent(data, "", "  ")
	if err := os.WriteFile(filepath.Join(resultsDir, name), out, 0644); err != nil {
		log.Printf("[results] write %s: %v", name, err)
		return
	}
	log.Printf("[results] saved %s", name)
}

type extractRequest struct {
	URL    string   `json:"url" binding:"required" example:"https://www.youtube.com/watch?v=dQw4w9WgXcQ"`
	NumCtx int      `json:"num_ctx,omitempty" example:"8192"`
	Styles []string `json:"styles,omitempty" example:"summary,takeaways"`
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
func handleExtract(s *pipeline.Pipeline, model string, defaultNumCtx int) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req extractRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "url is required"})
			return
		}

		styles, err := parseStyles(req.Styles)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		result, err := s.ProcessSingle(c.Request.Context(), req.URL, nil, req.NumCtx, styles)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		effectiveNumCtx := req.NumCtx
		if effectiveNumCtx == 0 {
			effectiveNumCtx = defaultNumCtx
		}
		result.Model = model
		result.NumCtx = effectiveNumCtx
		result.Styles = req.Styles

		c.JSON(http.StatusOK, result)

		if data, err := json.Marshal(result); err == nil {
			saveResult(req.URL, model, effectiveNumCtx, data)
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
func handleExtractStream(s *pipeline.Pipeline, model string, defaultNumCtx int) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req extractRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "url is required"})
			return
		}

		styles, err := parseStyles(req.Styles)
		if err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		progressCh := make(chan llm.ProgressEvent, 32)

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no") // disable nginx proxy buffering

		resultCh := make(chan json.RawMessage, 1)
		errCh := make(chan error, 1)

		effectiveNumCtx := req.NumCtx
		if effectiveNumCtx == 0 {
			effectiveNumCtx = defaultNumCtx
		}

		go func() {
			defer close(progressCh)
			r, err := s.ProcessSingle(c.Request.Context(), req.URL, progressCh, req.NumCtx, styles)
			if err != nil {
				errCh <- err
				return
			}
			r.Model = model
			r.NumCtx = effectiveNumCtx
			r.Styles = req.Styles
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
			saveResult(req.URL, model, effectiveNumCtx, data)
		case err := <-errCh:
			data, _ := json.Marshal(errorResponse{Error: err.Error()})
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", data)
		}

		if flusher != nil {
			flusher.Flush()
		}
	}
}

// parseStyles converts request style strings to typed ProcessingStyle values.
// Returns an error if any value is not a recognised style.
func parseStyles(raw []string) ([]llm.ProcessingStyle, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]llm.ProcessingStyle, 0, len(raw))
	for _, s := range raw {
		if !validStyles[s] {
			return nil, fmt.Errorf("unknown style %q: valid values are \"summary\", \"takeaways\"", s)
		}
		out = append(out, llm.ProcessingStyle(s))
	}
	return out, nil
}
