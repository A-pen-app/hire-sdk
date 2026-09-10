package service

import (
	"context"
	"errors"
	"testing"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

// Embeds the interface so anything beyond List panics.
type fakeCardStore struct {
	store.BusinessCard

	cards []*models.BusinessCard
	err   error

	called     bool
	gotAppID   string
	gotUserIDs []string
}

func (f *fakeCardStore) List(_ context.Context, appID string, userIDs []string) ([]*models.BusinessCard, error) {
	f.called = true
	f.gotAppID = appID
	f.gotUserIDs = userIDs
	return f.cards, f.err
}

func card(userID string, realName *string) *models.BusinessCard {
	c := &models.BusinessCard{UserID: userID}
	if realName != nil {
		c.Content = &models.BusinessCardContent{RealName: realName}
	}
	return c
}

func TestBusinessCardListKeysByUserID(t *testing.T) {
	alice, bob := "Alice", "Bob"
	bc := &fakeCardStore{cards: []*models.BusinessCard{
		card("user-1", &alice),
		card("user-2", nil),
		card("user-3", &bob),
	}}
	s := NewBusinessCard(bc, nil, fakeAppStore{})

	got, err := s.List(context.Background(), "com.yoku.apen", []string{"user-1", "user-2", "user-3", "user-4"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("returned %d cards, want 2", len(got))
	}
	if got["user-1"] == nil || got["user-1"].RealName == nil || *got["user-1"].RealName != alice {
		t.Errorf("user-1 = %+v, want the stored content", got["user-1"])
	}
	if _, ok := got["user-2"]; ok {
		t.Error("a nil-content row must be skipped, not stored as a nil entry")
	}
	if _, ok := got["user-4"]; ok {
		t.Error("user-4 has no card, so it must be absent from the map")
	}
	if bc.gotAppID != "app-1" {
		t.Errorf("app ID = %q, want the one resolved from the bundle ID", bc.gotAppID)
	}
	if len(bc.gotUserIDs) != 4 {
		t.Errorf("store got %d user IDs, want all 4", len(bc.gotUserIDs))
	}
}

func TestBusinessCardListEmptyInput(t *testing.T) {
	bc := &fakeCardStore{}
	s := NewBusinessCard(bc, nil, fakeAppStore{})

	got, err := s.List(context.Background(), "com.yoku.apen", nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Error("want an empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("returned %d cards, want none", len(got))
	}
	if bc.called {
		t.Error("empty input must not reach the store")
	}
}

func TestBusinessCardListStoreError(t *testing.T) {
	wantErr := errors.New("boom")
	s := NewBusinessCard(&fakeCardStore{err: wantErr}, nil, fakeAppStore{})

	if _, err := s.List(context.Background(), "com.yoku.apen", []string{"user-1"}); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
