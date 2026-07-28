package service

import (
	"context"
	"os"
	"testing"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
)

// TestMain initialises the logger. Several chat service methods call logging.Errorw
// on their error paths, and logging's package-level zap.Logger stays nil until
// Initialize runs, so any test that exercises a failure (membership rejection, for
// one) would nil-panic instead of returning its error.
//
// logging is already a non-test dependency of this module, so this adds nothing to
// go.mod — the suite stays standard-library-only for assertions.
func TestMain(m *testing.M) {
	if err := logging.Initialize(nil); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// Acceptance scenarios for chat room archive and delete.
//
// These mirror the scenario matrix in docs/chat_visibility.md one-for-one, and the
// CHAT-xxx IDs point at the shared test case page. The same matrix is implemented in
// megaphone and in the apen/nurse/phar API repos; those files are this one's
// consistency baseline, so a change here needs the same change there.
//
// Scenarios whose feature has not landed yet are marked t.Skip with the stage that
// will turn them on. Turning a skip into a passing test *is* the acceptance criterion
// for that stage — do not delete a skipped case to make a run green.

const (
	userA = "user-a"
	userB = "user-b"
	room  = "chat-1"
)

// setupRoom seeds one room shared by A and B, plus n messages alternating B, A, B, …
// so both participants have sent something. Returns the wired service.
func setupRoom(t *testing.T, n int) (Chat, *fakeChatStore) {
	t.Helper()
	c := newFakeChatStore()
	c.seedRoom(room, userA, userB)
	svc := newChatSvcForTest(c)

	ctx := context.Background()
	for i := 0; i < n; i++ {
		sender, receiver := userB, userA
		if i%2 == 1 {
			sender, receiver = userA, userB
		}
		if _, err := c.AddMessage(ctx, sender, room, receiver, models.MsgText, textPtr("m"+itoa(i+1)), nil, nil, nil); err != nil {
			t.Fatalf("seed message %d: %v", i+1, err)
		}
	}
	return svc, c
}

func textPtr(s string) *string { return &s }

// unreadOf reads the per-room unread count straight off the fake's chat_thread row,
// which is what rule R3 talks about.
func unreadOf(t *testing.T, c *fakeChatStore, userID string) int64 {
	t.Helper()
	th, err := c.Get(context.Background(), testAppID, room, userID)
	if err != nil {
		t.Fatalf("Get(%s): %v", userID, err)
	}
	return th.UnreadCount
}

// visibleMessages is the assertion the whole feature turns on: how many messages the
// service hands this particular viewer.
func visibleMessages(t *testing.T, svc Chat, userID string) int {
	t.Helper()
	msgs, _, err := svc.GetChatMessages(context.Background(), testBundleID, userID, room, "", 100)
	if err != nil {
		t.Fatalf("GetChatMessages(%s): %v", userID, err)
	}
	return len(msgs)
}

// listedRooms reports how many rooms the viewer's chat list contains.
func listedRooms(t *testing.T, svc Chat, userID string) int {
	t.Helper()
	chats, _, err := svc.GetChats(context.Background(), testBundleID, userID, "", 100)
	if err != nil {
		t.Fatalf("GetChats(%s): %v", userID, err)
	}
	return len(chats)
}

// ---------------------------------------------------------------------------
// Baseline — the harness itself
// ---------------------------------------------------------------------------

// Not one of the numbered scenarios: this pins down what "10 messages, both sides see
// them" means before any archive or delete enters the picture. If this breaks, every
// scenario below is measuring the wrong thing.
func TestBaselineBothSidesSeeEveryMessage(t *testing.T) {
	svc, _ := setupRoom(t, 10)

	if got := visibleMessages(t, svc, userA); got != 10 {
		t.Errorf("A sees %d messages, want 10", got)
	}
	if got := visibleMessages(t, svc, userB); got != 10 {
		t.Errorf("B sees %d messages, want 10", got)
	}
	if got := listedRooms(t, svc, userA); got != 1 {
		t.Errorf("A lists %d rooms, want 1", got)
	}
	if got := listedRooms(t, svc, userB); got != 1 {
		t.Errorf("B lists %d rooms, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Scenario 1 / 2 / 7 / 11 — archive (stage 1)
// ---------------------------------------------------------------------------

// CHAT-101 / CHAT-102
func TestArchiveHidesRoomButKeepsMessages(t *testing.T) {
	svc, _ := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	if got := listedRooms(t, svc, userA); got != 0 {
		t.Errorf("A lists %d rooms after archiving, want 0", got)
	}
	// CHAT-102: the room is only gone from the list. Opening it directly still shows
	// everything, which is what separates archive from delete.
	if got := visibleMessages(t, svc, userA); got != 10 {
		t.Errorf("A sees %d messages inside an archived room, want 10", got)
	}
}

// CHAT-103
func TestArchiveDoesNotAffectTheOtherParty(t *testing.T) {
	svc, _ := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	if got := listedRooms(t, svc, userB); got != 1 {
		t.Errorf("B lists %d rooms, want 1 — archive is one-sided", got)
	}
	if got := visibleMessages(t, svc, userB); got != 10 {
		t.Errorf("B sees %d messages, want 10", got)
	}
}

// CHAT-104
//
// This is the scenario the whole design turns on: A archives, B sends the 11th
// message, and both sides end up seeing all 11.
func TestNewMessageRestoresAnArchivedRoomWithFullHistory(t *testing.T) {
	svc, c := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if got := listedRooms(t, svc, userA); got != 0 {
		t.Fatalf("precondition: A lists %d rooms after archiving, want 0", got)
	}

	if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("m11"), nil, nil, nil); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	if got := listedRooms(t, svc, userA); got != 1 {
		t.Errorf("A lists %d rooms after a new message, want 1", got)
	}
	if got := visibleMessages(t, svc, userA); got != 11 {
		t.Errorf("A sees %d messages, want 11", got)
	}
	if got := visibleMessages(t, svc, userB); got != 11 {
		t.Errorf("B sees %d messages, want 11", got)
	}
}

// CHAT-105
func TestOwnMessageRestoresAnArchivedRoom(t *testing.T) {
	svc, c := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	// A sends into the room they archived themselves.
	if _, err := c.AddMessage(ctx, userA, room, userB, models.MsgText, textPtr("m11"), nil, nil, nil); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	if got := listedRooms(t, svc, userA); got != 1 {
		t.Errorf("A lists %d rooms after sending into an archived room, want 1", got)
	}
	if got := visibleMessages(t, svc, userA); got != 11 {
		t.Errorf("A sees %d messages, want 11", got)
	}
}

// CHAT-106
//
// Rule R3. Without this the badge would keep counting a room the user can no longer
// open, so it could never be cleared.
func TestArchiveMarksTheRoomAsRead(t *testing.T) {
	svc, c := setupRoom(t, 0)
	ctx := context.Background()

	// Both sides accumulate unread, so the one-sided assertion below actually has
	// something to detect: without B's non-zero count, "B is untouched" would pass
	// even if Archive zeroed both rows.
	for i := 0; i < 3; i++ {
		if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("m"), nil, nil, nil); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := c.AddMessage(ctx, userA, room, userB, models.MsgText, textPtr("m"), nil, nil, nil); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}
	if got := unreadOf(t, c, userA); got != 3 {
		t.Fatalf("precondition: A has %d unread, want 3", got)
	}
	if got := unreadOf(t, c, userB); got != 2 {
		t.Fatalf("precondition: B has %d unread, want 2", got)
	}

	if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	if got := unreadOf(t, c, userA); got != 0 {
		t.Errorf("A has %d unread after archiving, want 0", got)
	}
	if got := unreadOf(t, c, userB); got != 2 {
		t.Errorf("B has %d unread, want 2 — A archiving must not touch B", got)
	}
}

// CHAT-107
func TestArchivedRoomIsExcludedFromEveryListQuery(t *testing.T) {
	svc, c := setupRoom(t, 0)
	ctx := context.Background()

	// Give the room an unread message so the unread-only filter would otherwise
	// return it, then archive.
	if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("m1"), nil, nil, nil); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	for _, tc := range []struct {
		name string
		opts []models.GetOptionFunc
	}{
		{"default list", nil},
		{"unread only", []models.GetOptionFunc{models.ByStatus(models.None, true)}},
		{"marked todo", []models.GetOptionFunc{models.ByStatus(models.Todo, false)}},
		{"marked done", []models.GetOptionFunc{models.ByStatus(models.Done, false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chats, _, err := svc.GetChats(ctx, testBundleID, userA, "", 100, tc.opts...)
			if err != nil {
				t.Fatalf("GetChats: %v", err)
			}
			if len(chats) != 0 {
				t.Errorf("archived room appeared in %q (%d rooms)", tc.name, len(chats))
			}
		})
	}
}

// CHAT-108
func TestArchivingTwiceIsIdempotent(t *testing.T) {
	svc, _ := setupRoom(t, 10)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
			t.Fatalf("Archive #%d: %v", i+1, err)
		}
	}

	if got := listedRooms(t, svc, userA); got != 0 {
		t.Errorf("A lists %d rooms, want 0", got)
	}
	if got := visibleMessages(t, svc, userA); got != 10 {
		t.Errorf("A sees %d messages, want 10 — archiving never touches messages", got)
	}
}

