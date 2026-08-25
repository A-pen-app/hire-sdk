package models

import (
	"encoding/json"
	"testing"
)

// The values are stored as an int, so they are a wire format: a shift would
// silently reinterpret every row already written.
func TestSubscriptionStatusValues(t *testing.T) {
	for _, c := range []struct {
		name string
		got  SubscriptionStatus
		want SubscriptionStatus
	}{
		{"never", SubscriptionNever, 0},
		{"subscribed", SubscriptionSubscribed, 1},
		{"free option", SubOptionFree, 2},
		{"none", SubscriptionNone, 4},
		{"paused", SubscriptionPaused, 8},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}

// A pause stops the clock, it does not end the entitlement — so everything that
// only asks "subscribed or not" must answer the same as before.
func TestPausedStillReadsAsSubscribed(t *testing.T) {
	paused := SubscriptionSubscribed.Set(SubscriptionPaused)

	if !paused.HasOneOf(SubscriptionSubscribed) {
		t.Error("a paused subscription must still carry SubscriptionSubscribed")
	}
	if !paused.HasOneOf(SubscriptionPaused) {
		t.Error("the pause bit was lost")
	}

	b, err := json.Marshal(paused)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"SUBSCRIBED"` {
		t.Errorf("clients must not see a change: got %s", b)
	}
}
