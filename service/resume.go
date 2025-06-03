package service

import (
	"context"

	"github.com/A-pen-app/resume-sdk/models"
	"github.com/A-pen-app/resume-sdk/store"
)

type resumeService struct {
	store store.Resume
}

func NewResume(ctx context.Context, store store.Resume) Resume {
	return &resumeService{
		store: store,
	}
}

func (s *resumeService) Patch(ctx context.Context, userID string, resume *models.ResumeContent) error {
	return nil
}

func (s *resumeService) Get(ctx context.Context, userID string) (*models.Resume, error) {
	return nil, nil
}

func (s *resumeService) GetHistory(ctx context.Context, resumeID string) (*models.ResumeHistory, error) {
	return nil, nil
}
