package internal

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"greenclaw/internal/constant"
	"greenclaw/internal/llm"
	"greenclaw/internal/pipeline"
	"greenclaw/internal/store"
)

var validStyles = map[string]bool{
	string(llm.StyleSummary):   true,
	string(llm.StyleTakeaways): true,
}

const resultsDir = "results"

func randomHex() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

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

type httpResult struct {
	URL          string                   `json:"url"`
	ContentType  constant.HTTPContentType `json:"content_type"`
	VideoID      string                   `json:"video_id,omitempty"`
	Title        string                   `json:"title,omitempty"`
	Description  string                   `json:"description,omitempty"`
	Duration     string                   `json:"duration,omitempty"`
	ChannelName  string                   `json:"channel_name,omitempty"`
	LanguageCode string                   `json:"language_code,omitempty"`
	Style        string                   `json:"style,omitempty"`
	Content      []string                 `json:"content,omitempty"`
	Links        []string                 `json:"links,omitempty"`
	Error        string                   `json:"error,omitempty"`
	FetchedAt    any                      `json:"fetched_at"`
	Model        string                   `json:"model,omitempty"`
	NumCtx       int                      `json:"num_ctx,omitempty"`
	Styles       []string                 `json:"styles,omitempty"`
}

func toHTTPResult(r *store.Result) httpResult {
	h := httpResult{
		URL:         r.URL,
		Title:       r.Title,
		Description: r.Description,
		Links:       r.Links,
		Error:       r.Error,
		FetchedAt:   r.FetchedAt,
		Model:       r.Model,
		NumCtx:      r.NumCtx,
		Styles:      r.Styles,
	}
	if r.YouTube != nil {
		h.VideoID = r.YouTube.VideoID
		h.Duration = r.YouTube.Duration
		h.ChannelName = r.YouTube.ChannelName
		if len(r.YouTube.Captions) > 0 {
			h.LanguageCode = r.YouTube.Captions[0].LanguageCode
		}
		if len(r.YouTube.Processing) > 0 {
			h.Style = r.YouTube.Processing[0].Style
			h.Content = r.YouTube.Processing[0].Content
		}
	}
	return h
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
// @Description  Scrapes the given URL and returns structured content.
// @Tags         extract
// @Accept       json
// @Produce      json
// @Param        body  body      extractRequest  true  "URL to extract"
// @Success      200   {object}  httpResult
// @Failure      400   {object}  errorResponse
// @Router       /extract [post]
func handleExtract(p *pipeline.Pipeline, model string, defaultNumCtx int) gin.HandlerFunc {
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

		result, err := p.ProcessSingle(c.Request.Context(), req.URL, nil, req.NumCtx, styles)
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

		c.JSON(http.StatusOK, toHTTPResult(result))
	}
}

type graphRequest struct {
	URL string `json:"url" binding:"required" example:"https://www.youtube.com/watch?v=dQw4w9WgXcQ"`
}

// handleExtractGraph godoc
// @Summary      Populate knowledge graph from a previously extracted URL
// @Description  Runs entity extraction and ArangoDB graph population for a result that was previously processed by /extract or /extract/stream.
// @Tags         extract
// @Accept       json
// @Produce      json
// @Param        body  body      graphRequest  true  "URL to index"
// @Success      200   {object}  httpResult
// @Failure      400   {object}  errorResponse
// @Failure      404   {object}  errorResponse
// @Router       /extract/graph [post]
func handleExtractGraph(p *pipeline.Pipeline) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req graphRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "url is required"})
			return
		}

		p.IndexResult(c.Request.Context(), req.URL)
		c.Status(http.StatusOK)
	}
}
