package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type resumeStore struct {
	db *sqlx.DB
}

func NewResume(db *sqlx.DB) Resume {
	return &resumeStore{db: db}
}

func (s *resumeStore) Create(ctx context.Context, appID, userID string, content *models.ResumeContent) (*models.Resume, error) {
	resumeID := uuid.New().String()
	now := time.Now()
	query := `
	INSERT INTO public.resume (
		id,
		app_id,
		user_id,
		content,
		created_at,
		updated_at
	)
	VALUES (
		?,
		?,
		?,
		?,
		?,
		?
	)
	`
	query = s.db.Rebind(query)

	_, err := s.db.Exec(query,
		resumeID,
		appID,
		userID,
		content,
		now,
		now,
	)
	if err != nil {
		logging.Errorw(ctx, "failed to create resume", "err", err, "appID", appID, "userID", userID)
		return nil, err
	}

	return &models.Resume{
		ID:        resumeID,
		AppID:     appID,
		UserID:    userID,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *resumeStore) Get(ctx context.Context, appID, userID string) (*models.Resume, error) {
	query := `
	SELECT 
		id,
		app_id,
		user_id,
		content,
		created_at,
		updated_at
	FROM public.resume 
	WHERE app_id = ? AND user_id = ?
	`
	query = s.db.Rebind(query)

	var resume models.Resume
	err := s.db.QueryRowx(query, appID, userID).Scan(
		&resume.ID,
		&resume.AppID,
		&resume.UserID,
		&resume.Content,
		&resume.CreatedAt,
		&resume.UpdatedAt,
	)
	if err != nil {
		logging.Errorw(ctx, "failed to get resume", "err", err, "appID", appID, "userID", userID)
		return nil, err
	}

	return &resume, nil
}

func (s *resumeStore) GetUserAppliedPostIDs(ctx context.Context, appID, userID string) ([]string, error) {
	query := `
	SELECT 
		post_id
	FROM public.resume_relation
	WHERE 
		app_id = $1 
		AND 
		user_id = $2
	`
	query = s.db.Rebind(query)

	postIDs := []string{}
	err := s.db.Select(&postIDs, query, appID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to get applied post ids", "err", err, "appID", appID, "userID", userID)
		return nil, err
	}
	return postIDs, nil
}

func (s *resumeStore) Update(ctx context.Context, appID, userID string, content *models.ResumeContent) error {
	query := `
	UPDATE public.resume 
	SET	content = ?,
		updated_at = ?
	WHERE app_id = ? AND user_id = ?
	`
	query = s.db.Rebind(query)

	_, err := s.db.Exec(query, content, time.Now(), appID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to update resume", "err", err, "appID", appID, "userID", userID)
		return err
	}

	return nil
}

func (s *resumeStore) CreateSnapshot(ctx context.Context, appID, userID string) (*models.ResumeSnapshot, error) {
	snapshotID := uuid.New().String()
	now := time.Now()

	resume, err := s.Get(ctx, appID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to get resume for snapshot", "err", err, "appID", appID, "userID", userID)
		return nil, err
	}

	query := `
	INSERT INTO public.resume_snapshot (
		id,
		resume_id,
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

	_, err = s.db.Exec(query,
		snapshotID,
		resume.ID,
		resume.Content,
		now,
	)
	if err != nil {
		logging.Errorw(ctx, "failed to create resume snapshot", "err", err, "snapshotID", snapshotID, "resumeID", resume.ID)
		return nil, err
	}

	return &models.ResumeSnapshot{
		ID:        snapshotID,
		ResumeID:  resume.ID,
		Content:   resume.Content,
		CreatedAt: now,
	}, nil
}

func (s *resumeStore) GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error) {
	query := `
	SELECT 
		id,
		resume_id,
		content,
		created_at
	FROM public.resume_snapshot 
	WHERE id = ?
	`
	query = s.db.Rebind(query)

	var snapshot models.ResumeSnapshot
	err := s.db.QueryRowx(query, snapshotID).Scan(
		&snapshot.ID,
		&snapshot.ResumeID,
		&snapshot.Content,
		&snapshot.CreatedAt,
	)
	if err != nil {
		logging.Errorw(ctx, "failed to get resume snapshot", "err", err, "snapshotID", snapshotID)
		return nil, err
	}

	return &snapshot, nil
}

func (s *resumeStore) ListSnapshots(ctx context.Context, snapshotIDs []string) ([]*models.ResumeSnapshot, error) {
	query := `
	SELECT 
		id,
		resume_id,
		content,
		created_at
	FROM public.resume_snapshot
	WHERE id = ANY(?)
	`
	query = s.db.Rebind(query)

	var snapshots []*models.ResumeSnapshot
	err := s.db.Select(&snapshots, query, pq.Array(snapshotIDs))
	if err != nil {
		logging.Errorw(ctx, "failed to list resume snapshots", "err", err, "snapshotIDs", snapshotIDs)
		return nil, err
	}
	return snapshots, nil
}

func (s *resumeStore) CreateRelation(ctx context.Context, appID, userID string, snapshotID string, chatID string, postID string, status models.ResumeStatus) (*models.ResumeRelation, error) {
	relationID := uuid.New().String()
	now := time.Now()

	query := `
	INSERT INTO public.resume_relation (
		id,
		app_id,
		user_id,
		snapshot_id,
		post_id,
		chat_id,
		created_at,
		updated_at,
		status
	)
	VALUES (
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?
	)
	`
	query = s.db.Rebind(query)

	_, err := s.db.Exec(query, relationID, appID, userID, snapshotID, postID, chatID, now, now, status)
	if err != nil {
		logging.Errorw(ctx, "failed to create resume relation", "err", err, "snapshotID", snapshotID, "chatID", chatID, "postID", postID)
		return nil, err
	}

	return &models.ResumeRelation{
		ID:         relationID,
		AppID:      appID,
		UserID:     userID,
		SnapshotID: snapshotID,
		PostID:     postID,
		ChatID:     chatID,
		CreatedAt:  now,
		UpdatedAt:  now,
		Status:     status,
	}, nil
}

func (s *resumeStore) GetRelation(ctx context.Context, opts ...models.GetRelationOptionFunc) (*models.ResumeRelation, error) {
	opt := models.GetRelationOption{}
	for _, f := range opts {
		if err := f(&opt); err != nil {
			logging.Errorw(ctx, "failed to apply get relation option", "err", err)
			return nil, err
		}
	}

	query := `
	SELECT 
		id,
		app_id,
		user_id,
		snapshot_id,
		post_id,
		chat_id,
		is_read,
		created_at,
		updated_at,
		status
	FROM public.resume_relation`

	conditions := []string{}
	params := []interface{}{}

	if opt.ChatID != nil {
		conditions = append(conditions, "chat_id=?")
		params = append(params, *opt.ChatID)
	}
	if opt.SnapshotID != nil {
		conditions = append(conditions, "snapshot_id=?")
		params = append(params, *opt.SnapshotID)
	}
	if opt.UserID != nil {
		conditions = append(conditions, "user_id=?")
		params = append(params, *opt.UserID)
	}
	if opt.PostID != nil {
		conditions = append(conditions, "post_id=?")
		params = append(params, *opt.PostID)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	} else {
		logging.Errorw(ctx, "at least one filter condition is required")
		return nil, models.ErrorWrongParams
	}

	query = s.db.Rebind(query)

	var relation models.ResumeRelation
	err := s.db.QueryRowx(query, params...).Scan(
		&relation.ID,
		&relation.AppID,
		&relation.UserID,
		&relation.SnapshotID,
		&relation.PostID,
		&relation.ChatID,
		&relation.IsRead,
		&relation.CreatedAt,
		&relation.UpdatedAt,
		&relation.Status,
	)
	if err != nil {
		if err != sql.ErrNoRows {
			logging.Errorw(ctx, "failed to get resume relation", "err", err, "opts", opts)
		}
		return nil, err
	}

	return &relation, nil
}

func (s *resumeStore) ListRelations(ctx context.Context, appID string, opts ...models.ListRelationOptionFunc) ([]*models.ResumeRelation, error) {
	opt := models.ListRelationOption{}
	for _, f := range opts {
		if err := f(&opt); err != nil {
			logging.Errorw(ctx, "failed to apply list relation option", "err", err)
			return nil, err
		}
	}

	query := `
	SELECT
		id,
		app_id,
		user_id,
		snapshot_id,
		post_id,
		chat_id,
		is_read,
		created_at,
		updated_at,
		status
	FROM public.resume_relation
	WHERE app_id = ?`

	var args []interface{}
	args = append(args, appID)

	if opt.After != nil {
		query += ` AND created_at >= ?`
		args = append(args, *opt.After)
	}

	if len(opt.ChatIDs) > 0 {
		query += ` AND chat_id = ANY(?)`
		args = append(args, pq.Array(opt.ChatIDs))
	}

	query = s.db.Rebind(query)

	var relations []*models.ResumeRelation
	err := s.db.Select(&relations, query, args...)
	if err != nil {
		logging.Errorw(ctx, "failed to list resume relations", "err", err, "appID", appID, "opts", opts)
		return nil, err
	}
	return relations, nil
}

func (s *resumeStore) Read(ctx context.Context, snapshotID string) error {
	query := `
	UPDATE public.resume_relation
	SET is_read=true, updated_at=?
	WHERE snapshot_id=?
	`
	query = s.db.Rebind(query)
	_, err := s.db.Exec(query, time.Now(), snapshotID)
	if err != nil {
		logging.Errorw(ctx, "failed to update resume relation read status", "err", err, "chatID", snapshotID)
		return err
	}

	return nil
}

func (s *resumeStore) UpdateRelationStatus(ctx context.Context, snapshotID string, status models.ResumeStatus) error {
	if _, err := s.GetRelation(ctx, models.BySnapshot(snapshotID)); err != nil {
		logging.Errorw(ctx, "failed to get resume relation", "err", err, "snapshotID", snapshotID)
		return err
	}

	query := `
	UPDATE public.resume_relation
	SET status=?
	WHERE snapshot_id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.Exec(query, status, snapshotID); err != nil {
		logging.Errorw(ctx, "failed to update resume relation status", "err", err, "snapshotID", snapshotID)
		return err
	}

	return nil
}

func (s *resumeStore) UpdateRelationListStatus(ctx context.Context, postIDs []string, status models.ResumeStatus) error {
	query := `
	UPDATE public.resume_relation
	SET status=?
	WHERE post_id = ANY(?)
	`
	query = s.db.Rebind(query)
	if _, err := s.db.Exec(query, status, pq.Array(postIDs)); err != nil {
		logging.Errorw(ctx, "failed to update resume relation status", "err", err, "postIDs", postIDs)
		return err
	}

	return nil
}
