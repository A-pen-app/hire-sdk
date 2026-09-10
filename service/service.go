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
	ListRelations(ctx context.Context, bundleID, viewerID string, offset, count int, opts ...models.ListRelationOptionFunc) ([]*models.ResumeRelation, error)
	GetRelation(ctx context.Context, bundleID, viewerID string, opts ...models.GetRelationOptionFunc) (*models.ResumeRelation, error)
	GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error)
	GetResponseMediansByPost(ctx context.Context, bundleID string, after time.Time) (map[string]float64, error)
}

type Chat interface {
	New(ctx context.Context, bundleID, senderID, receiverID string, postID *string, options ...models.NewChatOptionFunc) (string, error)
	Get(ctx context.Context, bundleID, chatID, userID string) (*models.ChatRoom, error)
	GetChats(ctx context.Context, bundleID, userID string, next string, count int, options ...models.GetOptionFunc) ([]*models.ChatRoom, string, error)
	CountChats(ctx context.Context, bundleID, userID string, options ...models.GetOptionFunc) (int, error)
	GetChatMessages(ctx context.Context, bundleID, userID, chatID string, next string, count int) ([]*models.Message, string, error)
	FetchNewMessages(ctx context.Context, bundleID, userID, chatID string, lastMessageID string) ([]*models.Message, error)
	SendMessage(ctx context.Context, bundleID, userID, chatID string, options ...models.SendOptionFunc) (*models.Message, error)
	UnsendMessage(ctx context.Context, bundleID, userID, messageID string) error
	// Archive hides a room from this user's list, or brings it back (封存).
	Archive(ctx context.Context, bundleID, userID, chatID string, archived bool) error
	// Clear deletes the conversation for this user only (刪除對話).
	Clear(ctx context.Context, bundleID, userID, chatID string) error
	// Pin pins a room to the top of this user's list, or unpins it (置頂).
	Pin(ctx context.Context, bundleID, userID, chatID string, pinned bool) error
	GetBusinessCardOnly(ctx context.Context, bundleID string, before time.Duration) ([]*models.BusinessCardChat, error)
}

type BusinessCardService interface {
	Get(ctx context.Context, bundleID, userID string) (*models.BusinessCardContent, error)
	List(ctx context.Context, bundleID string, userIDs []string) (map[string]*models.BusinessCardContent, error)
	Update(ctx context.Context, bundleID, userID string, card *models.BusinessCardContent) (*models.BusinessCardContent, error)
}

type Agreement interface {
	Agree(ctx context.Context, bundleID, userID, version string) error
	Get(ctx context.Context, bundleID, userID string) (*models.AgreementRecord, error)
}

type Subscription interface {
	Get(ctx context.Context, bundleID, userID string) (*models.UserSubscription, error)
	Update(ctx context.Context, bundleID, userID string, status models.SubscriptionStatus, expiresAt *time.Time) error
}
