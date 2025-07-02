package service

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

type resumeService struct {
	r store.Resume
	a store.App
}

func NewResume(r store.Resume) Resume {
	return &resumeService{
		r: r,
	}
}

func (s *resumeService) Patch(ctx context.Context, bundleID, userID string, resume *models.ResumeContent) error {

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		return err
	}

	return s.r.Update(ctx, app.ID, userID, resume)
}

func (s *resumeService) Get(ctx context.Context, bundleID, userID string) (*models.Resume, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		return nil, err
	}

	resume, err := s.r.Get(ctx, app.ID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			resume, err = s.r.Create(ctx, app.ID, userID, &models.ResumeContent{})
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
	return s.r.GetSnapshot(ctx, snapshotID)
}
