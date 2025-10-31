package store

import (
	"context"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
)

type Resume interface {
	Create(ctx context.Context, appID, userID string, content *models.ResumeContent) (*models.Resume, error)
	Get(ctx context.Context, appID, userID string) (*models.Resume, error)
	GetUserAppliedPostIDs(ctx context.Context, appID, userID string) ([]string, error)
	Update(ctx context.Context, appID, userID string, resume *models.ResumeContent) error
	CreateSnapshot(ctx context.Context, appID, userID string) (*models.ResumeSnapshot, error)
	GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error)
	CreateRelation(ctx context.Context, appID, userID string, snapshotID string, chatID string, postID string, status models.ResumeStatus) (*models.ResumeRelation, error)
	GetRelation(ctx context.Context, opts ...models.GetRelationOptionFunc) (*models.ResumeRelation, error)
	ListRelations(ctx context.Context, appID string, opts ...models.ListRelationOptionFunc) ([]*models.ResumeRelation, error)
	Read(ctx context.Context, snapshotID string) error
	UpdateRelationStatus(ctx context.Context, snapshotID string, status models.ResumeStatus) error
	UpdateRelationListStatus(ctx context.Context, postIDs []string, status models.ResumeStatus) error
}

type Chat interface {
	Get(ctx context.Context, appID, chatID, userID string) (*models.ChatRoom, error)
	GetChats(ctx context.Context, appID, userID string, next string, count int, status models.ChatAnnotation, unreadOnly bool, includeNoMessage bool) ([]*models.ChatRoom, error)
	GetChatID(ctx context.Context, appID, senderID, receiverID string, postID *string) (string, error)
	Read(ctx context.Context, userID, chatID string) error
	GetMessage(ctx context.Context, messageID string) (*models.Message, error)
	GetMessages(ctx context.Context, chatID string, next string, count int) ([]*models.Message, error)
	GetNewMessages(ctx context.Context, chatID string, after time.Time) ([]*models.Message, error)
	GetFirstMessages(ctx context.Context, opt []models.FirstMessageOption) (map[string]*models.Message, error)
	AddMessage(ctx context.Context, userID, chatID, receiverID string, typ models.MessageType, body *string, mediaIDs []string, replyToMessageID *string, referenceID *string) (string, error)
	AddMessages(ctx context.Context, userID, chatID, receiverID string, msgs []*models.Message) error
	EditMessage(ctx context.Context, messageID string, newStatus models.MessageStatus) error
	Annotate(ctx context.Context, chatID, userID string, status models.ChatAnnotation) error
	Pin(ctx context.Context, chatID, userID string, isPinned bool) error
}

type App interface {
	GetByBundleID(ctx context.Context, bundleID string) (*models.App, error)
}

type Media interface {
	Get(ctx context.Context, mediaIDs []string) ([]*models.Media, error)
	New(ctx context.Context, upload *models.MediaUpload) (string, error)
}

type Agreement interface {
	Agree(ctx context.Context, appID, userID, version string) error
	Get(ctx context.Context, appID, userID string) (*models.AgreementRecord, error)
}

type Subscription interface {
	Get(ctx context.Context, appID, userID string) (*models.UserSubscription, error)
	List(ctx context.Context, appID string, userIDs []string) ([]*models.UserSubscription, error)
	Update(ctx context.Context, appID, userID string, status models.SubscriptionStatus, expiresAt *time.Time) error
}
