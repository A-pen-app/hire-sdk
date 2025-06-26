package models

import (
	"encoding/json"
	"errors"
	"time"
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
	RefID *string `json:"reference_id,omitempty" db:"-" example:"ce465117-1c0a-4746-8500-e4fb2c960f70"`

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
	CreatedAt     time.Time `json:"created_at" db:"created_at" example:"2023-10-01T04:00:00Z"`
	UpdatedAt     time.Time `json:"-" db:"updated_at"`
	LastMessageID *string   `json:"-" db:"last_message_id"`
	LastMessage   *Message  `json:"last_message" db:"-"`
	IsResumeRead  bool      `json:"is_resume_read" db:"is_resume_read"`
	PostID        string    `json:"post_id" db:"post_id"`

	Role           int            `json:"role" db:"-"`
	HireStatus     string         `json:"hire_status" db:"-"`
	ResumeSnapshot ResumeSnapshot `json:"resume_snapshot" db:"-"`
}

type MediaType int

const (
	Image MediaType = iota + 1
	Audio
	Video
	File
)

type Media struct {
	ID         string  `json:"-" `
	URL        string  `json:"url"`
	PreviewURL *string `json:"preview_url"`

	Placeholder *string   `json:"placeholder,omitempty"`
	Type        MediaType `json:"type"`

	// indicate the url to connect when user click the media
	RedirectURL *string    `json:"redirect_url,omitempty"`
	Title       *string    `json:"title,omitempty"`
	Size        *int64     `json:"size,omitempty"`
	ExpiredAt   *time.Time `json:"expired_at,omitempty"`
}

type DisplayUser struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	Gender  string `json:"gender"`
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

type getOption struct {
	status     ChatAnnotation
	unreadOnly bool
}
type GetOption func(*getOption) error

func ByStatus(status ChatAnnotation, unreadOnly bool) GetOption {
	return func(opt *getOption) error {
		switch status {
		case Todo, Done:
			opt.status = status
		case None:
			opt.unreadOnly = unreadOnly
		default:
			return errors.New("action not allowed")
		}
		return nil
	}
}

type sendOption struct {
	typ              MessageType
	body             *string
	mediaIDs         []string
	replyToMessageID *string
}
type SendOption func(*sendOption) error

func WithText(body string) SendOption {
	return func(opt *sendOption) error {
		if opt.typ != MsgEmpty {
			return errors.New("wrong parameters")
		}
		opt.typ = MsgText
		opt.body = &body
		return nil
	}
}

func WithMedia(mediaIDs []string) SendOption {
	return func(opt *sendOption) error {
		if opt.typ != MsgEmpty {
			return errors.New("wrong parameters")
		}
		opt.typ = MsgImage
		opt.mediaIDs = mediaIDs
		return nil
	}
}

func ReplyTo(replyToMessageID string) SendOption {
	return func(opt *sendOption) error {
		opt.replyToMessageID = &replyToMessageID
		return nil
	}
}
