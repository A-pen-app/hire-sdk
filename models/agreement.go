package models

import "time"

type AgreementRecord struct {
	VersionAgreed *string    `json:"version_agreed" binding:"required,min=1" db:"version_agreed"`
	AgreedAt      *time.Time `json:"agreed_at,omitempty" db:"agreed_at"`
	VersionLatest string     `json:"version_latest" db:"eula"`
}
