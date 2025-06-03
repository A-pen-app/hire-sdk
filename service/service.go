package service

import (
	"context"

	"github.com/A-pen-app/resume-sdk/models"
)

type Resume interface {
	Patch(ctx context.Context, userID string, resume *models.ResumeContent) error
	Get(ctx context.Context, userID string) (*models.Resume, error)
	GetHistory(ctx context.Context, resumeID string) (*models.ResumeHistory, error)
}
