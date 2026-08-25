package service

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

// The chat list and the received-resume list answer different questions about
// the same pair of rows, so the rule that lifts a lock lives here once: a job
// seeker always sees their own application, a live subscription lifts every
// lock, and otherwise the stored value stands.

// liftsLock reports whether the viewer sees past a stored lock.
func liftsLock(isJobSeeker, subscribed bool) bool {
	return isJobSeeker || subscribed
}

// subscribed reads a failed lookup as "not subscribed": the stored status still
// stands, so the worst case is a lock a reload lifts.
func subscribed(ctx context.Context, s store.Subscription, appID, viewerID string) bool {
	sub, err := s.Get(ctx, appID, viewerID)
	if err != nil && err != sql.ErrNoRows {
		logging.Errorw(ctx, "failed to get subscription", "err", err, "appID", appID, "viewerID", viewerID)
		return false
	}
	return sub != nil && sub.Status.HasOneOf(models.SubscriptionSubscribed)
}
