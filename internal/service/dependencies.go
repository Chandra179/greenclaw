package service

import (
	"greenclaw/pkg/graphdb"
	"greenclaw/pkg/transcribe"
	"greenclaw/pkg/youtube"
)

type Dependencies struct {
	YtClient         youtube.Client
	GraphDB          graphdb.Client
	TranscribeClient transcribe.Client
}
