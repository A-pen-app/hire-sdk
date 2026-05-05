package service

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

type businessCardService struct {
	bc store.BusinessCard
	r  store.Resume
	a  store.App
}

func NewBusinessCard(bc store.BusinessCard, r store.Resume, a store.App) BusinessCardService {
	return &businessCardService{
		bc: bc,
		r:  r,
		a:  a,
	}
}

// Get returns the user's business card content.
// If no card exists, falls back to resume data.
func (s *businessCardService) Get(ctx context.Context, bundleID, userID string) (*models.BusinessCardContent, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	bc, err := s.bc.Get(ctx, app.ID, userID)
	if err != nil && err != sql.ErrNoRows {
		logging.Errorw(ctx, "failed to get business card", "err", err, "userID", userID)
		return nil, err
	}

	if bc != nil && bc.Content != nil {
		return bc.Content, nil
	}

	// Fallback: no card yet, try to derive from resume
	resume, err := s.r.Get(ctx, app.ID, userID)
	if err != nil && err != sql.ErrNoRows {
		logging.Errorw(ctx, "failed to get resume for card fallback", "err", err, "userID", userID)
		return nil, err
	}

	if resume != nil && resume.Content != nil {
		return &models.BusinessCardContent{
			RealName:    resume.Content.RealName,
			Position:    resume.Content.Position,
			Departments: resume.Content.Departments,
		}, nil
	}

	return &models.BusinessCardContent{}, nil
}

// Update overwrites the user's business card.
// If no resume exists, also creates one with the card data.
func (s *businessCardService) Update(ctx context.Context, bundleID, userID string, card *models.BusinessCardContent) (*models.BusinessCardContent, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	if err := s.bc.Upsert(ctx, app.ID, userID, card); err != nil {
		logging.Errorw(ctx, "failed to upsert business card", "err", err, "userID", userID)
		return nil, err
	}

	// If no resume exists, seed one from the card data
	if _, err := s.r.Get(ctx, app.ID, userID); err == sql.ErrNoRows {
		content := &models.ResumeContent{
			RealName:            card.RealName,
			Position:            card.Position,
			Departments:         card.Departments,
			CurrentOrganization: card.CurrentOrganization,
			CurrentJobTitle:     card.CurrentJobTitle,
		}
		if _, err := s.r.Create(ctx, app.ID, userID, content); err != nil {
			logging.Errorw(ctx, "failed to seed resume from card", "err", err, "userID", userID)
		}
	} else if err != nil {
		logging.Errorw(ctx, "failed to get resume for card sync", "err", err, "userID", userID)
	}

	return card, nil
}