// CHAT-109
func TestArchiveRequiresMembership(t *testing.T) {
	svc, _ := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Archive(ctx, testBundleID, "user-c", room, true); err == nil {
		t.Fatal("Archive by a non-participant succeeded, want an error")
	}

	// Neither participant's view may have changed.
	if got := listedRooms(t, svc, userA); got != 1 {
		t.Errorf("A lists %d rooms, want 1", got)
	}
	if got := listedRooms(t, svc, userB); got != 1 {
		t.Errorf("B lists %d rooms, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Scenario 3 / 4 / 5 / 6 / 8 — delete (stage 2)
// ---------------------------------------------------------------------------

// CHAT-201 / CHAT-202
func TestDeleteClearsMySideAndLeavesTheirs(t *testing.T) {
	svc, _ := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	if got := listedRooms(t, svc, userA); got != 0 {
		t.Errorf("A lists %d rooms after deleting, want 0", got)
	}
	if got := visibleMessages(t, svc, userA); got != 0 {
		t.Errorf("A sees %d messages after deleting, want 0", got)
	}
	// The other side is untouched — no message row was modified, only A's cutoff.
	if got := listedRooms(t, svc, userB); got != 1 {
		t.Errorf("B lists %d rooms, want 1", got)
	}
	if got := visibleMessages(t, svc, userB); got != 10 {
		t.Errorf("B sees %d messages, want 10", got)
	}
}

// CHAT-203
func TestNewMessageAfterDeleteShowsOnlyPostCutoffMessages(t *testing.T) {
	svc, c := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("m11"), nil, nil, nil); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	if got := listedRooms(t, svc, userA); got != 1 {
		t.Errorf("A lists %d rooms after a new message, want 1", got)
	}
	if got := visibleMessages(t, svc, userA); got != 1 {
		t.Errorf("A sees %d messages, want 1 — only the one sent after the cutoff", got)
	}
	if got := visibleMessages(t, svc, userB); got != 11 {
		t.Errorf("B sees %d messages, want 11", got)
	}
}

// CHAT-204
func TestMyOwnMessageAfterDeleteIsVisibleToMe(t *testing.T) {
	svc, c := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := c.AddMessage(ctx, userA, room, userB, models.MsgText, textPtr("m11"), nil, nil, nil); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	if got := visibleMessages(t, svc, userA); got != 1 {
		t.Errorf("A sees %d messages, want 1 — their own, sent after the cutoff", got)
	}
	if got := visibleMessages(t, svc, userB); got != 11 {
		t.Errorf("B sees %d messages, want 11", got)
	}
}

// CHAT-205
func TestDeleteCutoffIsNeverReset(t *testing.T) {
	svc, c := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("after"), nil, nil, nil); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}

	if got := visibleMessages(t, svc, userA); got != 3 {
		t.Errorf("A sees %d messages, want exactly 3 — the original 10 must never come back", got)
	}
	if got := visibleMessages(t, svc, userB); got != 13 {
		t.Errorf("B sees %d messages, want 13", got)
	}
}

