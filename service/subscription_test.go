package service

import (
	"context"
	"testing"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

type fakeApps struct{ store.App }

func (fakeApps) GetByBundleID(context.Context, string) (*models.App, error) {
	return &models.App{ID: "app"}, nil
}

// fakeSubStore answers Get with one stored row and records what Update was asked to write.
type fakeSubStore struct {
	store.Subscription

	sub *models.UserSubscription

	called    bool
	gotStatus models.SubscriptionStatus
	gotExpiry *time.Time
}

func (f *fakeSubStore) Get(context.Context, string, string) (*models.UserSubscription, error) {
	return f.sub, nil
}

func (f *fakeSubStore) Update(_ context.Context, _, _ string, status models.SubscriptionStatus, expiresAt *time.Time) error {
	f.called = true
	f.gotStatus = status
	f.gotExpiry = expiresAt
	return nil
}

func TestSubscriptionUpdate_Expiry(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name       string
		status     models.SubscriptionStatus
		expiredAt  *time.Time
		wantErr    error
		wantStatus models.SubscriptionStatus
		wantExpiry *time.Time
	}{
		{
			name:      "subscribed without expiry is rejected",
			status:    models.SubscriptionSubscribed,
			expiredAt: nil,
			wantErr:   models.ErrorWrongParams,
		},
		{
			name:       "paused without expiry is stored as is",
			status:     models.SubscriptionSubscribed | models.SubscriptionPaused,
			expiredAt:  nil,
			wantStatus: models.SubscriptionSubscribed | models.SubscriptionPaused,
			wantExpiry: nil,
		},
		{
			name:       "subscribed with future expiry is stored as is",
			status:     models.SubscriptionSubscribed,
			expiredAt:  &future,
			wantStatus: models.SubscriptionSubscribed,
			wantExpiry: &future,
		},
		{
			name:       "subscribed with past expiry becomes none",
			status:     models.SubscriptionSubscribed,
			expiredAt:  &past,
			wantStatus: models.SubscriptionNone,
			wantExpiry: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeSubStore{}
			svc := NewSubscription(fakeApps{}, st)

			err := svc.Update(context.Background(), "bundle", "user", tt.status, tt.expiredAt)
			if err != tt.wantErr {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if st.called {
					t.Fatal("store must not be written on rejection")
				}
				return
			}
			if !st.called {
				t.Fatal("store was not written")
			}
			if st.gotStatus != tt.wantStatus {
				t.Errorf("status = %d, want %d", st.gotStatus, tt.wantStatus)
			}
			if (st.gotExpiry == nil) != (tt.wantExpiry == nil) || (st.gotExpiry != nil && !st.gotExpiry.Equal(*tt.wantExpiry)) {
				t.Errorf("expiry = %v, want %v", st.gotExpiry, tt.wantExpiry)
			}
		})
	}
}

func TestSubscriptionGet_Expiry(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name       string
		stored     *models.UserSubscription
		wantErr    bool
		wantStatus models.SubscriptionStatus
	}{
		{
			name:       "paused without expiry is returned as is",
			stored:     &models.UserSubscription{Status: models.SubscriptionSubscribed | models.SubscriptionPaused},
			wantStatus: models.SubscriptionSubscribed | models.SubscriptionPaused,
		},
		{
			name:    "subscribed without expiry is an error",
			stored:  &models.UserSubscription{Status: models.SubscriptionSubscribed},
			wantErr: true,
		},
		{
			name:       "subscribed with past expiry becomes none",
			stored:     &models.UserSubscription{Status: models.SubscriptionSubscribed, ExpiresAt: &past},
			wantStatus: models.SubscriptionNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSubscription(fakeApps{}, &fakeSubStore{sub: tt.stored})

			got, err := svc.Get(context.Background(), "bundle", "user")
			if tt.wantErr {
				if err == nil {
					t.Fatal("err = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %d, want %d", got.Status, tt.wantStatus)
			}
		})
	}
}
