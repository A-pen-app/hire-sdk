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

// The service logs unconditionally on some paths, and logging panics on a nil
// zap logger.
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

	// stand in for LIMIT/OFFSET
	from := min(f.gotOpt.Offset, len(f.relations))
	to := len(f.relations)
	if f.gotOpt.Count > 0 {
		to = min(from+f.gotOpt.Count, len(f.relations))
	}
	return f.relations[from:to], nil
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
	base := time.Date(2026, 8, 4, 12, 0, 5, 0, time.UTC)
	page := relationsAt(
		base.Add(900*time.Millisecond),
		base.Add(700*time.Millisecond),
		base.Add(500*time.Millisecond),
		base,
	)

	cases := []struct {
		name    string
		offset  int
		count   int
		wantLen int
		wantAt  time.Time
	}{
		{name: "first page", offset: 0, count: 2, wantLen: 2, wantAt: page[0].CreatedAt},
		{name: "second page", offset: 2, count: 2, wantLen: 2, wantAt: page[2].CreatedAt},
		{name: "a partial last page", offset: 3, count: 2, wantLen: 1, wantAt: page[3].CreatedAt},
		{name: "past the end", offset: 10, count: 2, wantLen: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeResumeStore{relations: page}
			s := NewResume(r, fakeAppStore{}, nil)

			got, err := s.ListRelations(context.Background(), "com.yoku.apen", c.offset, c.count, models.ByPostIDs([]string{"post-1"}))
			if err != nil {
				t.Fatalf("ListRelations: %v", err)
			}
			if len(got) != c.wantLen {
				t.Fatalf("returned %d relations, want %d", len(got), c.wantLen)
			}
			if c.wantLen > 0 && !got[0].CreatedAt.Equal(c.wantAt) {
				t.Errorf("page starts at %v, want %v", got[0].CreatedAt, c.wantAt)
			}
			if r.gotOpt.Offset != c.offset || r.gotOpt.Count != c.count {
				t.Errorf("store got offset=%d count=%d, want %d/%d", r.gotOpt.Offset, r.gotOpt.Count, c.offset, c.count)
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

// A negative offset would reach SQL as a negative OFFSET.
func TestListRelationsClampsNegativeOffset(t *testing.T) {
	r := &fakeResumeStore{relations: relationsAt(time.Now(), time.Now())}
	s := NewResume(r, fakeAppStore{}, nil)

	if _, err := s.ListRelations(context.Background(), "com.yoku.apen", -5, 20); err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	if r.gotOpt.Offset != 0 {
		t.Errorf("offset = %d, want it clamped to 0", r.gotOpt.Offset)
	}
}

// A non-positive count would reach SQL as LIMIT 0 or a negative LIMIT.
func TestListRelationsNonPositiveCountSkipsTheStore(t *testing.T) {
	for _, count := range []int{0, -1, -20} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			r := &fakeResumeStore{relations: relationsAt(time.Now(), time.Now())}
			s := NewResume(r, fakeAppStore{}, nil)

			got, err := s.ListRelations(context.Background(), "com.yoku.apen", 0, count)
			if err != nil {
				t.Fatalf("ListRelations: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("returned %d relations, want none", len(got))
			}
			if r.gotAppID != "" {
				t.Error("store was queried for a non-positive page")
			}
		})
	}
}