// CHAT-206
func TestDeleteMarksTheRoomAsRead(t *testing.T) {
	svc, c := setupRoom(t, 0)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("m"), nil, nil, nil); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}
	if got := unreadOf(t, c, userA); got != 3 {
		t.Fatalf("precondition: A has %d unread, want 3", got)
	}

	if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	if got := unreadOf(t, c, userA); got != 0 {
		t.Errorf("A has %d unread after deleting, want 0", got)
	}
	if got := unreadOf(t, c, userB); got != 0 {
		t.Errorf("B has %d unread, want 0 — A deleting must not touch B", got)
	}
}

// CHAT-207
func TestLastMessagePreviewRespectsTheCutoff(t *testing.T) {
	svc, c := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("m11"), nil, nil, nil); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	chats, _, err := svc.GetChats(ctx, testBundleID, userA, "", 100)
	if err != nil {
		t.Fatalf("GetChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("A lists %d rooms, want 1", len(chats))
	}
	if chats[0].LastMessage == nil {
		t.Fatal("A has no preview, want the message sent after the cutoff")
	}
	if got := *chats[0].LastMessage.Body; got != "m11" {
		t.Errorf("preview is %q, want %q — never a message from before the cutoff", got, "m11")
	}
}

// CHAT-208
func TestDeletingTwiceMovesTheCutoffForward(t *testing.T) {
	svc, c := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
		t.Fatalf("Clear #1: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("after"), nil, nil, nil); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}
	if got := visibleMessages(t, svc, userA); got != 2 {
		t.Fatalf("precondition: A sees %d messages, want 2", got)
	}

	if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
		t.Fatalf("Clear #2: %v", err)
	}

	if got := visibleMessages(t, svc, userA); got != 0 {
		t.Errorf("A sees %d messages after a second delete, want 0", got)
	}
	if got := visibleMessages(t, svc, userB); got != 12 {
		t.Errorf("B sees %d messages, want 12", got)
	}
}

