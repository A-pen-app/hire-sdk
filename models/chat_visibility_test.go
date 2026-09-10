package models

import (
	"testing"
	"time"
)

// The scenario matrix in docs/chat_visibility.md is shared with megaphone and the
// apen / nurse / phar repos. What can be asserted without a database is the pure part
// of it: rules R1 and R2, and the boundary cases CHAT-305 / CHAT-306, which are the
// two places every implementation has got wrong at least once.

var (
	base    = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	before  = base.Add(-time.Hour)
	after   = base.Add(time.Hour)
	nilTime *time.Time
)

func ptr(t time.Time) *time.Time { return &t }

func TestIsChatHidden(t *testing.T) {
	cases := []struct {
		name           string
		hiddenAt       *time.Time
		lastActivityAt time.Time
		want           bool
	}{
		{"never archived", nilTime, base, false},
		{"zero value counts as never archived", ptr(time.Time{}), base, false},
		{"archived, nothing since", ptr(base), before, true},
		{"archived, a message since (CHAT-104)", ptr(base), after, false},
		{"activity exactly at hidden_at stays archived (CHAT-306)", ptr(base), base, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsChatHidden(c.hiddenAt, c.lastActivityAt); got != c.want {
				t.Errorf("IsChatHidden(%v, %v) = %v, want %v", c.hiddenAt, c.lastActivityAt, got, c.want)
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
		{"never deleted", base, nilTime, true},
		{"zero value counts as never deleted", base, ptr(time.Time{}), true},
		{"before the cutoff (CHAT-202)", before, ptr(base), false},
		{"after the cutoff (CHAT-203)", after, ptr(base), true},
		{"exactly at the cutoff stays hidden (CHAT-305)", base, ptr(base), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsAfterCutoff(c.createdAt, c.clearedAt); got != c.want {
				t.Errorf("IsAfterCutoff(%v, %v) = %v, want %v", c.createdAt, c.clearedAt, got, c.want)
			}
		})
	}
}

func TestIsMessageDeletedFor(t *testing.T) {
	const sender, receiver = "sender", "receiver"

	cases := []struct {
		name     string
		status   MessageStatus
		viewerID string
		want     bool
	}{
		{"normal, sender", Normal, sender, false},
		{"normal, receiver", Normal, receiver, false},
		{"deleted by sender, seen by sender", DeletedBySender, sender, true},
		{"deleted by sender, seen by receiver", DeletedBySender, receiver, false},
		{"deleted by receiver, seen by receiver", DeletedByReceiver, receiver, true},
		{"deleted by receiver, seen by sender", DeletedByReceiver, sender, false},
		{"both sides deleted, seen by sender", DeletedBySender | DeletedByReceiver, sender, true},
		{"unsent is not a per-side delete", Unsent, sender, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsMessageDeletedFor(c.status, sender, c.viewerID); got != c.want {
				t.Errorf("IsMessageDeletedFor(%v, %q, %q) = %v, want %v", c.status, sender, c.viewerID, got, c.want)
			}
		})
	}
}

func TestIsMessageVisibleFor(t *testing.T) {
	const sender, receiver = "sender", "receiver"
	msg := func(createdAt time.Time, status MessageStatus) *Message {
		return &Message{CreatedAt: createdAt, SenderID: sender, Status: status}
	}

	cases := []struct {
		name      string
		msg       *Message
		viewerID  string
		clearedAt *time.Time
		want      bool
	}{
		{"nil message", nil, sender, nilTime, false},
		{"plain message, never deleted", msg(base, Normal), receiver, nilTime, true},
		{"before the cutoff", msg(before, Normal), receiver, ptr(base), false},
		{"after the cutoff", msg(after, Normal), receiver, ptr(base), true},
		{"unsent stays in the list", msg(base, Unsent), receiver, nilTime, true},
		{"per-message delete hides it from that side", msg(base, DeletedBySender), sender, nilTime, false},
		{"per-message delete leaves the other side alone", msg(base, DeletedBySender), receiver, nilTime, true},
		// Scenario 9 (CHAT-303): the two layers stack, either one is enough to hide it.
		{"cutoff and per-message delete stack", msg(before, DeletedBySender), sender, ptr(base), false},
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
	const sender, receiver = "sender", "receiver"
	msg := func(createdAt time.Time, status MessageStatus) *Message {
		return &Message{CreatedAt: createdAt, SenderID: sender, Status: status}
	}

	cases := []struct {
		name      string
		msg       *Message
		viewerID  string
		clearedAt *time.Time
		want      bool
	}{
		{"plain message", msg(base, Normal), receiver, nilTime, true},
		{"unsent has nothing to preview", msg(base, Unsent), receiver, nilTime, false},
		{"before the cutoff", msg(before, Normal), receiver, ptr(base), false},
		{"after the cutoff", msg(after, Normal), receiver, ptr(base), true},
		{"deleted by this side", msg(base, DeletedByReceiver), receiver, nilTime, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsLastMessageVisibleFor(c.msg, c.viewerID, c.clearedAt); got != c.want {
				t.Errorf("IsLastMessageVisibleFor() = %v, want %v", got, c.want)
			}
		})
	}
}
