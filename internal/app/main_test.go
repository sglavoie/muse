package app

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// database/sql connection pool goroutine; cleaned up when db.Close() is
		// called, but app tests do not perform a full shutdown.
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		// DB writer background goroutine; same reason.
		goleak.IgnoreTopFunction("github.com/sglavoie/muse/internal/db.(*Writer).run"),
	)
}
