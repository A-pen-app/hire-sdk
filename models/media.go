package models

import "time"

type MediaType int

const (
	Image MediaType = iota + 1
	Audio
	Video
	File
)

type Media struct {
	ID         string  `json:"-" db:"id"`
	URL        string  `json:"url" db:"url"`
	PreviewURL *string `json:"preview_url" db:"preview_url"`

	Placeholder *string   `json:"placeholder,omitempty" db:"placeholder"`
	Type        MediaType `json:"type" db:"type"`

	// indicate the url to connect when user click the media
	RedirectURL *string `json:"redirect_url,omitempty" db:"redirect_url"`
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
