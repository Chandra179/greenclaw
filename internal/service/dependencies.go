package service

import (
	"greenclaw/internal/config"
	"greenclaw/pkg/storage"
	"greenclaw/pkg/transcribe"
	"greenclaw/pkg/youtube"
)

type Dependencies struct {
	YtClient         youtube.Client
	TranscribeClient transcribe.Client
	Storage          *storage.Client
	Cfg              config.Config
}
