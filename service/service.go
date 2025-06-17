package service

import (
	"context"

	"github.com/A-pen-app/hire-sdk/models"
)

type Resume interface {
	Patch(ctx context.Context, userID string, resume *models.ResumeContent) error
	Get(ctx context.Context, userID string) (*models.Resume, error)
	GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error)
}
