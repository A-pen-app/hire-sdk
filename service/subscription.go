package service

import (
	"context"
	"errors"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

type subscriptionService struct {
	a store.App
	s store.Subscription
}

func NewSubscription(a store.App, s store.Subscription) Subscription {
	return &subscriptionService{a: a, s: s}
}

func (s *subscriptionService) Get(ctx context.Context, bundleID, userID string) (*models.UserSubscription, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "get app by bundle id failed", "err", err, "bundle_id", bundleID)
		return nil, err
	}

	subscription, err := s.s.Get(ctx, app.ID, userID)
	if err != nil {
		logging.Errorw(ctx, "get subscription failed", "err", err, "app_id", app.ID, "user_id", userID)
		return nil, err
	}

	if subscription.Status.HasOneOf(models.SubscriptionSubscribed) {
		if subscription.ExpiresAt != nil {
			logging.Errorw(ctx, "subscription expires at is nil", "app_id", app.ID, "user_id", userID)
			return nil, errors.New("subscription expires at is nil")
		}

		if subscription.ExpiresAt.Before(time.Now()) {
			subscription.Status = (subscription.Status &^ models.SubscriptionSubscribed) | models.SubscriptionNone
		}
	}

	return subscription, nil
}

func (s *subscriptionService) Update(ctx context.Context, bundleID, userID string, status models.SubscriptionStatus, expiredAt *time.Time) error {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "get app by bundle id failed", "err", err, "bundle_id", bundleID)
		return err
	}

	if status.HasOneOf(models.SubscriptionSubscribed) {
		if expiredAt == nil {
			logging.Errorw(ctx, "expired at is nil", "app_id", app.ID, "user_id", userID)
			return models.ErrorWrongParams
		}

		if expiredAt.Before(time.Now()) {
			status = (status &^ models.SubscriptionSubscribed) | models.SubscriptionNone
			expiredAt = nil
		}
	}

	if status.HasOneOf(models.SubscriptionNone) {
		expiredAt = nil
	}

	return s.s.Update(ctx, app.ID, userID, status, expiredAt)
}
