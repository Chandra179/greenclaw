package service

import (
	"greenclaw/internal/config"
	"greenclaw/pkg/graphdb"
	"greenclaw/pkg/llm"
	"greenclaw/pkg/transcribe"
	"greenclaw/pkg/youtube"
)

type Dependencies struct {
	YtClient         youtube.Client
	GraphDB          graphdb.Client
	TranscribeClient transcribe.Client
	LLMClient        *llm.OllamaClient
	Cfg              config.Config
}
