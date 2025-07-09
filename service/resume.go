package service

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

type resumeService struct {
	r store.Resume
	a store.App
}

func NewResume(r store.Resume, a store.App) Resume {
	return &resumeService{
		r: r,
		a: a,
	}
}

func (s *resumeService) Patch(ctx context.Context, bundleID, userID string, resume *models.ResumeContent) error {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return err
	}

	if err := s.r.Update(ctx, app.ID, userID, resume); err != nil {
		logging.Errorw(ctx, "failed to update resume", "err", err, "appID", app.ID, "userID", userID)
		return err
	}
	return nil
}

func (s *resumeService) Get(ctx context.Context, bundleID, userID string) (*models.Resume, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	resume, err := s.r.Get(ctx, app.ID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			resume, err = s.r.Create(ctx, app.ID, userID, &models.ResumeContent{})
			if err != nil {
				logging.Errorw(ctx, "failed to create resume", "err", err, "appID", app.ID, "userID", userID)
				return nil, err
			}
		} else {
			logging.Errorw(ctx, "failed to get resume", "err", err, "appID", app.ID, "userID", userID)
			return nil, err
		}
	}
	return resume, nil
}

func (s *resumeService) GetUserAppliedPostIDs(ctx context.Context, bundleID, userID string) ([]string, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	postIDs, err := s.r.GetUserAppliedPostIDs(ctx, app.ID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to get resume", "err", err, "appID", app.ID, "userID", userID)
		return nil, err
	}
	return postIDs, nil
}

func (s *resumeService) GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error) {
	snapshot, err := s.r.GetSnapshot(ctx, snapshotID)
	if err != nil {
		logging.Errorw(ctx, "failed to get resume snapshot", "err", err, "snapshotID", snapshotID)
		return nil, err
	}
	return snapshot, nil
}
