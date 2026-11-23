// Package knownbug marks the tests for bugs the review (.scratch/review-2026-11-20.md) found and nothing has fixed
// yet. Such a test is skipped, unless SIIKABOT_KNOWN_BUGS is set, which runs them all to see which bugs are still
// there. The fix removes the call.
package knownbug

import (
	"os"
	"testing"
)

// Env runs the tests for known bugs when it's set
const Env = "SIIKABOT_KNOWN_BUGS"

// Skip skips a test for a known bug, unless Env is set
func Skip(t testing.TB, finding, what string) {
	t.Helper()
	if os.Getenv(Env) == "" {
		t.Skipf("known bug %s (.scratch/review-2026-11-20.md): %s", finding, what)
	}
}
