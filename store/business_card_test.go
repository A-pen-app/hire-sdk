package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestBusinessCardList(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	conn := &recordingConnector{
		columns: []string{"id", "app_id", "user_id", "content", "created_at", "updated_at"},
		rows: [][]driver.Value{
			{"card-1", "app-1", "user-1", []byte(`{"real_name":"Alice"}`), now, now},
			{"card-2", "app-1", "user-2", nil, now, now},
		},
	}
	db := sqlx.NewDb(sql.OpenDB(conn), "postgres")
	defer db.Close()

	cards, err := NewBusinessCard(db).List(context.Background(), "app-1", []string{"user-1", "user-2"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("returned %d cards, want 2", len(cards))
	}
	if cards[0].UserID != "user-1" {
		t.Errorf("user ID = %q, want user-1", cards[0].UserID)
	}
	// content is jsonb and must arrive through BusinessCardContent.Scan
	if cards[0].Content == nil || cards[0].Content.RealName == nil || *cards[0].Content.RealName != "Alice" {
		t.Errorf("content = %+v, want the scanned real name", cards[0].Content)
	}
	if cards[1].Content != nil {
		t.Errorf("content = %+v, want nil for a NULL column", cards[1].Content)
	}
	if !cards[0].CreatedAt.Equal(now) {
		t.Errorf("created at = %v, want %v", cards[0].CreatedAt, now)
	}

	for _, want := range []string{"FROM public.business_card", "app_id = $1", "user_id = ANY($2)"} {
		if !strings.Contains(conn.query, want) {
			t.Errorf("query missing %q:\n%s", want, conn.query)
		}
	}

	if len(conn.args) != 2 {
		t.Fatalf("args = %v, want the app ID and the user ID array", conn.args)
	}
	if conn.args[0] != "app-1" {
		t.Errorf("args[0] = %v, want app-1", conn.args[0])
	}
	userIDs, ok := conn.args[1].(string)
	if !ok {
		t.Fatalf("args[1] = %v (%T), want a postgres array literal", conn.args[1], conn.args[1])
	}
	if !strings.Contains(userIDs, "user-1") || !strings.Contains(userIDs, "user-2") {
		t.Errorf("args[1] = %q, want both user IDs", userIDs)
	}
}

func TestBusinessCardListEmptyInput(t *testing.T) {
	conn := &recordingConnector{}
	db := sqlx.NewDb(sql.OpenDB(conn), "postgres")
	defer db.Close()

	cards, err := NewBusinessCard(db).List(context.Background(), "app-1", nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if cards != nil {
		t.Errorf("cards = %v, want nil", cards)
	}
	if conn.query != "" {
		t.Errorf("empty input must not hit the database, got query:\n%s", conn.query)
	}
}
