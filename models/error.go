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
)
