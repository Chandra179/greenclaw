package transcribe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// HTTPClient sends audio to a remote Whisper service over HTTP.
type HTTPClient struct {
	endpoint   string
	httpClient *http.Client
	language   string
}

// NewHTTPClient creates a client that calls a remote Whisper HTTP service.
func NewHTTPClient(endpoint string, timeout time.Duration, language string) *HTTPClient {
	return &HTTPClient{
		endpoint:   endpoint,
		httpClient: &http.Client{Timeout: timeout},
		language:   language,
	}
}

// Transcribe uploads the audio file to the remote service and returns the result.
func (h *HTTPClient) Transcribe(ctx context.Context, audioPath string) (*Result, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("opening audio file: %w", err)
	}
	defer f.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()

		part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			pw.CloseWithError(err)
			return
		}
		if h.language != "" {
			writer.WriteField("language", h.language)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint+"/transcribe", pr)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transcription request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("transcription service returned %d: %s", resp.StatusCode, body)
	}

	var result Result
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding transcription response: %w", err)
	}

	return &result, nil
}
