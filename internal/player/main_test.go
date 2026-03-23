package player

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	Muted = true
	goleak.VerifyTestMain(m)
}
