package service

import (
	"context"

	"github.com/A-pen-app/hire-sdk/models"
)

type Resume interface {
	Patch(ctx context.Context, bundleID, userID string, resume *models.ResumeContent) error
	Get(ctx context.Context, bundleID, userID string) (*models.Resume, error)
	GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error)
}

type Chat interface {
	New(ctx context.Context, bundleID, senderID, receiverID string, postID *string, resume *models.ResumeContent) (string, error)
	Get(ctx context.Context, bundleID, chatID, userID string) (*models.ChatRoom, error)
	GetChats(ctx context.Context, bundleID, userID string, next string, count int, options ...models.GetOptionFunc) ([]*models.ChatRoom, string, error)
	GetChatMessages(ctx context.Context, bundleID, userID, chatID string, next string, count int) ([]*models.Message, string, error)
	SendMessage(ctx context.Context, bundleID, userID, chatID string, options ...models.SendOptionFunc) (*models.Message, error)
	UnsendMessage(ctx context.Context, bundleID, userID, messageID string) error
}

type Agreement interface {
	Agree(ctx context.Context, bundleID, userID, version string) error
	Get(ctx context.Context, bundleID, userID string) (*models.AgreementRecord, error)
}
