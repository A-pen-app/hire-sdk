package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// HireContact represents quick contact info for hire posts and chat rooms
type HireContact struct {
	Email         *string `json:"email,omitempty"`
	Phone         *string `json:"phone,omitempty"`
	LineAccountID *string `json:"line_account_id,omitempty"`
}

// Value implements the driver.Valuer interface for inserting as jsonb
func (h HireContact) Value() (driver.Value, error) {
	return json.Marshal(h)
}

// Scan implements the sql.Scanner interface for reading jsonb
func (h *HireContact) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, h)
}
