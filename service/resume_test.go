package service

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

// The service logs unconditionally on some paths, and logging.Infow panics on
// a nil zap logger.
func TestMain(m *testing.M) {
	if err := logging.Initialize(nil); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

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
			wantNext:  "2026-08-04T12:00:05.5Z",
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

// The cursor feeds straight into "created_at < ?::timestamp", so it has to keep
// the fraction the column stores. Same format as the created_at the caller saw.
func TestListRelationsCursorFormat(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{
			name: "microseconds survive",
			at:   time.Date(2026, 8, 4, 12, 0, 5, 123456000, time.UTC),
			want: "2026-08-04T12:00:05.123456Z",
		},
		{
			name: "a whole second carries no fraction",
			at:   time.Date(2026, 8, 4, 12, 0, 5, 0, time.UTC),
			want: "2026-08-04T12:00:05Z",
		},
		{
			name: "a non-UTC location keeps its offset; the cast drops it",
			at:   time.Date(2026, 8, 4, 12, 0, 5, 0, time.FixedZone("CST", 8*60*60)),
			want: "2026-08-04T12:00:05+08:00",
		},
		// Values taken from the column itself: the fraction must survive
		// unpadded and untruncated.
		{
			name: "full microseconds, as stored",
			at:   time.Date(2025, 7, 18, 7, 28, 13, 851711000, time.UTC),
			want: "2025-07-18T07:28:13.851711Z",
		},
		{
			name: "a trimmed fraction, as stored",
			at:   time.Date(2025, 10, 23, 13, 53, 55, 19300000, time.UTC),
			want: "2025-10-23T13:53:55.0193Z",
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
			if _, err := time.Parse(time.RFC3339Nano, next); err != nil {
				t.Errorf("cursor does not round-trip through its own layout: %v", err)
			}
		})
	}
}

// If the cursor never reaches the store, every page returns the newest rows.
func TestListRelationsForwardsTheCursor(t *testing.T) {
	r := &fakeResumeStore{relations: relationsAt(time.Now())}
	s := NewResume(r, fakeAppStore{}, nil)

	if _, _, err := s.ListRelations(context.Background(), "com.yoku.apen", "2026-08-04T12:00:05.5Z", 20); err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	if r.gotOpt.Before == nil {
		t.Fatal("cursor never reached the store")
	}
	want := time.Date(2026, 8, 4, 12, 0, 5, 500000000, time.UTC)
	if !r.gotOpt.Before.Equal(want) {
		t.Errorf("cursor = %v, want %v", *r.gotOpt.Before, want)
	}
}

// The cursor is opaque to the caller, so a mangled or stale one falls back to
// the newest rows instead of erroring or reaching the database.
func TestListRelationsIgnoresAnUnparseableCursor(t *testing.T) {
	for _, cursor := range []string{"2026-08-04 12:00:05.5", "not-a-time", "0"} {
		t.Run(cursor, func(t *testing.T) {
			r := &fakeResumeStore{relations: relationsAt(time.Now())}
			s := NewResume(r, fakeAppStore{}, nil)

			if _, _, err := s.ListRelations(context.Background(), "com.yoku.apen", cursor, 20); err != nil {
				t.Fatalf("ListRelations: %v", err)
			}
			if r.gotOpt.Before != nil {
				t.Errorf("cursor = %v, want the query left unbounded", *r.gotOpt.Before)
			}
		})
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
