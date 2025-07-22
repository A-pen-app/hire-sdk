package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
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
		if err != sql.ErrNoRows {
			logging.Errorw(ctx, "get user subscription failed", "err", err, "app_id", appID, "user_id", userID)
		}
		return nil, err
	}

	return subscription, nil
}

func (ss *subscriptionStore) List(ctx context.Context, appID string, userIDs []string) ([]*models.UserSubscription, error) {
	subscriptions := []*models.UserSubscription{}

	// Handle empty userIDs slice
	if len(userIDs) == 0 {
		return subscriptions, nil
	}

	query := `
	SELECT 
		app_id,
		user_id,	
		status,
		expires_at,
		created_at,
		updated_at
	FROM public.user_subscription 
	WHERE app_id = ? AND user_id = ANY(?)
	`

	query = ss.db.Rebind(query)
	err := ss.db.Select(&subscriptions, query, appID, pq.Array(userIDs))
	if err != nil {
		logging.Errorw(ctx, "get user subscriptions failed", "err", err, "app_id", appID, "user_ids", userIDs)
		return nil, err
	}

	return subscriptions, nil
}

func (ss *subscriptionStore) Update(ctx context.Context, appID, userID string, status models.SubscriptionStatus, expiresAt *time.Time) error {
	query := `
	INSERT INTO public.user_subscription (
		app_id,
		user_id,
		status,
		expires_at,
		created_at,
		updated_at
	)
	VALUES (?, ?, ?, ?, now(), now())
	ON CONFLICT (app_id, user_id)
	DO UPDATE SET
		status = EXCLUDED.status,
		expires_at = EXCLUDED.expires_at,
		updated_at = now()
	`

	query = ss.db.Rebind(query)
	_, err := ss.db.ExecContext(ctx, query,
		appID,
		userID,
		status,
		expiresAt,
	)
	if err != nil {
		logging.Errorw(ctx, "upsert user subscription failed", "err", err, "app_id", appID, "user_id", userID, "status", status)
		return err
	}

	return nil
}
