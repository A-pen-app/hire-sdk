package service

import (
	"context"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

type agreementService struct {
	a  store.App
	am store.Agreement
}

func NewAgreement(a store.App, am store.Agreement) Agreement {
	return &agreementService{a: a, am: am}
}

func (s *agreementService) Agree(ctx context.Context, bundleID, userID, version string) error {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		return err
	}

	return s.am.Agree(ctx, app.ID, userID, version)
}

func (s *agreementService) Get(ctx context.Context, bundleID, userID string) (*models.AgreementRecord, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		return nil, err
	}

	return s.am.Get(ctx, app.ID, userID)
}
