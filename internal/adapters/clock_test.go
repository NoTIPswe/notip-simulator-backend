package adapters_test

import (
	"testing"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters"
)

func TestSystemClock_Now_IsWithinCallWindow(t *testing.T) {
	c := adapters.SystemClock{}
	before := time.Now()
	now := c.Now()
	after := time.Now()

	if now.Before(before) || now.After(after) {
		t.Fatalf("clock time %v is outside call window [%v, %v]", now, before, after)
	}
}
