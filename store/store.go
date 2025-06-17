package store

import (
	"context"

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
