package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type ResumeContent struct {
	// common
	RealName           string              `json:"real_name"`            // Shared by all
	Email              string              `json:"email"`                // Shared by all
	PhoneNumber        string              `json:"phone_number"`         // Shared by all
	PreferredLocations []string            `json:"preferred_locations"`  // Shared by all
	ExpectedSalary     *string             `json:"expected_salary"`      // Shared by all
	CollaborationType  []CollaborationType `json:"collaboration_type"`   // Shared by all
	AvailableStartDate *string             `json:"available_start_date"` // Shared by all
	SpecialRequirement *string             `json:"special_requirement"`  // Shared by all
	ContactTime        []ContactTime       `json:"contact_time"`         // Shared by all

	// for doctor
	Position     *string  `json:"position,omitempty"`
	Departments  []string `json:"departments,omitempty"`
	Subspecialty *string  `json:"subspecialty,omitempty"`
	Expertise    *string  `json:"expertise,omitempty"`

	// for doctor and pharmacist
	AlmaMater        *AlmaMater `json:"alma_mater,omitempty"`
	YearOfGraduation *string    `json:"year_of_graduation,omitempty"`

	// for pharmacist and nurse
	CurrentOrganization *string `json:"current_organization,omitempty"`
	CurrentJobTitle     *string `json:"current_job_title,omitempty"`

	// for nurse
	Age                *int                `json:"age,omitempty"`
	Certificate        *string             `json:"certificate,omitempty"`
	HospitalExperience *HospitalExperience `json:"hospital_experience,omitempty"`
}

type HospitalExperience struct {
	Department       string  `json:"department"`
	YearOfGraduation *string `json:"year_of_graduation"`
}

type ContactTime struct {
	DayOfWeek string `json:"day_of_week" example:"星期一"`
	StartTime string `json:"start_time" example:"09:00"`
	EndTime   string `json:"end_time" example:"18:00"`
}

type AlmaMater struct {
	Key   string  `json:"key"`
	Value *string `json:"value"`
}

type CollaborationType int

const (
	CollaborationType_FullTime       CollaborationType = iota // 全職
	CollaborationType_PartTime                                // 兼職
	CollaborationType_Attending                               // 掛牌
	CollaborationType_Lecturer                                // 講座
	CollaborationType_Prescription                            // 葉配
	CollaborationType_Endorsement                             // 代言
	CollaborationType_Telemedicine                            // 遠距醫療
	CollaborationType_MarketResearch                          // 市調訪談
)

type Resume struct {
	ID       string        `json:"-" db:"id"`
	UserID   string        `json:"-" db:"user_id"`
	Content  ResumeContent `json:"content" db:"content"`
	CreateAt time.Time     `json:"-" db:"create_at"`
	UpdateAt time.Time     `json:"-" db:"update_at"`
}

type ResumeHistory struct {
	ID        string        `json:"-" db:"id"`
	UserID    string        `json:"-" db:"user_id"`
	Content   ResumeContent `json:"content" db:"content"`
	CreatedAt time.Time     `json:"-" db:"created_at"`
	ChatID    string        `json:"chat_id" db:"chat_id"`
}

// Value implements the driver.Valuer interface for inserting as jsonb
func (r ResumeContent) Value() (driver.Value, error) {
	return json.Marshal(r)
}

// Scan implements the sql.Scanner interface for reading jsonb
func (r *ResumeContent) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, r)
}
