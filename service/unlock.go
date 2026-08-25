package service

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

// liftsLock reports whether the viewer sees past a stored lock: their own
// application always, a live subscription otherwise. Both screens share it, so
// they cannot answer differently. subscribed is deferred — an owner costs no
// subscription lookup.
func liftsLock(isOwner bool, subscribed func() bool) bool {
	return isOwner || subscribed()
}

// subscribed reads a failed lookup as "not subscribed": the stored status
// stands, so the worst case is a lock a reload lifts.
func subscribed(ctx context.Context, s store.Subscription, appID, viewerID string) bool {
	sub, err := s.Get(ctx, appID, viewerID)
	if err != nil && err != sql.ErrNoRows {
		logging.Errorw(ctx, "failed to get subscription", "err", err, "appID", appID, "viewerID", viewerID)
		return false
	}
	return sub != nil && sub.Status.HasOneOf(models.SubscriptionSubscribed)
}
