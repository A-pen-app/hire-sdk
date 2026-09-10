package models

import "errors"

var (
	ErrorNotFound          = errors.New("not found")
	ErrorDuplicateEntry    = errors.New("duplicate entry")
	ErrorWrongParams       = errors.New("wrong parameters")
	ErrorUnsupported       = errors.New("unsupported")
	ErrorNotAllowed        = errors.New("action not allowed")
	ErrorInsufficientQuota = errors.New("insufficient quota")
	ErrorUserNotVerified   = errors.New("user not verified")

	// ErrorChatArchiveUnavailable is returned when archiving or deleting a chat room is
	// asked of an environment whose chat_thread table has no hidden_at / cleared_at
	// columns yet.
	//
	// Those columns are added by hand, per environment, and this library ships ahead of
	// the DDL — see store.chatStore.archivable and docs/chat_visibility.md. Until they
	// land, archive and delete are the two things the build cannot do, and saying so is
	// better than a 500-shaped "column does not exist" from Postgres. Callers should
	// answer it as 501.
	ErrorChatArchiveUnavailable = errors.New("chat archive is not available in this environment")
)
