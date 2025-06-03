package store

import (
	"context"

	"github.com/A-pen-app/resume-sdk/models"
)

type Resume interface {
	Create(ctx context.Context, userID string, resume *models.ResumeContent) (*models.Resume, error)
	Get(ctx context.Context, userID string) (*models.Resume, error)
	Update(ctx context.Context, userID string, resume *models.ResumeContent) error
	CreateHistory(ctx context.Context, userID string, chatID string, resume *models.ResumeContent) (*models.ResumeHistory, error)
	GetHistory(ctx context.Context, id string) (*models.ResumeHistory, error)
}
