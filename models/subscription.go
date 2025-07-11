package models

import (
	"time"
)

type SubscriptionStatus int

const (
	SubscriptionNone       SubscriptionStatus = iota // 無訂閱
	SubscriptionSubscribed                           // 已訂閱
)

type UserSubscription struct {
	AppID     string             `json:"-" db:"app_id"`
	UserID    string             `json:"-" db:"user_id"`
	Status    SubscriptionStatus `json:"status" db:"status"`
	ExpiresAt *time.Time         `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time          `json:"-" db:"created_at"`
	UpdatedAt time.Time          `json:"-" db:"updated_at"`
}
