package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"greenclaw/internal/config"
	"greenclaw/internal/scraper"
)

func main() {
	cfg := config.Default()

	flag.IntVar(&cfg.HTTPConcurrency, "concurrency", cfg.HTTPConcurrency, "max concurrent HTTP requests")
	flag.IntVar(&cfg.BrowserConcurrency, "browser-concurrency", cfg.BrowserConcurrency, "max concurrent browser sessions")
	flag.StringVar(&cfg.OutputFormat, "output", cfg.OutputFormat, "output format: text or json")
	flag.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "request timeout")
	urlsFile := flag.String("urls-file", "", "file containing URLs (one per line)")
	ytAudio := flag.Bool("youtube-audio", false, "download audio from YouTube videos")
	ytSubtitles := flag.String("youtube-subtitles", "", "export YouTube subtitles in given formats (comma-separated: srt,vtt,ttml)")
	ytLangs := flag.String("youtube-langs", "", "YouTube transcript/subtitle languages (comma-separated, empty = all)")
	flag.Parse()

	// Apply YouTube CLI flags
	if *ytAudio {
		cfg.YouTube.DownloadAudio = true
	}
	if *ytSubtitles != "" {
		cfg.YouTube.ExportSubtitles = true
		cfg.YouTube.SubtitleFormats = strings.Split(*ytSubtitles, ",")
	}
	if *ytLangs != "" {
		cfg.YouTube.TranscriptLangs = strings.Split(*ytLangs, ",")
	}

	urls := flag.Args()

	// Load URLs from file if provided
	if *urlsFile != "" {
		fileURLs, err := loadURLsFromFile(*urlsFile)
		if err != nil {
			log.Fatalf("error reading urls file: %v", err)
		}
		urls = append(urls, fileURLs...)
	}

	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "usage: greenclaw [flags] <url>...")
		fmt.Fprintln(os.Stderr, "       greenclaw --urls-file urls.txt")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Setup context with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received %v, shutting down...", sig)
		cancel()
	}()

	log.Printf("processing %d URLs (http=%d, browser=%d concurrency)",
		len(urls), cfg.HTTPConcurrency, cfg.BrowserConcurrency)

	s := scraper.New(cfg)
	defer s.Close()

	rs := s.Run(ctx, urls)

	// Output results
	results := rs.All()
	switch cfg.OutputFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
	default:
		for _, r := range results {
			printResult(r)
		}
	}

	log.Printf("done: %d results", rs.Count())
}

func printResult(r any) {
	// Type assert to access fields
	type result struct {
		URL         string    `json:"url"`
		ContentType string    `json:"content_type"`
		Title       string    `json:"title,omitempty"`
		Description string    `json:"description,omitempty"`
		Text        string    `json:"text,omitempty"`
		Links       []string  `json:"links,omitempty"`
		FilePath    string    `json:"file_path,omitempty"`
		RawData     any       `json:"raw_data,omitempty"`
		Stage       int       `json:"stage"`
		Error       string    `json:"error,omitempty"`
		FetchedAt   time.Time `json:"fetched_at"`
	}

	// Marshal and unmarshal to get a generic map for printing
	b, _ := json.Marshal(r)
	var m map[string]any
	json.Unmarshal(b, &m)

	fmt.Println(strings.Repeat("─", 60))
	if url, ok := m["url"]; ok {
		fmt.Printf("URL:   %v\n", url)
	}
	if e, ok := m["error"]; ok && e != "" {
		fmt.Printf("Error: %v\n", e)
		return
	}
	if ct, ok := m["content_type"]; ok && ct != "" {
		fmt.Printf("Type:  %v\n", ct)
	}
	if stage, ok := m["stage"]; ok {
		fmt.Printf("Stage: %v\n", stage)
	}
	if t, ok := m["title"]; ok && t != "" {
		fmt.Printf("Title: %v\n", t)
	}
	if d, ok := m["description"]; ok && d != "" {
		fmt.Printf("Desc:  %v\n", d)
	}
	if fp, ok := m["file_path"]; ok && fp != "" {
		fmt.Printf("File:  %v\n", fp)
	}
	if text, ok := m["text"].(string); ok && text != "" {
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		fmt.Printf("Text:  %s\n", text)
	}
	if links, ok := m["links"].([]any); ok && len(links) > 0 {
		fmt.Printf("Links: %d found\n", len(links))
	}
}

func loadURLsFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var urls []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			urls = append(urls, line)
		}
	}
	return urls, scanner.Err()
}
