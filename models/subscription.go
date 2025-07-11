package models

import (
	"encoding/json"
	"time"
)

type SubscriptionStatus int

const (
	SubscriptionSubscribed SubscriptionStatus = 1 << iota // 已訂閱
	SubOptionFree                                         // 有免費券
	SubscriptionNone                                      // 有訂閱過但沒有有效訂閱
)

const (
	SubscriptionNever SubscriptionStatus = 0 // 從來沒有訂閱過
)

func (s SubscriptionStatus) HasOneOf(flag SubscriptionStatus) bool {
	return s&flag != 0
}

func (s SubscriptionStatus) MarshalJSON() ([]byte, error) {
	var str string
	switch s {
	case SubscriptionSubscribed, SubOptionFree:
		str = "SUBSCRIBED"
	case SubscriptionNever, SubscriptionNone:
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
