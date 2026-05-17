package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type VideoRecord struct {
	VideoID    string    `json:"video_id"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Transcript string    `json:"transcript"`
	Language   string    `json:"language"`
	Duration   string    `json:"duration"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Client struct {
	db *sql.DB
}

func NewClient(dsn string) (*Client, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	c := &Client{db: db}
	if err := c.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return c, nil
}

func (c *Client) migrate() error {
	_, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS videos (
			video_id    TEXT PRIMARY KEY,
			url         TEXT NOT NULL DEFAULT '',
			title       TEXT NOT NULL DEFAULT '',
			transcript  TEXT NOT NULL DEFAULT '',
			language    TEXT NOT NULL DEFAULT '',
			duration    TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	return err
}

func (c *Client) StoreVideo(ctx context.Context, v VideoRecord) error {
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO videos (video_id, url, title, transcript, language, duration, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(video_id) DO UPDATE SET
			url        = excluded.url,
			title      = excluded.title,
			transcript = excluded.transcript,
			language   = excluded.language,
			duration   = excluded.duration,
			updated_at = datetime('now')
	`, v.VideoID, v.URL, v.Title, v.Transcript, v.Language, v.Duration)
	return err
}

func (c *Client) GetVideo(ctx context.Context, videoID string) (*VideoRecord, error) {
	var v VideoRecord
	var createdAt, updatedAt string
	err := c.db.QueryRowContext(ctx, `
		SELECT video_id, url, title, transcript, language, duration, created_at, updated_at
		FROM videos WHERE video_id = ?
	`, videoID).Scan(&v.VideoID, &v.URL, &v.Title, &v.Transcript, &v.Language, &v.Duration, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	v.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &v, nil
}

func (c *Client) Close() error {
	return c.db.Close()
}
