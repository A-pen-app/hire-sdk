package service

import (
	"context"
	"database/sql"
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

// fakeSubs answers one subscription state for every lookup.
type fakeSubs struct{ subscribed bool }

func (f fakeSubs) Get(context.Context, string, string) (*models.UserSubscription, error) {
	if !f.subscribed {
		return nil, sql.ErrNoRows
	}
	return &models.UserSubscription{Status: models.SubscriptionSubscribed}, nil
}
func (f fakeSubs) List(context.Context, string, []string) ([]*models.UserSubscription, error) {
	return nil, nil
}
func (f fakeSubs) Update(context.Context, string, string, models.SubscriptionStatus, *time.Time) error {
	return nil
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
			s := NewResume(r, fakeAppStore{}, nil, fakeSubs{})

			got, err := s.ListRelations(context.Background(), "com.yoku.apen", "viewer", c.offset, c.count, models.ByPostIDs([]string{"post-1"}))
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

// Postgres rejects a negative OFFSET, and the store is reachable without going
// through the service, so Paginate clamps it.
func TestPaginateClampsNegativeOffset(t *testing.T) {
	opt := models.ListRelationOption{}
	if err := models.Paginate(-5, 20)(&opt); err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if opt.Offset != 0 {
		t.Errorf("offset = %d, want it clamped to 0", opt.Offset)
	}

	r := &fakeResumeStore{relations: relationsAt(time.Now(), time.Now())}
	s := NewResume(r, fakeAppStore{}, nil, fakeSubs{})
	if _, err := s.ListRelations(context.Background(), "com.yoku.apen", "viewer", -5, 20); err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	if r.gotOpt.Offset != 0 {
		t.Errorf("offset reaching the store = %d, want 0", r.gotOpt.Offset)
	}
}

// A non-positive count would reach SQL as LIMIT 0 or a negative LIMIT.
func TestListRelationsNonPositiveCountSkipsTheStore(t *testing.T) {
	for _, count := range []int{0, -1, -20} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			r := &fakeResumeStore{relations: relationsAt(time.Now(), time.Now())}
			s := NewResume(r, fakeAppStore{}, nil, fakeSubs{})

			got, err := s.ListRelations(context.Background(), "com.yoku.apen", "viewer", 0, count)
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

func TestVisibleStatusFollowsSubscription(t *testing.T) {
	// 沒有覆蓋的話，訂閱中的徵才方在履歷列表會看到全鎖，而同一批履歷在聊天室
	// 畫面是開的——同一個人同一時間兩個畫面互相矛盾。
	locked := []*models.ResumeRelation{
		{ID: "r1", Status: models.ResumeStatusLocked},
		{ID: "r2", Status: models.ResumeStatusUnlocked},
	}

	for _, c := range []struct {
		name       string
		subscribed bool
		want       []models.ResumeStatus
	}{
		{"訂閱中：全部視為解鎖", true, []models.ResumeStatus{models.ResumeStatusUnlocked, models.ResumeStatusUnlocked}},
		{"沒訂閱：照 DB 的值", false, []models.ResumeStatus{models.ResumeStatusLocked, models.ResumeStatusUnlocked}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeResumeStore{relations: []*models.ResumeRelation{
				{ID: locked[0].ID, Status: locked[0].Status},
				{ID: locked[1].ID, Status: locked[1].Status},
			}}
			s := NewResume(r, fakeAppStore{}, nil, fakeSubs{subscribed: c.subscribed})

			got, err := s.ListRelations(context.Background(), "com.yoku.apen", "viewer", 0, 10)
			if err != nil {
				t.Fatalf("ListRelations: %v", err)
			}
			for i := range got {
				if got[i].Status != c.want[i] {
					t.Errorf("relation %d: got %v, want %v", i, got[i].Status, c.want[i])
				}
			}
		})
	}
}
