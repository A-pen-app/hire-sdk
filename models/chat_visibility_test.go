package models

import (
	"testing"
	"time"
)

// These tests lock the archive / delete visibility rules (R1, R2) defined in
// docs/chat_visibility.md. The same rules are implemented in hire-sdk and in the
// apen/nurse/phar API repos, and the equivalent test files there must stay in
// agreement with this one — they are each other's consistency baseline.

var (
	tBase  = time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	tEarly = tBase.Add(-time.Hour)
	tLate  = tBase.Add(time.Hour)
)

func ptr(t time.Time) *time.Time { return &t }

func TestIsChatHidden(t *testing.T) {
	cases := []struct {
		name           string
		hiddenAt       *time.Time
		lastActivityAt time.Time
		want           bool
	}{
		{"never archived", nil, tBase, false},
		{"archived, no activity since", ptr(tBase), tEarly, true},
		{"archived, newer message restores it", ptr(tBase), tLate, false},
		// CHAT-306: restoring requires an activity strictly later than the archive
		// timestamp. Equal timestamps keep the room hidden.
		{"archived, activity exactly at the archive time", ptr(tBase), tBase, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsChatHidden(c.hiddenAt, c.lastActivityAt); got != c.want {
				t.Errorf("IsChatHidden() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsAfterCutoff(t *testing.T) {
	cases := []struct {
		name      string
		createdAt time.Time
		clearedAt *time.Time
		want      bool
	}{
		{"never deleted", tEarly, nil, true},
		{"created before the cutoff", tEarly, ptr(tBase), false},
		{"created after the cutoff", tLate, ptr(tBase), true},
		// CHAT-305: a message created exactly at the cutoff is not after it.
		{"created exactly at the cutoff", tBase, ptr(tBase), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsAfterCutoff(c.createdAt, c.clearedAt); got != c.want {
				t.Errorf("IsAfterCutoff() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsMessageDeletedFor(t *testing.T) {
	const sender, receiver = "user-a", "user-b"

	cases := []struct {
		name     string
		status   MessageStatus
		viewerID string
		want     bool
	}{
		{"normal, sender views", Normal, sender, false},
		{"normal, receiver views", Normal, receiver, false},

		{"deleted by sender, sender views", DeletedBySender, sender, true},
		{"deleted by sender, receiver still sees it", DeletedBySender, receiver, false},

		{"deleted by receiver, receiver views", DeletedByReceiver, receiver, true},
		{"deleted by receiver, sender still sees it", DeletedByReceiver, sender, false},

		{"deleted by both, sender views", DeletedBySender | DeletedByReceiver, sender, true},
		{"deleted by both, receiver views", DeletedBySender | DeletedByReceiver, receiver, true},

		// Unsent is a separate concern: it hides the content from both sides but is
		// not a per-viewer delete.
		{"unsent, sender views", Unsent, sender, false},
		{"unsent, receiver views", Unsent, receiver, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsMessageDeletedFor(c.status, sender, c.viewerID); got != c.want {
				t.Errorf("IsMessageDeletedFor() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsMessageVisibleFor(t *testing.T) {
	const sender, receiver = "user-a", "user-b"

	msg := func(status MessageStatus, createdAt time.Time) *Message {
		return &Message{SenderID: sender, Status: status, CreatedAt: createdAt}
	}

	cases := []struct {
		name      string
		msg       *Message
		viewerID  string
		clearedAt *time.Time
		want      bool
	}{
		{"nil message", nil, receiver, nil, false},
		{"normal message, no cutoff", msg(Normal, tBase), receiver, nil, true},

		{"before the cutoff", msg(Normal, tEarly), receiver, ptr(tBase), false},
		{"after the cutoff", msg(Normal, tLate), receiver, ptr(tBase), true},
		{"exactly at the cutoff", msg(Normal, tBase), receiver, ptr(tBase), false},

		{"deleted for this viewer", msg(DeletedByReceiver, tLate), receiver, ptr(tBase), false},
		{"deleted for the other side only", msg(DeletedBySender, tLate), receiver, ptr(tBase), true},

		// CHAT-303: the per-message delete and the room-level cutoff stack. Either one
		// alone is enough to hide a message.
		{"deleted for this viewer and before the cutoff", msg(DeletedByReceiver, tEarly), receiver, ptr(tBase), false},

		// Unsent rows stay in the list so the placeholder can be rendered.
		{"unsent stays visible in the message list", msg(Unsent, tLate), receiver, ptr(tBase), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsMessageVisibleFor(c.msg, c.viewerID, c.clearedAt); got != c.want {
				t.Errorf("IsMessageVisibleFor() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsLastMessageVisibleFor(t *testing.T) {
	const sender, receiver = "user-a", "user-b"

	msg := func(status MessageStatus, createdAt time.Time) *Message {
		return &Message{SenderID: sender, Status: status, CreatedAt: createdAt}
	}

	cases := []struct {
		name      string
		msg       *Message
		clearedAt *time.Time
		want      bool
	}{
		{"normal message", msg(Normal, tLate), ptr(tBase), true},
		// The only difference from IsMessageVisibleFor: an unsent message has nothing
		// to preview, so the chat list falls back to the next candidate.
		{"unsent has no preview", msg(Unsent, tLate), ptr(tBase), false},
		{"before the cutoff", msg(Normal, tEarly), ptr(tBase), false},
		{"deleted for this viewer", msg(DeletedByReceiver, tLate), ptr(tBase), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsLastMessageVisibleFor(c.msg, receiver, c.clearedAt); got != c.want {
				t.Errorf("IsLastMessageVisibleFor() = %v, want %v", got, c.want)
			}
		})
	}
}
