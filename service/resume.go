package service

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

type resumeService struct {
	r store.Resume
	a store.App
	c store.Chat
}

func NewResume(r store.Resume, a store.App, c store.Chat) Resume {
	return &resumeService{
		r: r,
		a: a,
		c: c,
	}
}

func (s *resumeService) Patch(ctx context.Context, bundleID, userID string, resume *models.ResumeContent) error {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return err
	}

	if err := s.r.Update(ctx, app.ID, userID, resume); err != nil {
		logging.Errorw(ctx, "failed to update resume", "err", err, "appID", app.ID, "userID", userID)
		return err
	}
	return nil
}

func (s *resumeService) Get(ctx context.Context, bundleID, userID string) (*models.Resume, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	resume, err := s.r.Get(ctx, app.ID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			resume, err = s.r.Create(ctx, app.ID, userID, &models.ResumeContent{})
			if err != nil {
				logging.Errorw(ctx, "failed to create resume", "err", err, "appID", app.ID, "userID", userID)
				return nil, err
			}
		} else {
			logging.Errorw(ctx, "failed to get resume", "err", err, "appID", app.ID, "userID", userID)
			return nil, err
		}
	}
	return resume, nil
}

func (s *resumeService) GetUserAppliedPostIDs(ctx context.Context, bundleID, userID string) ([]string, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	postIDs, err := s.r.GetUserAppliedPostIDs(ctx, app.ID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to get resume", "err", err, "appID", app.ID, "userID", userID)
		return nil, err
	}
	return postIDs, nil
}

func (s *resumeService) GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error) {
	snapshot, err := s.r.GetSnapshot(ctx, snapshotID)
	if err != nil {
		logging.Errorw(ctx, "failed to get resume snapshot", "err", err, "snapshotID", snapshotID)
		return nil, err
	}
	return snapshot, nil
}

func (s *resumeService) GetResponseMediansByPost(ctx context.Context, bundleID string, after time.Time) (map[string]float64, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	// Get all resume relations for this app (filtered by time)
	relations, err := s.r.ListRelations(ctx, app.ID, models.ByAfter(after))
	if err != nil {
		logging.Errorw(ctx, "failed to list resume relations", "err", err, "appID", app.ID)
		return nil, err
	}

	// Group relations by post_id and prepare job seeker IDs for batch query
	postRelations := make(map[string][]*models.ResumeRelation)
	opt := make([]models.FirstMessageOption, 0, len(relations))

	for _, relation := range relations {
		postRelations[relation.PostID] = append(postRelations[relation.PostID], relation)
		opt = append(opt, models.FirstMessageOption{ChatID: relation.ChatID, ExcludedSenderID: &relation.UserID})
	}

	// Batch fetch first employer messages for all chat rooms
	firstMessages, err := s.c.GetFirstMessages(ctx, opt)
	if err != nil {
		logging.Errorw(ctx, "failed to get first employer messages", "err", err)
		return nil, err
	}

	// Calculate median response time for each post
	result := make(map[string]float64)
	for postID, rels := range postRelations {
		samples := []float64{}

		for _, rel := range rels {
			// Check if employer has replied
			if firstEmployerMessage, exists := firstMessages[rel.ChatID]; exists && firstEmployerMessage != nil {
				responseTime := firstEmployerMessage.CreatedAt.Sub(rel.CreatedAt).Hours()
				samples = append(samples, responseTime)
			}
		}

		// Calculate median if we have samples
		if len(samples) > 0 {
			sort.Float64s(samples)
			n := len(samples)
			if n%2 == 0 {
				// Even number of samples: average of middle two
				result[postID] = (samples[n/2-1] + samples[n/2]) / 2
			} else {
				// Odd number of samples: middle value
				result[postID] = samples[n/2]
			}
		}
	}

	return result, nil
}
