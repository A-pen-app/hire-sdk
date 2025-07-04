package store

import (
	"context"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
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

func (s *resumeStore) CreateRelation(ctx context.Context, appID, userID string, snapshotID string, chatID string, postID string) (*models.ResumeRelation, error) {
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
		updated_at
	)
	VALUES (
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

	_, err := s.db.Exec(query, relationID, appID, userID, snapshotID, postID, chatID, now, now)
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
	}, nil
}

func (s *resumeStore) GetRelation(ctx context.Context, chatID string) (*models.ResumeRelation, error) {
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
		updated_at
	FROM public.resume_relation
	WHERE chat_id = ?
	`
	query = s.db.Rebind(query)

	var relation models.ResumeRelation
	err := s.db.QueryRowx(query, chatID).Scan(
		&relation.ID,
		&relation.AppID,
		&relation.UserID,
		&relation.SnapshotID,
		&relation.PostID,
		&relation.ChatID,
		&relation.IsRead,
		&relation.CreatedAt,
		&relation.UpdatedAt,
	)
	if err != nil {
		logging.Errorw(ctx, "failed to get resume relation", "err", err, "chatID", chatID)
		return nil, err
	}

	return &relation, nil
}

func (s *resumeStore) Read(ctx context.Context, chatID string) error {
	query := `
	UPDATE public.resume_relation
	SET is_read=true, updated_at=?
	WHERE chat_id=?
	`
	query = s.db.Rebind(query)
	_, err := s.db.Exec(query, time.Now(), chatID)
	if err != nil {
		logging.Errorw(ctx, "failed to update resume relation read status", "err", err, "chatID", chatID)
		return err
	}

	return nil
}
