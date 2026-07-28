package service

import (
	"context"
	"database/sql"
	"sort"
	"sync"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

// In-memory fakes for the chat archive / delete suite (see chat_test.go).
//
// Only the behaviour those scenarios depend on is modelled. Each fake embeds its
// store interface, so any method this file does not implement still satisfies the
// compiler but nil-panics the moment a test reaches it — which is the signal you
// want: it means the test is exercising a path the fake was never meant to cover.
//
// Where the fake deliberately diverges from store/chat.go, the difference is called
// out inline. The important shared property is the one the whole feature rests on:
// chat_thread is per-user, so every read and write here is keyed by
// (chatID, ownerID) and touching one participant's row never changes the other's.
//
// This file is the hire-sdk twin of megaphone's service/chat_fakes_test.go. The two
// diverge only where the schemas do — hire-sdk rooms carry app_id, post_id and
// is_pinned, and every service method resolves a bundle ID to an app first.

const testBundleID = "com.test.hire"
const testAppID = "app-1"

// ---------------------------------------------------------------------------
// chat store
// ---------------------------------------------------------------------------

// fakeThread mirrors one public.chat_thread row.
type fakeThread struct {
	chatID      string
	senderID    string // the owner of this row
	receiverID  string // the counterparty
	unreadCount int64
	lastSeenAt  *time.Time
	status      models.ChatAnnotation
	controlFlag models.ChatControlFlag
	isPinned    bool
	hiddenAt    *time.Time
	clearedAt   *time.Time
}

// fakeChat mirrors one public.chat row — the state both participants share.
type fakeChat struct {
	id            string
	appID         string
	updatedAt     time.Time
	createdAt     time.Time
	lastMessageID *string
	postID        *string
}

type fakeChatStore struct {
	store.Chat

	mu       sync.Mutex
	chats    map[string]*fakeChat
	threads  map[string]*fakeThread // chatID + "\x00" + ownerID
	messages []*models.Message      // append-only, ordered oldest first
	seq      int

	// clock drives created_at / updated_at. Tests advance it explicitly so that
	// "strictly later than" boundaries (CHAT-305 / CHAT-306) are reproducible rather
	// than dependent on how fast the test runs.
	now time.Time
}

func newFakeChatStore() *fakeChatStore {
	return &fakeChatStore{
		chats:   map[string]*fakeChat{},
		threads: map[string]*fakeThread{},
		now:     time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC),
	}
}

func threadKey(chatID, ownerID string) string { return chatID + "\x00" + ownerID }

// seedRoom creates a room plus both chat_thread rows, the way GetChatID does.
func (f *fakeChatStore) seedRoom(chatID, userA, userB string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chats[chatID] = &fakeChat{id: chatID, appID: testAppID, updatedAt: f.now, createdAt: f.now}
	f.threads[threadKey(chatID, userA)] = &fakeThread{chatID: chatID, senderID: userA, receiverID: userB}
	f.threads[threadKey(chatID, userB)] = &fakeThread{chatID: chatID, senderID: userB, receiverID: userA}
}

func (f *fakeChatStore) Get(ctx context.Context, appID, chatID, userID string) (*models.ChatRoom, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.threads[threadKey(chatID, userID)]
	if !ok {
		// Matches the real store: a non-participant gets sql.ErrNoRows, which is what
		// the service layer relies on as its membership check.
		return nil, sql.ErrNoRows
	}
	if f.chats[chatID].appID != appID {
		return nil, sql.ErrNoRows
	}
	return f.toChatRoom(t), nil
}

// toChatRoom builds the joined chat_thread + chat view the real SELECT returns.
// Caller must hold the lock.
func (f *fakeChatStore) toChatRoom(t *fakeThread) *models.ChatRoom {
	c := f.chats[t.chatID]
	return &models.ChatRoom{
		ChatID:        t.chatID,
		SenderID:      t.senderID,
		ReceiverID:    t.receiverID,
		AppID:         c.appID,
		UnreadCount:   t.unreadCount,
		LastSeenAt:    t.lastSeenAt,
		Status:        t.status,
		ControlFlag:   t.controlFlag,
		IsPinned:      t.isPinned,
		CreatedAt:     c.createdAt,
		UpdatedAt:     c.updatedAt,
		LastMessageID: c.lastMessageID,
		PostID:        c.postID,
		HiddenAt:      t.hiddenAt,
		ClearedAt:     t.clearedAt,
	}
}

