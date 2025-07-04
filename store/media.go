package store

import (
	"context"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type mediaStore struct {
	db *sqlx.DB
}

func NewMedia(db *sqlx.DB) Media {
	return &mediaStore{
		db: db,
	}
}

func (m *mediaStore) Get(ctx context.Context, mediaIDs []string) ([]*models.Media, error) {
	medias := []*models.Media{}
	if len(mediaIDs) == 0 {
		return medias, nil
	}
	query := `
	SELECT 
		id, 
		url, 
		placeholder, 
		type, 
		preview_url, 
		redirect_url, 
		title, 
		size, 
		expired_at 
	FROM public.media WHERE id=?`
	query = m.db.Rebind(query)
	for _, mediaID := range mediaIDs {
		media := models.Media{}
		if err := m.db.QueryRowx(query, mediaID).StructScan(&media); err != nil {
			logging.Errorw(ctx, "db query media failed", "err", err, "media_id", mediaID)
			continue
		}
		medias = append(medias, &media)

	}
	return medias, nil
}

func (m *mediaStore) New(ctx context.Context, upload *models.MediaUpload) (string, error) {
	if upload == nil {
		logging.Errorw(ctx, "upload parameter is nil")
		return "", models.ErrorWrongParams
	}

	mediaID := uuid.New().String()
	query := `
	INSERT INTO public.media (
		id,
		url,
		placeholder,
		type,
		preview_url,
		redirect_url,
		title,
		size,
		expired_at
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
	query = m.db.Rebind(query)
	if _, err := m.db.Exec(query,
		mediaID,
		upload.URL,
		upload.PreviewURL,
		upload.MediaType,
		upload.RedirectURL,
		upload.Title,
		upload.Size,
		upload.ExpiredAt,
	); err != nil {
		logging.Errorw(ctx, "insert new media failed", "err", err, "url", upload.URL, "preview_url", upload.PreviewURL, "media_type", upload.MediaType, "redirect_url", upload.RedirectURL, "title", upload.Title, "size", upload.Size, "expired_at", upload.ExpiredAt)
		return "", err
	}
	return mediaID, nil
}