// CHAT-209
func TestDeleteRequiresMembership(t *testing.T) {
	svc, _ := setupRoom(t, 10)
	ctx := context.Background()

	if err := svc.Clear(ctx, testBundleID, "user-c", room); err == nil {
		t.Fatal("Clear by a non-participant succeeded, want an error")
	}

	if got := visibleMessages(t, svc, userA); got != 10 {
		t.Errorf("A sees %d messages, want 10", got)
	}
	if got := visibleMessages(t, svc, userB); got != 10 {
		t.Errorf("B sees %d messages, want 10", got)
	}
}

// CHAT-301 / CHAT-302
func TestArchiveAndDeleteInteract(t *testing.T) {
	ctx := context.Background()

	t.Run("delete supersedes archive", func(t *testing.T) {
		svc, _ := setupRoom(t, 10)
		if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
			t.Fatalf("Archive: %v", err)
		}
		if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
			t.Fatalf("Clear: %v", err)
		}
		if got := listedRooms(t, svc, userA); got != 0 {
			t.Errorf("A lists %d rooms, want 0", got)
		}
		if got := visibleMessages(t, svc, userA); got != 0 {
			t.Errorf("A sees %d messages, want 0", got)
		}
	})

	t.Run("archiving after a delete keeps the cutoff", func(t *testing.T) {
		svc, c := setupRoom(t, 10)
		if err := svc.Clear(ctx, testBundleID, userA, room); err != nil {
			t.Fatalf("Clear: %v", err)
		}
		if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("m11"), nil, nil, nil); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
		if err := svc.Archive(ctx, testBundleID, userA, room, true); err != nil {
			t.Fatalf("Archive: %v", err)
		}
		if got := listedRooms(t, svc, userA); got != 0 {
			t.Fatalf("A lists %d rooms after archiving, want 0", got)
		}

		if _, err := c.AddMessage(ctx, userB, room, userA, models.MsgText, textPtr("m12"), nil, nil, nil); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
		if got := listedRooms(t, svc, userA); got != 1 {
			t.Errorf("A lists %d rooms, want 1", got)
		}
		// Two messages after the cutoff, never the original ten.
		if got := visibleMessages(t, svc, userA); got != 2 {
			t.Errorf("A sees %d messages, want 2", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 9 / 9b — the per-message delete
// ---------------------------------------------------------------------------

// CHAT-311: a message the viewer deleted stays in the list and renders as unsent — the
// row is never dropped, because a short page makes the mobile clients conclude there is
// no older history (apen-api#180).
//
// hire-sdk has no per-message delete writer at all: the read side is implemented in
// aggregateMessages, but nothing ever sets DeletedBySender or DeletedByReceiver (gap G4
// in docs/chat_visibility.md). megaphone is where this scenario runs today.
func TestPerMessageDeleteRendersAsUnsentAndKeepsTheRow(t *testing.T) {
	t.Skip("hire-sdk has no per-message delete writer (G4)")
}

// CHAT-311, the other direction.
func TestPerMessageDeleteBySenderRendersAsUnsentForTheSenderOnly(t *testing.T) {
	t.Skip("hire-sdk has no per-message delete writer (G4)")
}

// CHAT-304: unsend is a different thing from a per-message delete in intent, even
// though the two now render the same way — unsend affects both sides, a per-message
// delete only the viewer.
func TestUnsendKeepsTheRowForBothSides(t *testing.T) {
	svc, c := setupRoom(t, 3)
	ctx := context.Background()

	target := c.messages[1] // sent by A, so A is allowed to unsend it
	if err := svc.UnsendMessage(ctx, testBundleID, userA, target.ID); err != nil {
		t.Fatalf("UnsendMessage: %v", err)
	}

	if got := visibleMessages(t, svc, userA); got != 3 {
		t.Errorf("A sees %d messages after an unsend, want 3 — the placeholder row stays", got)
	}
	if got := visibleMessages(t, svc, userB); got != 3 {
		t.Errorf("B sees %d messages after an unsend, want 3 — the placeholder row stays", got)
	}
}

// CHAT-303: the per-message delete and the room-level cutoff stack. The cutoff removes
// rows (in SQL, so paging stays correct), the per-message delete only blanks one.
func TestPerMessageDeleteStacksWithTheRoomCutoff(t *testing.T) {
	t.Skip("hire-sdk has no per-message delete writer (G4)")
}

// ---------------------------------------------------------------------------
// Scenario 12 — megaphone only
// ---------------------------------------------------------------------------

// CHAT-401 and CHAT-402 are megaphone-only: the ChatAnnotation marks are reachable
// there through PATCH /chats/:chat_id/mark. In hire-sdk, Annotate is not on the
// service interface and has no route (gap G2 in docs/chat_visibility.md), so there is
// nothing to assert here.
