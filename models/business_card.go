package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// BusinessCardContent represents the JSONB content of a business card
type BusinessCardContent struct {
	// common
	RealName *string `json:"real_name,omitempty"`

	// for doctor
	Position    *string  `json:"position,omitempty"`
	Departments []string `json:"departments,omitempty"`

	// for pharmacist and nurse
	CurrentOrganization *string `json:"current_organization,omitempty"`
	CurrentJobTitle     *string `json:"current_job_title,omitempty"`

	// YearOfExperience is apen-only, mirroring user.year_of_experience in apen's
	// main DB with its sentinel encoding: 0 = less than 1 year (also
	// no-experience and students), 21 = more than 20 years. Never render the
	// raw value — decode via FormatExperienceYears.
	//
	// ResumeContent.YearOfExperience carries the SAME business concept under the
	// same name and jsonb key, but in the legacy 12-step encoding
	// (YearOfExperienceType, 0..11). The Go type is what tells them apart: *int
	// here, YearOfExperienceType there. Never assign one to the other without
	// converting — the two ranges overlap, so a mix-up is silent.
	YearOfExperience *int `json:"year_of_experience,omitempty"`

	// YearOfExperienceUpdatedAt stamps when YearOfExperience was last set. The
	// card stores a BASELINE; the value shown to users auto-increments from this
	// timestamp (effective = baseline + whole years elapsed, capped by position).
	// apen keeps the same pair on user and on the social card so each surface
	// computes the effective value independently — see apen's
	// models.EffectiveExperienceYears. nil means "never set, do not increment".
	YearOfExperienceUpdatedAt *time.Time `json:"year_of_experience_updated_at,omitempty"`

	// ExperienceRange is the nurse / phar range-based tenure; mutually
	// exclusive with YearOfExperience. Range-based tenure is a fixed bucket and
	// does NOT auto-increment, so it carries no updated-at stamp.
	ExperienceRange *ExperienceRange `json:"experience_range,omitempty"`

	// PreferredLocations is dual-written with the resume's preferred_locations:
	// updating either surface syncs the other in one transaction (see
	// store.BusinessCard.Upsert and store.Resume.Update).
	PreferredLocations []string `json:"preferred_locations,omitempty"`
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
