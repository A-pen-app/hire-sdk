package service

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

type resumeService struct {
	store store.Resume
}

func NewResume(store store.Resume) Resume {
	return &resumeService{
		store: store,
	}
}

func (s *resumeService) Patch(ctx context.Context, appID, userID string, resume *models.ResumeContent) error {
	return s.store.Update(ctx, appID, userID, resume)
}

func (s *resumeService) Get(ctx context.Context, appID, userID string) (*models.Resume, error) {
	resume, err := s.store.Get(ctx, appID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			resume, err = s.store.Create(ctx, appID, userID, &models.ResumeContent{})
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
