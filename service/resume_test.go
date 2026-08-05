package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

// Embeds the interface so anything beyond the method under test panics rather
// than silently passing.
type fakeResumeStore struct {
	store.Resume

	relations []*models.ResumeRelation

	gotAppID string
	gotOpt   models.ListRelationOption
}

func (f *fakeResumeStore) ListRelations(ctx context.Context, appID string, opts ...models.ListRelationOptionFunc) ([]*models.ResumeRelation, error) {
	f.gotAppID = appID
	f.gotOpt = models.ListRelationOption{}
	for _, apply := range opts {
		if err := apply(&f.gotOpt); err != nil {
			return nil, err
		}
	}

	count := f.gotOpt.Count
	if count == 0 || count > len(f.relations) {
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

func TestListRelationsPaging(t *testing.T) {
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

			got, next, err := s.ListRelations(context.Background(), "com.yoku.apen", "", c.count, models.ByPostIDs([]string{"post-1"}))
			if err != nil {
				t.Fatalf("ListRelations: %v", err)
			}
			if len(got) != c.wantLen {
				t.Errorf("returned %d relations, want %d", len(got), c.wantLen)
			}
			if next != c.wantNext {
				t.Errorf("next = %q, want %q", next, c.wantNext)
			}
			// one extra row is what tells the service another page exists
			if r.gotOpt.Count != c.count+1 {
				t.Errorf("asked the store for %d rows, want %d", r.gotOpt.Count, c.count+1)
			}
			if r.gotAppID != "app-1" {
				t.Errorf("app ID = %q, want the one resolved from the bundle ID", r.gotAppID)
			}
			// caller options must survive alongside the pagination one
			if len(r.gotOpt.PostIDs) != 1 || r.gotOpt.PostIDs[0] != "post-1" {
				t.Errorf("post IDs = %v, want the caller's option to reach the store", r.gotOpt.PostIDs)
			}
		})
	}
}

// The cursor feeds straight into "created_at < ?::timestamp", so it must carry
// no offset and keep the fraction the column stores.
func TestListRelationsCursorFormat(t *testing.T) {
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

			_, next, err := s.ListRelations(context.Background(), "com.yoku.apen", "", 1)
			if err != nil {
				t.Fatalf("ListRelations: %v", err)
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

// If the cursor never reaches the store, every page returns the newest rows.
func TestListRelationsForwardsTheCursor(t *testing.T) {
	r := &fakeResumeStore{relations: relationsAt(time.Now())}
	s := NewResume(r, fakeAppStore{}, nil)

	if _, _, err := s.ListRelations(context.Background(), "com.yoku.apen", "2026-08-04 12:00:05.5", 20); err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	if r.gotOpt.Before == nil {
		t.Fatal("cursor never reached the store")
	}
	if *r.gotOpt.Before != "2026-08-04 12:00:05.5" {
		t.Errorf("cursor = %q, want the one handed in", *r.gotOpt.Before)
	}
}

// A negative count used to panic on relations[count-1]. apen clamps it, but
// three other repos consume this SDK.
func TestListRelationsNonPositiveCountSkipsTheStore(t *testing.T) {
	for _, count := range []int{0, -1, -20} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			r := &fakeResumeStore{relations: relationsAt(time.Now(), time.Now())}
			s := NewResume(r, fakeAppStore{}, nil)

			got, next, err := s.ListRelations(context.Background(), "com.yoku.apen", "cursor", count)
			if err != nil {
				t.Fatalf("ListRelations: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("returned %d relations, want none", len(got))
			}
			if next != "cursor" {
				t.Errorf("next = %q, want the cursor handed in", next)
			}
			if r.gotAppID != "" {
				t.Error("store was queried for a non-positive page")
			}
		})
	}
}
