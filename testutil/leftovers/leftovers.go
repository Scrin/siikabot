// Package leftovers removes the temporary directories that test processes leave behind when they end without
// cleaning up, such as with a panic. The servers they started stop with them (they're started with a parent-death
// signal), but their directories stay.
package leftovers

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Remove removes the temporary directories whose names start with prefix that were left behind. Other test runs
// may be using directories of the same kind at the same time, so only one over an hour old, whose pid file (a path
// within it) names no running process, goes.
func Remove(prefix, pidFile string) {
	dirs, _ := filepath.Glob(filepath.Join(os.TempDir(), prefix+"*"))
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() || time.Since(info.ModTime()) < time.Hour {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(dir, pidFile)); err == nil {
			pid, _ := strconv.Atoi(strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0]))
			if pid > 0 && syscall.Kill(pid, 0) == nil {
				continue // still running
			}
		}
		_ = os.RemoveAll(dir)
	}
}

// WritePID writes the test process's ID into a directory, for Remove to check
func WritePID(dir, pidFile string) error {
	return os.WriteFile(filepath.Join(dir, pidFile), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
}
