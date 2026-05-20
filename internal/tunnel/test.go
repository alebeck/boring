package tunnel

import (
	"time"

	"github.com/alebeck/boring/internal/auth"
)

// TestResult is the outcome of a tunnel connection test.
type TestResult struct {
	OK       bool
	Duration time.Duration
	Err      string
}

// TestConnection verifies that the SSH handshake and authentication for a
// tunnel succeed, including every jump hop. It opens no listener and forwards
// no traffic; the SSH client is connected, then closed and fully drained
// before returning. prompter handles any interactive auth (2FA / passphrase)
// and may be nil for non-interactive use.
func TestConnection(desc *Desc, prompter auth.Prompter) TestResult {
	start := time.Now()
	t := FromDesc(desc)
	t.prompter = prompter
	if err := t.prepare(); err != nil {
		return TestResult{Duration: time.Since(start), Err: err.Error()}
	}
	if err := t.makeClient(); err != nil {
		return TestResult{Duration: time.Since(start), Err: err.Error()}
	}
	res := TestResult{OK: true, Duration: time.Since(start)}
	t.client.Close()
	t.wg.Wait() // drain the goroutines makeClient spawned, for deterministic teardown
	return res
}
