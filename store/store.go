package store

import (
	"context"

	"github.com/A-pen-app/resume-sdk/models"
)

type Resume interface {
	Create(ctx context.Context, userID string, content *models.ResumeContent) (*models.Resume, error)
	Get(ctx context.Context, userID string) (*models.Resume, error)
	Update(ctx context.Context, userID string, resume *models.ResumeContent) error
	CreateSnapshot(ctx context.Context, userID string, chatID string) (*models.ResumeSnapshot, error)
	GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error)
}
