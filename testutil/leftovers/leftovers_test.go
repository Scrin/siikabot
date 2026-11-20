package leftovers

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRemove(t *testing.T) {
	prefix := "leftovers-test-" + strconv.Itoa(os.Getpid()) + "-"
	make := func(name string, age time.Duration, pid int) string {
		t.Helper()
		dir := filepath.Join(os.TempDir(), prefix+name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		if pid != 0 {
			if err := os.WriteFile(filepath.Join(dir, "test.pid"), []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		old := time.Now().Add(-age)
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	abandoned := make("abandoned", 2*time.Hour, 1<<22+12345) // a PID nothing has
	noPID := make("no-pid", 2*time.Hour, 0)
	running := make("running", 2*time.Hour, os.Getpid())
	starting := make("starting", time.Minute, 0)

	Remove(prefix, "test.pid")

	for dir, kept := range map[string]bool{abandoned: false, noPID: false, running: true, starting: true} {
		if _, err := os.Stat(dir); (err == nil) != kept {
			t.Errorf("%s kept: %v, want %v", filepath.Base(dir), err == nil, kept)
		}
	}
}
