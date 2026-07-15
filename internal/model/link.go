package model

import "time"

type Link struct {
	OriginalURL    string
	ShortCode      string
	CreatedAt      time.Time
	LastAccessedAt *time.Time
	AccessCount    int64
}
