package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type ResumeContent struct {
	// common
	RealName           *string             `json:"real_name"`
	Email              *string             `json:"email"`
	PhoneNumber        *string             `json:"phone_number"`
	PreferredLocations []string            `json:"preferred_locations"`
	ExpectedSalary     *string             `json:"expected_salary"`
	CollaborationTypes []CollaborationType `json:"collaboration_types"`
	AvailableStartDate *string             `json:"available_start_date"`
	SpecialRequirement *string             `json:"special_requirement"`
	ContactTimes       []ContactTime       `json:"contact_times"`
	Gender             *string             `json:"gender,omitempty"`
	// for doctor
	Position        *string  `json:"position,omitempty"`
	Departments     []string `json:"departments,omitempty"`
	CustomSpecialty *string  `json:"custom_specialty,omitempty"`
	Expertise       *string  `json:"expertise,omitempty"`

	// for doctor and pharmacist
	AlmaMater        *AlmaMater `json:"alma_mater,omitempty"`
	YearOfGraduation *string    `json:"year_of_graduation,omitempty"`

	// for pharmacist and nurse
	CurrentOrganization *string `json:"current_organization,omitempty"`
	CurrentJobTitle     *string `json:"current_job_title,omitempty"`

	// for nurse
	BirthYear          *string             `json:"birth_year,omitempty"`
	Certificate        *string             `json:"certificate,omitempty"`
	HospitalExperience *HospitalExperience `json:"hospital_experience,omitempty"`
}

type HospitalExperience struct {
	Department       *string `json:"department,omitempty"`
	YearOfExperience int     `json:"year_of_experience"`
}

type ContactTime struct {
	DayOfWeek string `json:"day_of_week" example:"星期一"`
	StartTime string `json:"start_time" example:"09:00"`
	EndTime   string `json:"end_time" example:"18:00"`
}

type AlmaMater struct {
	Key         string  `json:"key"`
	CustomValue *string `json:"custom_value"`
}

type CollaborationType int

const (
	CollaborationType_FullTime          CollaborationType = iota // 全職
	CollaborationType_PartTime                                   // 兼職
	CollaborationType_Attending                                  // 掛牌
	CollaborationType_Lecturer                                   // 講座
	CollaborationType_Prescription                               // 葉配
	CollaborationType_Endorsement                                // 代言
	CollaborationType_Telemedicine                               // 遠距醫療
	CollaborationType_MarketResearch                             // 市調訪談
	CollaborationType_AcademicEditing                            // 學術編輯
	CollaborationType_ProductExperience                          // 產品體驗
)

type Resume struct {
	ID        string         `json:"-" db:"id"`
	AppID     string         `json:"-" db:"app_id"`
	UserID    string         `json:"-" db:"user_id"`
	Content   *ResumeContent `json:"content" db:"content"`
	CreatedAt time.Time      `json:"-" db:"created_at"`
	UpdatedAt time.Time      `json:"-" db:"updated_at"`
}

type ResumeSnapshot struct {
	ID        string         `json:"id" db:"id"`
	ResumeID  string         `json:"-" db:"resume_id"`
	Content   *ResumeContent `json:"content" db:"content"`
	CreatedAt time.Time      `json:"-" db:"created_at"`
}

type ResumeRelation struct {
	ID         string       `json:"-" db:"id"`
	AppID      string       `json:"-" db:"app_id"`
	UserID     string       `json:"-" db:"user_id"`
	SnapshotID string       `json:"-" db:"snapshot_id"`
	PostID     string       `json:"-" db:"post_id"`
	ChatID     string       `json:"-" db:"chat_id"`
	IsRead     bool         `json:"-" db:"is_read"`
	CreatedAt  time.Time    `json:"-" db:"created_at"`
	UpdatedAt  time.Time    `json:"-" db:"updated_at"`
	Status     ResumeStatus `json:"-" db:"status"`
}

type ResumeStatus int

const (
	ResumeStatusLocked ResumeStatus = iota
	ResumeStatusUnlocked
)

func (u ResumeStatus) MarshalJSON() ([]byte, error) {
	str := ""
	switch u {
	case ResumeStatusLocked:
		str = "LOCKED"
	case ResumeStatusUnlocked:
		str = "UNLOCKED"
	}
	return json.Marshal(str)
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

type GetRelationOption struct {
	ChatID     *string
	SnapshotID *string
}
type GetRelationOptionFunc func(*GetRelationOption) error

func ByChat(chatID string) GetRelationOptionFunc {
	return func(opt *GetRelationOption) error {
		opt.ChatID = &chatID
		return nil
	}
}

func BySnapshot(snapshotID string) GetRelationOptionFunc {
	return func(opt *GetRelationOption) error {
		opt.SnapshotID = &snapshotID
		return nil
	}
}
