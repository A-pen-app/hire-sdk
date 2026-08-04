package service

import (
	"context"
	"testing"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

// Embeds the interface so only the method under test is implemented; anything
// else ListReceived reaches for panics instead of silently passing.
type fakeResumeStore struct {
	store.Resume

	relations []*models.ResumeRelation

	gotAppID   string
	gotPostIDs []string
	gotNext    string
	gotCount   int
}

func (f *fakeResumeStore) ListReceived(ctx context.Context, appID string, postIDs []string, next string, count int) ([]*models.ResumeRelation, error) {
	f.gotAppID, f.gotPostIDs, f.gotNext, f.gotCount = appID, postIDs, next, count
	if count > len(f.relations) {
		count = len(f.relations)
	}
	return f.relations[:count], nil
}

type fakeAppStore struct {
	store.App
}

func (fakeAppStore) GetByBundleID(ctx context.Context, bundleID string) (*models.App, error) {
	return &models.App{ID: "app-1", BundleID: bundleID}, nil
}

func relationsAt(times ...time.Time) []*models.ResumeRelation {
	relations := make([]*models.ResumeRelation, 0, len(times))
	for _, t := range times {
		relations = append(relations, &models.ResumeRelation{CreatedAt: t})
	}
	return relations
}

func TestListReceivedPaging(t *testing.T) {
	// Descending, and the middle two share a second: the cursor has to keep
	// sub-second precision or the second page skips one of them.
	base := time.Date(2026, 8, 4, 12, 0, 5, 0, time.UTC)
	page := relationsAt(
		base.Add(900*time.Millisecond),
		base.Add(700*time.Millisecond),
		base.Add(500*time.Millisecond),
		base,
	)

	cases := []struct {
		name      string
		relations []*models.ResumeRelation
		count     int
		wantLen   int
		wantNext  string
	}{
		{
			name:      "fewer than a full page ends the list",
			relations: page[:2],
			count:     3,
			wantLen:   2,
			wantNext:  "",
		},
		{
			name:      "exactly one page ends the list",
			relations: page[:3],
			count:     3,
			wantLen:   3,
			wantNext:  "",
		},
		{
			name:      "a further row yields the last returned row as the cursor",
			relations: page,
			count:     3,
			wantLen:   3,
			wantNext:  "2026-08-04 12:00:05.5",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeResumeStore{relations: c.relations}
			s := NewResume(r, fakeAppStore{}, nil)

			got, next, err := s.ListReceived(context.Background(), "com.yoku.apen", []string{"post-1"}, "", c.count)
			if err != nil {
				t.Fatalf("ListReceived: %v", err)
			}
			if len(got) != c.wantLen {
				t.Errorf("returned %d relations, want %d", len(got), c.wantLen)
			}
			if next != c.wantNext {
				t.Errorf("next = %q, want %q", next, c.wantNext)
			}
			// one extra row is what tells the service another page exists
			if r.gotCount != c.count+1 {
				t.Errorf("asked the store for %d rows, want %d", r.gotCount, c.count+1)
			}
			if r.gotAppID != "app-1" {
				t.Errorf("app ID = %q, want the one resolved from the bundle ID", r.gotAppID)
			}
		})
	}
}

// The cursor feeds straight into "created_at < ?::timestamp", so it must carry
// no offset and keep the fraction the column stores.
func TestListReceivedCursorFormat(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{
			name: "microseconds survive",
			at:   time.Date(2026, 8, 4, 12, 0, 5, 123456000, time.UTC),
			want: "2026-08-04 12:00:05.123456",
		},
		{
			name: "a whole second carries no fraction",
			at:   time.Date(2026, 8, 4, 12, 0, 5, 0, time.UTC),
			want: "2026-08-04 12:00:05",
		},
		{
			name: "a non-UTC location is rendered as wall clock, no offset",
			at:   time.Date(2026, 8, 4, 12, 0, 5, 0, time.FixedZone("CST", 8*60*60)),
			want: "2026-08-04 12:00:05",
		},
		// Values taken from the column itself. Postgres trims trailing zeros
		// off the fraction, so the layout has to as well or the cursor stops
		// matching what is stored.
		{
			name: "full microseconds, as stored",
			at:   time.Date(2025, 7, 18, 7, 28, 13, 851711000, time.UTC),
			want: "2025-07-18 07:28:13.851711",
		},
		{
			name: "a trimmed fraction, as stored",
			at:   time.Date(2025, 10, 23, 13, 53, 55, 19300000, time.UTC),
			want: "2025-10-23 13:53:55.0193",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeResumeStore{relations: relationsAt(c.at, c.at.Add(-time.Second))}
			s := NewResume(r, fakeAppStore{}, nil)

			_, next, err := s.ListReceived(context.Background(), "com.yoku.apen", []string{"post-1"}, "", 1)
			if err != nil {
				t.Fatalf("ListReceived: %v", err)
			}
			if next != c.want {
				t.Errorf("next = %q, want %q", next, c.want)
			}
			if _, err := time.Parse(models.CursorTimeLayout, next); err != nil {
				t.Errorf("cursor does not round-trip through its own layout: %v", err)
			}
		})
	}
}

func TestListReceivedZeroCountSkipsTheStore(t *testing.T) {
	r := &fakeResumeStore{relations: relationsAt(time.Now())}
	s := NewResume(r, fakeAppStore{}, nil)

	got, next, err := s.ListReceived(context.Background(), "com.yoku.apen", []string{"post-1"}, "cursor", 0)
	if err != nil {
		t.Fatalf("ListReceived: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("returned %d relations, want none", len(got))
	}
	if next != "cursor" {
		t.Errorf("next = %q, want the cursor handed in", next)
	}
	if r.gotCount != 0 {
		t.Error("store was queried for a zero-sized page")
	}
}
