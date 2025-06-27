package store

import (
	"context"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
)

type Resume interface {
	Create(ctx context.Context, userID string, content *models.ResumeContent) (*models.Resume, error)
	Get(ctx context.Context, userID string) (*models.Resume, error)
	Update(ctx context.Context, userID string, resume *models.ResumeContent) error
	CreateSnapshot(ctx context.Context, userID string) (*models.ResumeSnapshot, error)
	GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error)
	CreateRelation(ctx context.Context, userID string, snapshotID string, chatID string, postID string) (*models.ResumeRelation, error)
	GetRelation(ctx context.Context, chatID string) (*models.ResumeRelation, error)
}

type Chat interface {
	Get(ctx context.Context, chatID, userID string) (*models.ChatRoom, error)
	GetChats(ctx context.Context, userID string, next string, count int, status models.ChatAnnotation, unreadOnly bool) ([]*models.ChatRoom, error)
	GetChatID(ctx context.Context, postID, senderID, receiverID string) (string, error)
	Read(ctx context.Context, userID, chatID string) error
	GetMessage(ctx context.Context, messageID string) (*models.Message, error)
	GetMessages(ctx context.Context, chatID string, next string, count int) ([]*models.Message, error)
	GetNewMessages(ctx context.Context, chatID string, after time.Time) ([]*models.Message, error)
	AddMessage(ctx context.Context, userID, chatID, receiverID string, typ models.MessageType, body *string, mediaIDs []string, replyToMessageID *string) (string, error)
	AddMessages(ctx context.Context, userID, chatID, receiverID string, msgs []*models.Message) error
	EditMessage(ctx context.Context, messageID string, newStatus models.MessageStatus) error
	Annotate(ctx context.Context, chatID, userID string, status models.ChatAnnotation) error
}
