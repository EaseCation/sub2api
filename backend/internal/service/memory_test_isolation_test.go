package service

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"
)

// runIsolatedMemoryTest prevents background workers from earlier service tests
// from contributing to process-wide MemStats and AllocsPerRun measurements.
// The child runs the same test and retains all original memory assertions.
func runIsolatedMemoryTest(t *testing.T) bool {
	t.Helper()
	const envKey = "SUB2API_ISOLATED_MEMORY_TEST"
	if os.Getenv(envKey) == t.Name() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.CommandContext(ctx, executable, "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1", "-test.timeout=55s")
	cmd.Env = append(os.Environ(), envKey+"="+t.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated memory check failed: %v\n%s", err, output)
	}
	return true
}
