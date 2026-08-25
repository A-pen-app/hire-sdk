package service

import (
	"context"
	"database/sql"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

// Who sees past a stored lock, on the chat screen and the received-resume screen
// alike: the owner of the application always, a live subscription otherwise. The
// check is one "||" at each site; what has to be shared is subscribed below,
// which decides what counts as live and which way a failed lookup falls.

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
