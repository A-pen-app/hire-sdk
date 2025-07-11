package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/jmoiron/sqlx"
)

type subscriptionStore struct {
	db *sqlx.DB
}

func NewSubscription(db *sqlx.DB) Subscription {
	return &subscriptionStore{db: db}
}

func (ss *subscriptionStore) Get(ctx context.Context, appID, userID string) (*models.UserSubscription, error) {
	subscription := &models.UserSubscription{}

	query := `
	SELECT 
		app_id,
		user_id,
		status,
		expires_at,
		created_at,
		updated_at
	FROM public.user_subscription 
	WHERE app_id = ? AND user_id = ?
	`

	query = ss.db.Rebind(query)
	if err := ss.db.QueryRowx(query, appID, userID).StructScan(subscription); err != nil {
		if err == sql.ErrNoRows {
			defaultSub := &models.UserSubscription{
				AppID:     appID,
				UserID:    userID,
				Status:    models.SubscriptionNone,
				ExpiresAt: nil,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			insertQuery := `
			INSERT INTO public.user_subscription (
				app_id,
				user_id,
				status,
				expires_at,
				created_at,
				updated_at
			)
			VALUES (?, ?, ?, ?, now(), now())
			`
			insertQuery = ss.db.Rebind(insertQuery)
			_, insertErr := ss.db.ExecContext(ctx, insertQuery, appID, userID, models.SubscriptionNone, nil)
			if insertErr != nil {
				logging.Errorw(ctx, "insert default user subscription failed", "err", insertErr, "app_id", appID, "user_id", userID)
				return nil, insertErr
			}

			return defaultSub, nil
		}
		logging.Errorw(ctx, "get user subscription failed", "err", err, "app_id", appID, "user_id", userID)
		return nil, err
	}

	return subscription, nil
}

func (ss *subscriptionStore) Update(ctx context.Context, appID, userID string, status models.SubscriptionStatus, expiresAt *time.Time) error {

	query := `
	UPDATE public.user_subscription 
	SET 
		status = ?,
		expires_at = ?,
		updated_at = now()
	WHERE app_id = ? AND user_id = ?
	`

	query = ss.db.Rebind(query)
	_, err := ss.db.ExecContext(ctx, query,
		status,
		expiresAt,
		appID,
		userID,
	)
	if err != nil {
		logging.Errorw(ctx, "update user subscription failed", "err", err, "app_id", appID, "user_id", userID, "status", status)
		return err
	}

	return nil
}
