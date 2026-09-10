package models

import "time"

// Chat room visibility rules — archive (hidden_at) and delete (cleared_at).
//
// The rules below are specified in docs/chat_visibility.md and are shared verbatim
// with megaphone and the apen / nurse / phar repos. All three chat implementations
// must behave identically from the outside; only the storage mechanism differs. Keep
// this file, the SQL in store/chat.go, the aggregation in service/chat.go, and the
// spec document in sync.
//
// Both columns live on the per-user chat_thread row, which is what makes archive and
// delete one-sided: writing one participant's row never affects the other's. They are
// nullable timestamptz here, so nil is "never archived" / "never deleted".
//
// Most of the work is done in SQL — GetChats applies R1 and GetMessages applies R2 —
// so these functions exist for the one place that cannot: a chat's last_message_id
// points straight at a row, with no room-scoped query to hang a condition off.

// IsChatHidden reports whether a chat room is hidden from the owner of a chat_thread
// row — rule R1.
//
// A nil or zero hiddenAt means the room was never archived. Otherwise the room comes
// back as soon as the room's own last activity is later than the archive: that is what
// un-archives a room here, with no extra write on the send path, because chat.updated_at
// is written by AddMessage / AddMessages and by nothing else.
//
// The comparison is strict: activity exactly at hiddenAt leaves the room hidden (test
// case CHAT-306).
func IsChatHidden(hiddenAt *time.Time, lastActivityAt time.Time) bool {
	if hiddenAt == nil || hiddenAt.IsZero() {
		return false
	}
	return !lastActivityAt.After(*hiddenAt)
}

// IsAfterCutoff reports whether a timestamp falls after a delete cutoff — the
// timestamp half of rule R2.
//
// A nil or zero clearedAt means the user never deleted the room, so everything is
// after the cutoff. The comparison is strict: a message created exactly at the cutoff
// is not after it and stays hidden (test case CHAT-305).
func IsAfterCutoff(createdAt time.Time, clearedAt *time.Time) bool {
	if clearedAt == nil || clearedAt.IsZero() {
		return true
	}
	return createdAt.After(*clearedAt)
}

// IsMessageDeletedFor reports whether the viewer deleted this message through the
// per-message delete flow.
//
// The two bits are one per side, so which one applies depends on whether the viewer
// sent the message. This is the pre-existing mechanism and it stacks with the
// room-level cutoff (scenario 9): a message must clear both to be visible.
func IsMessageDeletedFor(status MessageStatus, senderID, viewerID string) bool {
	if viewerID == senderID {
		return status.HasOneOf(DeletedBySender)
	}
	return status.HasOneOf(DeletedByReceiver)
}

// IsMessageVisibleFor reports whether a message should appear in the viewer's message
// list — rules R2 and R4.
//
// Unsent messages count as visible: the list still renders a placeholder for both
// sides, so the caller keeps the row and wipes its content. Use
// IsLastMessageVisibleFor for the chat-list preview, which has nothing to show for one.
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
// This is IsMessageVisibleFor plus "an unsent message has nothing to preview".
func IsLastMessageVisibleFor(msg *Message, viewerID string, clearedAt *time.Time) bool {
	if !IsMessageVisibleFor(msg, viewerID, clearedAt) {
		return false
	}
	return !msg.Status.HasOneOf(Unsent)
}
