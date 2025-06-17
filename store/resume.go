package store

import (
	"context"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type resumeStore struct {
	db *sqlx.DB
}

func NewResume(db *sqlx.DB) Resume {
	return &resumeStore{db: db}
}

func (s *resumeStore) Create(ctx context.Context, userID string, content *models.ResumeContent) (*models.Resume, error) {
	resumeID := uuid.New().String()
	now := time.Now()
	query := `
	INSERT INTO public.resume (
		id,
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
		?
	)
	`
	query = s.db.Rebind(query)

	var returnedID string
	_, err := s.db.Exec(query,
		resumeID,
		userID,
		content,
		now,
		now,
	)
	if err != nil {
		return nil, err
	}

	return &models.Resume{
		ID:        returnedID,
		UserID:    userID,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *resumeStore) Get(ctx context.Context, userID string) (*models.Resume, error) {
	query := `
	SELECT 
		id,
		user_id,
		content,
		created_at,
		updated_at
	FROM public.resume 
	WHERE user_id = ?
	`
	query = s.db.Rebind(query)

	var resume models.Resume
	err := s.db.QueryRowx(query, userID).Scan(
		&resume.ID,
		&resume.UserID,
		&resume.Content,
		&resume.CreatedAt,
		&resume.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &resume, nil
}

func (s *resumeStore) Update(ctx context.Context, userID string, content *models.ResumeContent) error {
	query := `
	UPDATE public.resume 
	SET	content = ?,
		updated_at = ?
	WHERE user_id = ?
	`
	query = s.db.Rebind(query)

	_, err := s.db.Exec(query, content, time.Now(), userID)
	if err != nil {
		return err
	}

	return nil
}

func (s *resumeStore) CreateSnapshot(ctx context.Context, userID string) (*models.ResumeSnapshot, error) {
	snapshotID := uuid.New().String()
	now := time.Now()

	resume, err := s.Get(ctx, userID)
	if err != nil {
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
		return nil, err
	}

	return &snapshot, nil
}

func (s *resumeStore) CreateRelation(ctx context.Context, userID string, snapshotID string, chatID string, postID string) (*models.ResumeRelation, error) {
	relationID := uuid.New().String()
	now := time.Now()

	query := `
	INSERT INTO public.resume_relation (
		id,
		user_id,
		snapshot_id,
		post_id,
		chat_id,
		created_at
	)
	VALUES (
		?,
		?,
		?,
		?,
		?
	)
	`
	query = s.db.Rebind(query)

	_, err := s.db.Exec(query, relationID, snapshotID, chatID, postID, userID, now)
	if err != nil {
		return nil, err
	}

	return &models.ResumeRelation{
		ID:         relationID,
		UserID:     userID,
		SnapshotID: snapshotID,
		PostID:     postID,
		ChatID:     chatID,
		CreatedAt:  now,
	}, nil
}

func (s *resumeStore) GetRelation(ctx context.Context, chatID string) (*models.ResumeRelation, error) {
	query := `
	SELECT 
		id,
		user_id,
		snapshot_id,
		post_id,
		chat_id,
		created_at
	FROM public.resume_relation
	WHERE chat_id = ?
	`
	query = s.db.Rebind(query)

	var relation models.ResumeRelation
	err := s.db.QueryRowx(query, chatID).Scan(
		&relation.ID,
		&relation.UserID,
		&relation.SnapshotID,
		&relation.PostID,
		&relation.ChatID,
		&relation.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &relation, nil
}