func (f *fakeChatStore) GetChats(ctx context.Context, appID, userID string, next string, count int, status models.ChatAnnotation, unreadOnly bool, isOfficialRole bool) ([]*models.ChatRoom, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := []*models.ChatRoom{}
	for _, t := range f.threads {
		c := f.chats[t.chatID]
		if t.senderID != userID || c.appID != appID {
			continue
		}
		// Mirrors the real WHERE clause: rooms marked Deleted are excluded.
		if t.status == models.Deleted {
			continue
		}
		// Rule R1, mirroring
		// (CT.hidden_at IS NULL OR C.updated_at > CT.hidden_at).
		if models.IsChatHidden(t.hiddenAt, c.updatedAt) {
			continue
		}
		if status != models.None && t.status != status {
			continue
		}
		if unreadOnly && t.unreadCount == 0 {
			continue
		}
		out = append(out, f.toChatRoom(t))
	}
	// ORDER BY CT.is_pinned DESC, C.updated_at DESC
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsPinned != out[j].IsPinned {
			return out[i].IsPinned
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if count > 0 && len(out) > count {
		out = out[:count]
	}
	return out, nil
}

func (f *fakeChatStore) Pin(ctx context.Context, chatID, userID string, isPinned bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.threads[threadKey(chatID, userID)]
	if !ok {
		return sql.ErrNoRows
	}
	t.isPinned = isPinned
	return nil
}

func (f *fakeChatStore) Annotate(ctx context.Context, chatID, userID string, status models.ChatAnnotation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.threads[threadKey(chatID, userID)]
	if !ok {
		return sql.ErrNoRows
	}
	t.status = status
	return nil
}

func (f *fakeChatStore) SetHidden(ctx context.Context, chatID, userID string, hidden bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.threads[threadKey(chatID, userID)]
	if !ok {
		return sql.ErrNoRows
	}
	if !hidden {
		t.hiddenAt = nil
		return nil
	}
	at := f.now
	t.hiddenAt = &at
	// Rule R3: archiving marks the room read.
	t.unreadCount = 0
	return nil
}

func (f *fakeChatStore) Read(ctx context.Context, userID, chatID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.threads[threadKey(chatID, userID)]; ok {
		t.unreadCount = 0
	}
	return nil
}

func (f *fakeChatStore) GetMessage(ctx context.Context, messageID string) (*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages {
		if m.ID == messageID {
			cp := *m
			return &cp, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeChatStore) GetMessages(ctx context.Context, chatID string, next string, count int) ([]*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Newest first, matching ORDER BY created_at DESC.
	out := []*models.Message{}
	for i := len(f.messages) - 1; i >= 0; i-- {
		if f.messages[i].ChatID != chatID {
			continue
		}
		cp := *f.messages[i]
		out = append(out, &cp)
		if count > 0 && len(out) == count {
			break
		}
	}
	return out, nil
}

func (f *fakeChatStore) GetNewMessages(ctx context.Context, chatID string, after time.Time) ([]*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*models.Message{}
	for _, m := range f.messages {
		if m.ChatID == chatID && m.CreatedAt.After(after) {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeChatStore) AddMessage(ctx context.Context, userID, chatID, receiverID string, typ models.MessageType, body *string, mediaIDs []string, replyToMessageID, referenceID *string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.seq++
	f.now = f.now.Add(time.Minute)
	id := "msg-" + itoa(f.seq)
	f.messages = append(f.messages, &models.Message{
		ID:               id,
		ChatID:           chatID,
		SenderID:         userID,
		CreatedAt:        f.now,
		Status:           models.Normal,
		Type:             typ,
		Body:             body,
		MediaIDs:         mediaIDs,
		ReplyToMessageID: replyToMessageID,
		RefID:            referenceID,
	})

	// The shared chat row moves forward. This is the column R1 compares against, so it
	// is what makes a new message un-archive a room.
	if c, ok := f.chats[chatID]; ok {
		c.updatedAt = f.now
		c.lastMessageID = &id
	}
	// The receiver's unread count goes up and NeverGotMessages is cleared.
	if t, ok := f.threads[threadKey(chatID, receiverID)]; ok {
		t.unreadCount++
		t.controlFlag &^= models.NeverGotMessages
	}
	return id, nil
}

func (f *fakeChatStore) EditMessage(ctx context.Context, messageID string, newStatus models.MessageStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages {
		if m.ID == messageID {
			// The real store ORs the flag in rather than replacing it, so a message can
			// be deleted by both sides independently.
			m.Status |= newStatus
			return nil
		}
	}
	return sql.ErrNoRows
}

// itoa keeps the fake dependency-free; strconv would do but this reads better in IDs.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// the other five stores
// ---------------------------------------------------------------------------

// fakeAppStore resolves the one bundle ID these tests use. Every service method starts
// here, so it is the only one of the five that must actually work.
type fakeAppStore struct {
	store.App
}

func (f *fakeAppStore) GetByBundleID(ctx context.Context, bundleID string) (*models.App, error) {
	if bundleID != testBundleID {
		return nil, sql.ErrNoRows
	}
	return &models.App{ID: testAppID, BundleID: bundleID}, nil
}

// The remaining stores are only reached by the enrichment paths (resume snapshots,
// business cards, media, subscriptions) that the archive / delete scenarios stay out
// of. They return empty results rather than nil-panicking so that GetChats can walk
// its batch-enrichment steps without a real hiring database.
type fakeResumeStore struct{ store.Resume }

func (f *fakeResumeStore) ListRelations(ctx context.Context, appID string, opts ...models.ListRelationOptionFunc) ([]*models.ResumeRelation, error) {
	return nil, nil
}

type fakeBusinessCardStore struct{ store.BusinessCard }

func (f *fakeBusinessCardStore) GetSnapshotOwners(ctx context.Context, snapshotIDs []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type fakeSubscriptionStore struct{ store.Subscription }

func (f *fakeSubscriptionStore) Get(ctx context.Context, appID, userID string) (*models.UserSubscription, error) {
	return nil, sql.ErrNoRows
}

type fakeMediaStore struct{ store.Media }

// ---------------------------------------------------------------------------
// wiring
// ---------------------------------------------------------------------------

func newChatSvcForTest(c *fakeChatStore) Chat {
	return NewChat(c, &fakeResumeStore{}, &fakeAppStore{}, &fakeMediaStore{}, &fakeSubscriptionStore{}, &fakeBusinessCardStore{})
}
