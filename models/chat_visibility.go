package models

import "time"

// Chat room visibility rules — archive (hidden_at) and delete (cleared_at).
//
// The rules below are specified in docs/chat_visibility.md and are shared verbatim
// with megaphone and the apen/nurse/phar API repos. All three implementations must
// behave identically; only the storage mechanism differs. Keep this file, the SQL in
// store/chat.go, and the spec document in sync.
//
// Both hidden_at and cleared_at live on the per-user chat_thread row, which is what
// makes archive and delete one-sided: writing one participant's row never affects the
// other's.

// IsChatHidden reports whether a chat room is hidden from the owner of a chat_thread
// row — rule R1.
//
// A room stays hidden until an activity strictly later than hiddenAt occurs. Because
// hire-sdk only ever bumps chat.updated_at when a message is added (store/chat.go,
// AddMessage and AddMessages — the other UPDATE public.chat statements set a single
// unrelated column), passing that column as lastActivityAt makes a new message
// un-archive the room automatically, with no extra write on the send path.
//
// The comparison is deliberately strict: an activity timestamp exactly equal to
// hiddenAt leaves the room hidden (test case CHAT-306).
func IsChatHidden(hiddenAt *time.Time, lastActivityAt time.Time) bool {
	if hiddenAt == nil {
		return false
	}
	return !lastActivityAt.After(*hiddenAt)
}

// IsAfterCutoff reports whether a timestamp falls after a delete cutoff — the
// timestamp half of rule R2.
//
// A nil clearedAt means the user never deleted the room, so everything is after the
// cutoff. The comparison is strict: a message created exactly at the cutoff is not
// after it and stays hidden (test case CHAT-305).
func IsAfterCutoff(createdAt time.Time, clearedAt *time.Time) bool {
	if clearedAt == nil {
		return true
	}
	return createdAt.After(*clearedAt)
}

// IsMessageDeletedFor reports whether a message was deleted by the given viewer
// through the per-message delete flow.
//
// message.status is a bitmask holding one flag per side, so the same row can be
// deleted by the sender, by the receiver, or by both, and each side only ever hides
// its own copy.
func IsMessageDeletedFor(status MessageStatus, senderID, viewerID string) bool {
	if status.HasOneOf(DeletedBySender) && viewerID == senderID {
		return true
	}
	return status.HasOneOf(DeletedByReceiver) && viewerID != senderID
}

// IsMessageVisibleFor reports whether a message should appear in the viewer's message
// list — rule R2.
//
// Unsent messages count as visible: the list still renders a "message unsent"
// placeholder, so the caller wipes their content rather than dropping the row. Use
// IsLastMessageVisibleFor for the chat-list preview, which drops them instead.
func IsMessageVisibleFor(msg *Message, viewerID string, clearedAt *time.Time) bool {
	if msg == nil {
		return false
	}
	if !IsAfterCutoff(msg.CreatedAt, clearedAt) {
		return false
	}
	return !IsMessageDeletedFor(msg.Status, msg.SenderID, viewerID)
}

// IsLastMessageVisibleFor reports whether a message may be used as the viewer's
// chat-list preview.
//
// This is IsMessageVisibleFor plus "unsent messages have nothing to preview", matching
// the behaviour the service layer already had before these rules were extracted.
func IsLastMessageVisibleFor(msg *Message, viewerID string, clearedAt *time.Time) bool {
	if !IsMessageVisibleFor(msg, viewerID, clearedAt) {
		return false
	}
	return !msg.Status.HasOneOf(Unsent)
}
