package models

import (
	"encoding/json"
	"errors"
	"time"

	feedmodel "github.com/A-pen-app/feed-sdk/model"
)

// MessageType describes the type of message
type MessageType int

const (
	MsgEmpty MessageType = iota
	MsgText
	MsgImage
	MsgForm
	MsgMeetup
	MsgFile
	MsgPost
)

func (t MessageType) String() string {
	switch t {
	case MsgText:
		return "text"
	case MsgImage:
		return "image"
	case MsgForm:
		return "form"
	case MsgMeetup:
		return "meetup"
	case MsgFile:
		return "file"
	case MsgPost:
		return "post"
	default:
		return "unknown"
	}
}

type MessageStatus int

const (
	Unsent MessageStatus = 1 << iota
	DeletedBySender
	DeletedByReceiver
)
const (
	Normal      MessageStatus = 0
	Unavailable               = Unsent | DeletedBySender | DeletedByReceiver
)

func (s MessageStatus) String() string {
	switch s {
	case Normal:
		return "NORMAL"
	case Unsent:
		return "UNSENT"
	case DeletedBySender, DeletedByReceiver:
		return "DELETED"
	case Unavailable:
		return "UNAVAILABLE"
	default:
		return ""
	}
}

func (s MessageStatus) HasOneOf(flag MessageStatus) bool {
	return s&flag != 0
}

func (s MessageStatus) MarshalJSON() ([]byte, error) {
	if s < Normal || s > Unavailable {
		return nil, errors.New("wrong parameters")
	}
	str := ""
	switch s {
	case Normal:
		str = "NORMAL"
	case Unsent:
		str = "UNSENT"
	case DeletedBySender, DeletedByReceiver:
		str = "DELETED"
	case Unavailable:
		str = "UNAVAILABLE"
	}
	return json.Marshal(str)
}

type ChatControlFlag int

const (
	NeverGotMessages ChatControlFlag = 1 << iota
	BlockedByUser
	HidByAdmin
)
const Pass ChatControlFlag = 0

func (f ChatControlFlag) HasOneOf(flag ChatControlFlag) bool {
	return f&flag != 0
}

// Message describes message structure
type Message struct {
	// common fields
	ID        string        `json:"message_id" db:"id" example:"uuid"`
	ChatID    string        `json:"chat_id" db:"chat_id" example:"uuid"`
	CreatedAt time.Time     `json:"created_at" db:"created_at" example:"2023-10-01T04:00:00Z"`
	Status    MessageStatus `json:"status" db:"status"`
	IsMine    *bool         `json:"is_mine,omitempty" db:"-"`
	// 0: 不使用
	// 1: 文字訊息
	// 2: 圖片訊息
	// 3: 問卷
	// 4: 活動
	// 5: 檔案
	// 6: 文章
	Type     MessageType  `json:"type" db:"type" example:"1"`
	SenderID string       `json:"-" db:"sender_id"`
	Sender   *DisplayUser `json:"sender,omitempty" db:"-"`

	// ReplyTo exists only if ReplyToMessageID not null
	ReplyToMessageID *string  `json:"-" db:"reply_to_message_id" example:"uuid"`
	ReplyTo          *Message `json:"reply_to,omitempty" db:"-"`

	// Type=MsgText
	Body *string `json:"body,omitempty" db:"body" example:"test message"`

	// Type=MsgImage
	MediaIDs []string `json:"-" db:"media_ids"`
	Medias   []*Media `json:"medias,omitempty" db:"-"`

	// output-only fields, injected from other table
	// for Type=MsgForm, it stands for form_id
	// for Type=MsgMeetup, it stands for meetup_id
	RefID *string `json:"reference_id,omitempty" db:"reference_id" example:"ce465117-1c0a-4746-8500-e4fb2c960f70"`

	// Type=MsgForm, MsgMeetup
	Title     *string `json:"title,omitempty" db:"-" example:"問卷標題"`
	BannerURL *string `json:"banner_url,omitempty" db:"-" example:"https://www.google.com.tw/images/branding/googlelogo/2x/googlelogo_color_272x92dp.png"`

	// Type=MsgMeetup
	StartAt *time.Time `json:"start_at,omitempty" db:"-" example:"2024-10-01T04:00:00Z"`
	Fee     *Money     `json:"fee,omitempty" db:"-"`
	Tags    []string   `json:"tags,omitempty" db:"-" example:"mission,reward"`

	// Type=MsgForm
	Desc        *string       `json:"description,omitempty" db:"-" example:"問卷描述"`
	EndAt       *time.Time    `json:"end_at,omitempty" db:"-" example:"2024-10-01T04:00:00Z"`
	Reward      *Money        `json:"reward,omitempty" db:"-"`
	SubmittedAt *time.Time    `json:"submitted_at,omitempty" db:"-" example:"2024-09-01T04:00:00Z"`
	Audience    *AudienceForm `json:"audience,omitempty" db:"-" `

	Link *string `json:"link,omitempty" db:"-" example:"apen://mp_chats/{chat_id} or https://www.google.com.tw/"`
}

type ChatAnnotation int

const (
	None ChatAnnotation = iota
	Todo
	Done
	Deleted
)

