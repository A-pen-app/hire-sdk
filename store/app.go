package store

import (
	"context"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/jmoiron/sqlx"
)

type appStore struct {
	db *sqlx.DB
}

// NewApp returns an implementation of store.App
func NewApp(db *sqlx.DB) App {
	return &appStore{db: db}
}

func (a *appStore) GetByBundleID(ctx context.Context, bundleID string) (*models.App, error) {
	var app models.App
	query := `
	SELECT id, name, bundle_id
	FROM public.app
	WHERE bundle_id = ?
	`
	query = a.db.Rebind(query)

	err := a.db.QueryRowxContext(ctx, query, bundleID).StructScan(&app)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	return &app, nil
}
