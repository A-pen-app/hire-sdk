package service

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/resume-sdk/models"
	"github.com/A-pen-app/resume-sdk/store"
)

type resumeService struct {
	store store.Resume
}

func NewResume(store store.Resume) Resume {
	return &resumeService{
		store: store,
	}
}

func (s *resumeService) Patch(ctx context.Context, userID string, resume *models.ResumeContent) error {
	return s.store.Update(ctx, userID, resume)
}

func (s *resumeService) Get(ctx context.Context, userID string) (*models.Resume, error) {
	resume, err := s.store.Get(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			resume, err = s.store.Create(ctx, userID, &models.ResumeContent{})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return resume, nil
}

func (s *resumeService) GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error) {
	return s.store.GetSnapshot(ctx, snapshotID)
}