func (s ChatAnnotation) MarshalJSON() ([]byte, error) {
	if s < None || s > Deleted {
		return nil, errors.New("wrong parameters")
	}
	str := ""
	switch s {
	case None:
		str = "NONE"
	case Todo:
		str = "TODO"
	case Done:
		str = "DONE"
	case Deleted:
		str = "DELETED"
	}
	return json.Marshal(str)
}

type ChatRoom struct {
	//chat_thread
	ChatID      string          `json:"chat_id" db:"chat_id"`
	SenderID    string          `json:"sender_id" db:"sender_id"`
	ReceiverID  string          `json:"-" db:"receiver_id"`
	Receiver    *DisplayUser    `json:"receiver" db:"-"`
	UnreadCount int64           `json:"unread_count" db:"unread_count"`
	LastSeenAt  *time.Time      `json:"last_seen_at" db:"last_seen_at" example:"2023-10-01T04:00:00Z"`
	Status      ChatAnnotation  `json:"status" db:"status"`
	ControlFlag ChatControlFlag `json:"-" db:"control_flag"`
	IsPinned    bool            `json:"is_pinned" db:"is_pinned"`

	//chat
	AppID          string              `json:"-" db:"app_id"`
	CreatedAt      time.Time           `json:"created_at" db:"created_at" example:"2023-10-01T04:00:00Z"`
	UpdatedAt      time.Time           `json:"-" db:"updated_at"`
	LastMessageID  *string             `json:"-" db:"last_message_id"`
	LastMessage    *Message            `json:"last_message" db:"-"`
	PostID         *string             `json:"post_id" db:"post_id"`
	Role           Role                `json:"role" db:"-"`
	HireStatus     *HireStatus         `json:"hire_status" db:"-" default:"INACTIVE" example:"INACTIVE"`
	ResumeSnapshot *ChatResumeSnapshot `json:"resume_snapshot" db:"-"`
}

func (p ChatRoom) Feedtype() feedmodel.FeedType {
	return feedmodel.TypeChat
}

func (p ChatRoom) Score() float64 {
	return 0.
}

func (p ChatRoom) GetID() string {
	return p.ChatID
}

type ChatResumeSnapshot struct {
	ID      string         `json:"id"`
	Content *ResumeContent `json:"content"`
	IsRead  bool           `json:"is_read"`
	Status  ResumeStatus   `json:"status"`
}

type HireStatus string

const (
	HireStatusInactive HireStatus = "INACTIVE"
	HireStatusActive   HireStatus = "ACTIVE"
	HireStatusDeleted  HireStatus = "DELETED"
)

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

type Role int

const (
	RoleNone      Role = iota // 0: none
	RoleOfficial              // 1: 官方
	RoleJobSeeker             // 2: 求職方
	RoleRecruiter             // 3: 徵才方
)

type DisplayUser struct {
	ID        string  `json:"user_id"`
	Name      string  `json:"name"`
	Picture   string  `json:"picture"`
	Gender    string  `json:"gender"`
	Character *string `json:"-"`
	PushToken *string `json:"-"`
}

type Money struct {
	Currency string `json:"currency"`
	Quantity string `json:"quantity"`
}

type AudienceForm struct {
	Departments  []string `json:"departments,omitempty"`
	Positions    []string `json:"positions,omitempty"`
	MaxSeniority *int     `json:"max_seniority,omitempty"`
	MinSeniority *int     `json:"min_seniority,omitempty"`
}

type GetOption struct {
	Status     ChatAnnotation
	UnreadOnly bool
}
type GetOptionFunc func(*GetOption) error

func ByStatus(status ChatAnnotation, unreadOnly bool) GetOptionFunc {
	return func(opt *GetOption) error {
		switch status {
		case Todo, Done:
			opt.Status = status
		case None:
			opt.UnreadOnly = unreadOnly
		default:
			return errors.New("action not allowed")
		}
		return nil
	}
}

type SendOption struct {
	Type             MessageType
	Body             *string
	MediaIDs         []string
	ReplyToMessageID *string
}
type SendOptionFunc func(*SendOption) error

func WithText(body string) SendOptionFunc {
	return func(opt *SendOption) error {
		if opt.Type != MsgEmpty {
			return errors.New("wrong parameters")
		}
		opt.Type = MsgText
		opt.Body = &body
		return nil
	}
}

func WithMedia(mediaIDs []string) SendOptionFunc {
	return func(opt *SendOption) error {
		if opt.Type != MsgEmpty {
			return errors.New("wrong parameters")
		}
		opt.Type = MsgImage
		opt.MediaIDs = mediaIDs
		return nil
	}
}

func WithFile(fileIDs []string) SendOptionFunc {
	return func(opt *SendOption) error {
		if opt.Type != MsgEmpty {
			return errors.New("wrong parameters")
		}
		opt.Type = MsgFile
		opt.MediaIDs = fileIDs
		return nil
	}
}

func ReplyTo(replyToMessageID string) SendOptionFunc {
	return func(opt *SendOption) error {
		opt.ReplyToMessageID = &replyToMessageID
		return nil
	}
}
