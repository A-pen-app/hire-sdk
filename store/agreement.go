package store

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/jmoiron/sqlx"
)

type agreementStore struct {
	db *sqlx.DB
}

func NewAgreement(db *sqlx.DB) Agreement {
	return &agreementStore{db: db}
}

func (as *agreementStore) Agree(ctx context.Context, appID, userID, version string) error {
	query := `
	INSERT INTO public.agreement (
		app_id,
		user_id,
		version_agreed,
		agreed_at
	)
	VALUES (
		?,
		?,
		?,
		now()
	)
	`

	query = as.db.Rebind(query)
	_, err := as.db.Exec(query,
		appID,
		userID,
		version,
		// now()
	)
	if err != nil {
		logging.Errorw(ctx, "insert new agreement record failed", "err", err, "user_id", userID, "version", version)
		return err
	}
	return nil
}

func (as *agreementStore) Get(ctx context.Context, appID, userID string) (*models.AgreementRecord, error) {

	r := models.AgreementRecord{}

	query := `
	SELECT eula FROM public.version WHERE id=1
	`

	if err := as.db.QueryRowx(query).StructScan(&r); err != nil {
		logging.Errorw(ctx, "get latest eula version failed", "err", err)
		return nil, err
	}

	query = `
	SELECT
		version_agreed,
		agreed_at
	FROM public.agreement
	WHERE app_id=? AND user_id=?
	ORDER BY agreed_at DESC
	LIMIT 1
	`
	query = as.db.Rebind(query)
	if err := as.db.QueryRowx(query, userID).StructScan(&r); err != nil {
		if err == sql.ErrNoRows {
			r.VersionAgreed = nil
			r.AgreedAt = nil
			return &r, nil
		}
		logging.Errorw(ctx, "get user agreement record failed", "err", err, "user_id", userID)
		return nil, err
	}

	return &r, nil
}
