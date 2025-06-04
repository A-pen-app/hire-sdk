package store

import (
	"context"

	"github.com/A-pen-app/resume-sdk/models"
	"github.com/jmoiron/sqlx"
)

type resumeStore struct {
	db *sqlx.DB
}

func NewResume(ctx context.Context, db *sqlx.DB) Resume {
	return &resumeStore{db: db}
}

func (s *resumeStore) Create(ctx context.Context, userID string, content *models.ResumeContent) (*models.Resume, error) {
	return nil, nil
}

func (s *resumeStore) Get(ctx context.Context, userID string) (*models.Resume, error) {
	return nil, nil
}

func (s *resumeStore) Update(ctx context.Context, userID string, content *models.ResumeContent) error {
	return nil
}

func (s *resumeStore) CreateSnapshot(ctx context.Context, userID string, chatID string) (*models.ResumeSnapshot, error) {
	return nil, nil
}

func (s *resumeStore) GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error) {
	return nil, nil
}
