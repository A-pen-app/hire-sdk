package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// BusinessCardContent represents the JSONB content of a business card
type BusinessCardContent struct {
	RealName    *string  `json:"real_name,omitempty"`
	Position    *string  `json:"position,omitempty"`
	Departments []string `json:"departments,omitempty"`
}

// Value implements the driver.Valuer interface for inserting as jsonb
func (b BusinessCardContent) Value() (driver.Value, error) {
	return json.Marshal(b)
}

// Scan implements the sql.Scanner interface for reading jsonb
func (b *BusinessCardContent) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, b)
}

// BusinessCard represents the latest version of a user's business card
type BusinessCard struct {
	ID        string               `json:"-" db:"id"`
	AppID     string               `json:"-" db:"app_id"`
	UserID    string               `json:"-" db:"user_id"`
	Content   *BusinessCardContent `json:"content" db:"content"`
	CreatedAt time.Time            `json:"-" db:"created_at"`
	UpdatedAt time.Time            `json:"-" db:"updated_at"`
}

// BusinessCardSnapshot represents a snapshot of a business card at a point in time
type BusinessCardSnapshot struct {
	ID             string               `json:"id" db:"id"`
	BusinessCardID string               `json:"-" db:"business_card_id"`
	Content        *BusinessCardContent `json:"content" db:"content"`
	CreatedAt      time.Time            `json:"-" db:"created_at"`
}
