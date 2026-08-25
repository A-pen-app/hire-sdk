package models

import (
	"encoding/json"
	"time"
)

type SubscriptionStatus int

// Bit values, stored as an int. Append new ones at the end — inserting in the
// middle shifts what is already in the database.
const (
	SubscriptionSubscribed SubscriptionStatus = 1 << iota // 已訂閱
	SubOptionFree                                         // 有免費券
	SubscriptionNone                                      // 有訂閱過但沒有有效訂閱
	// SubscriptionPaused rides alongside SubscriptionSubscribed: a paused
	// subscription is still an entitlement, its clock is just stopped. Callers
	// that only ask "may this user hire" keep reading Subscribed and see no
	// change; the ones that must act on the pause read this.
	SubscriptionPaused
)

const (
	SubscriptionNever SubscriptionStatus = 0 // 從來沒有訂閱過
)

func (s SubscriptionStatus) HasOneOf(flag SubscriptionStatus) bool {
	return s&flag != 0
}

func (s SubscriptionStatus) Set(flag SubscriptionStatus) SubscriptionStatus {
	return s | flag
}

func (s SubscriptionStatus) MarshalJSON() ([]byte, error) {
	str := ""
	if s.HasOneOf(SubscriptionSubscribed) || s.HasOneOf(SubOptionFree) {
		str = "SUBSCRIBED"
	} else {
		str = "UNSUBSCRIBED"
	}
	return json.Marshal(str)
}

type UserSubscription struct {
	AppID     string             `json:"-" db:"app_id"`
	UserID    string             `json:"-" db:"user_id"`
	Status    SubscriptionStatus `json:"status" db:"status"`
	ExpiresAt *time.Time         `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time          `json:"-" db:"created_at"`
	UpdatedAt time.Time          `json:"-" db:"updated_at"`
}
