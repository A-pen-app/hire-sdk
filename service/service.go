package service

import (
	"context"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
)

type Resume interface {
	Patch(ctx context.Context, bundleID, userID string, resume *models.ResumeContent) error
	Get(ctx context.Context, bundleID, userID string) (*models.Resume, error)
	GetUserAppliedPostIDs(ctx context.Context, bundleID, userID string) ([]string, error)
	GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error)
}

type Chat interface {
	New(ctx context.Context, bundleID, senderID, receiverID string, postID *string, resume *models.ResumeContent, resumeStatus models.ResumeStatus) (string, error)
	Get(ctx context.Context, bundleID, chatID, userID string) (*models.ChatRoom, error)
	GetChats(ctx context.Context, bundleID, userID string, next string, count int, options ...models.GetOptionFunc) ([]*models.ChatRoom, string, error)
	GetChatMessages(ctx context.Context, bundleID, userID, chatID string, next string, count int) ([]*models.Message, string, error)
	FetchNewMessages(ctx context.Context, bundleID, userID, chatID string, lastMessageID string) ([]*models.Message, error)
	SendMessage(ctx context.Context, bundleID, userID, chatID string, options ...models.SendOptionFunc) (*models.Message, error)
	UnsendMessage(ctx context.Context, bundleID, userID, messageID string) error
}

type Agreement interface {
	Agree(ctx context.Context, bundleID, userID, version string) error
	Get(ctx context.Context, bundleID, userID string) (*models.AgreementRecord, error)
}

type Subscription interface {
	Get(ctx context.Context, bundleID, userID string) (*models.UserSubscription, error)
	Update(ctx context.Context, bundleID, userID string, status models.SubscriptionStatus, expiresAt *time.Time) error
}
