package models

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"time"
)

type MediaType int

const (
	Image MediaType = iota + 1
	Audio
	Video
	File
)

type Media struct {
	ID         string `json:"-" db:"id"`
	URL        URL    `json:"url" db:"url"`
	PreviewURL *URL   `json:"preview_url" db:"preview_url"`

	Placeholder *string   `json:"placeholder,omitempty" db:"placeholder"`
	Type        MediaType `json:"type" db:"type"`

	// indicate the url to connect when user click the media
	RedirectURL *URL    `json:"redirect_url,omitempty" db:"redirect_url"`
	Title       *string `json:"title,omitempty" db:"title"`

	Size      *string    `json:"size,omitempty" db:"size"`
	ExpiredAt *time.Time `json:"expired_at,omitempty" db:"expired_at"`
}

type MediaUpload struct {
	MediaType   MediaType  `json:"-"`
	URL         string     `json:"-"`
	PreviewURL  *string    `json:"-"`
	Placeholder *string    `json:"-"`
	RedirectURL *string    `json:"-"`
	Title       *string    `json:"-"`
	Size        *string    `json:"-"`
	ExpiredAt   *time.Time `json:"-"`
}

type URL string

func (u URL) MarshalJSON() ([]byte, error) {
	urlStr := string(u)
	if urlStr == "" {
		return json.Marshal("")
	}

	dir, filename := filepath.Split(urlStr)

	escapedFilename := url.PathEscape(filename)

	result := dir + escapedFilename

	return json.Marshal(result)
}
