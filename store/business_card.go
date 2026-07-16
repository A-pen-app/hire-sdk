package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type businessCard struct {
	db *sqlx.DB
}

func NewBusinessCard(db *sqlx.DB) BusinessCard {
	return &businessCard{db: db}
}

func (s *businessCard) Get(ctx context.Context, appID, userID string) (*models.BusinessCard, error) {
	query := `
	SELECT
		id,
		app_id,
		user_id,
		content,
		created_at,
		updated_at
	FROM public.business_card
	WHERE app_id = ? AND user_id = ?
	`
	query = s.db.Rebind(query)

	var record models.BusinessCard
	err := s.db.QueryRowxContext(ctx, query, appID, userID).Scan(
		&record.ID,
		&record.AppID,
		&record.UserID,
		&record.Content,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

// Upsert writes the user's business card. PreferredLocations is dual-written:
// when the card carries a non-nil value, the user's resume (if any) gets its
// preferred_locations replaced in the same transaction so the two surfaces
// stay in sync. A nil value leaves the resume untouched, so callers unaware
// of locations can't wipe them.
func (s *businessCard) Upsert(ctx context.Context, appID, userID string, card *models.BusinessCardContent) error {
	now := time.Now()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		logging.Errorw(ctx, "failed to begin business card upsert tx", "err", err, "appID", appID, "userID", userID)
		return err
	}
	defer tx.Rollback()

	query := `
	INSERT INTO public.business_card (id, app_id, user_id, content, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT (app_id, user_id) DO UPDATE SET
		content = EXCLUDED.content,
		updated_at = EXCLUDED.updated_at
	`
	query = tx.Rebind(query)

	id := uuid.New().String()
	if _, err := tx.ExecContext(ctx, query, id, appID, userID, card, now, now); err != nil {
		logging.Errorw(ctx, "failed to upsert business card", "err", err, "appID", appID, "userID", userID)
		return err
	}

	if card != nil && card.PreferredLocations != nil {
		locations, err := json.Marshal(card.PreferredLocations)
		if err != nil {
			logging.Errorw(ctx, "failed to marshal preferred locations", "err", err, "appID", appID, "userID", userID)
			return err
		}
		// No-op when the user has no resume yet; the service layer seeds one.
		query := `
		UPDATE public.resume
		SET content = jsonb_set(COALESCE(content, '{}'::jsonb), '{preferred_locations}', ?::jsonb),
			updated_at = ?
		WHERE app_id = ? AND user_id = ?
		`
		query = tx.Rebind(query)
		if _, err := tx.ExecContext(ctx, query, string(locations), now, appID, userID); err != nil {
			logging.Errorw(ctx, "failed to sync preferred locations to resume", "err", err, "appID", appID, "userID", userID)
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		logging.Errorw(ctx, "failed to commit business card upsert tx", "err", err, "appID", appID, "userID", userID)
		return err
	}
	return nil
}

func (s *businessCard) CreateSnapshot(ctx context.Context, appID, userID string, card *models.BusinessCardContent) (*models.BusinessCardSnapshot, error) {
	snapshotID := uuid.New().String()
	now := time.Now()

	query := `
	INSERT INTO public.business_card_snapshot (
		id,
		business_card_id,
		content,
		created_at
	)
	VALUES (
		?,
		?,
		?,
		?
	)
	`
	query = s.db.Rebind(query)

	// Look up the business card ID
	bc, err := s.Get(ctx, appID, userID)
	var businessCardID string
	if err == nil {
		businessCardID = bc.ID
	}

	_, err = s.db.ExecContext(ctx, query,
		snapshotID,
		businessCardID,
		card,
		now,
	)
	if err != nil {
		logging.Errorw(ctx, "failed to create business card snapshot", "err", err, "appID", appID, "userID", userID)
		return nil, err
	}

	return &models.BusinessCardSnapshot{
		ID:             snapshotID,
		BusinessCardID: businessCardID,
		Content:        card,
		CreatedAt:      now,
	}, nil
}

func (s *businessCard) GetSnapshot(ctx context.Context, snapshotID string) (*models.BusinessCardSnapshot, error) {
	query := `
	SELECT
		id,
		business_card_id,
		content,
		created_at
	FROM public.business_card_snapshot
	WHERE id = ?
	`
	query = s.db.Rebind(query)

	var snapshot models.BusinessCardSnapshot
	err := s.db.QueryRowxContext(ctx, query, snapshotID).Scan(
		&snapshot.ID,
		&snapshot.BusinessCardID,
		&snapshot.Content,
		&snapshot.CreatedAt,
	)
	if err != nil {
		logging.Errorw(ctx, "failed to get business card snapshot", "err", err, "snapshotID", snapshotID)
		return nil, err
	}

	return &snapshot, nil
}

func (s *businessCard) ListSnapshots(ctx context.Context, snapshotIDs []string) ([]*models.BusinessCardSnapshot, error) {
	if len(snapshotIDs) == 0 {
		return nil, nil
	}

	query := `
	SELECT id, business_card_id, content, created_at
	FROM public.business_card_snapshot
	WHERE id = ANY(?)
	`
	query = s.db.Rebind(query)

	var snapshots []*models.BusinessCardSnapshot
	if err := s.db.SelectContext(ctx, &snapshots, query, pq.Array(snapshotIDs)); err != nil {
		logging.Errorw(ctx, "failed to list business card snapshots", "err", err, "snapshotIDs", snapshotIDs)
		return nil, err
	}
	return snapshots, nil
}

func (s *businessCard) GetSnapshotOwners(ctx context.Context, snapshotIDs []string) (map[string]string, error) {
	if len(snapshotIDs) == 0 {
		return map[string]string{}, nil
	}

	type row struct {
		SnapshotID string `db:"snapshot_id"`
		UserID     string `db:"user_id"`
	}

	query := `
	SELECT bcs.id AS snapshot_id, bc.user_id
	FROM public.business_card_snapshot bcs
	JOIN public.business_card bc ON bcs.business_card_id = bc.id
	WHERE bcs.id = ANY(?)
	`
	query = s.db.Rebind(query)

	var rows []row
	if err := s.db.SelectContext(ctx, &rows, query, pq.Array(snapshotIDs)); err != nil {
		logging.Errorw(ctx, "failed to get user ids by snapshot ids", "err", err, "snapshotIDs", snapshotIDs)
		return nil, err
	}

	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.SnapshotID] = r.UserID
	}
	return result, nil
}
