package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherIgnoresMetadataChanges(t *testing.T) {
	dir := t.TempDir()
	content := filepath.Join(dir, "review")
	replacement := filepath.Join(dir, "replacement")
	for _, path := range []string{content, replacement} {
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan struct{}, 4)
	w, err := New(func() { events <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.Set([]string{content})
	if err := os.Chmod(content, 0o640); err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
		t.Fatal("permission change triggered a reload")
	case <-time.After(500 * time.Millisecond):
	}
	if err := os.Rename(replacement, content); err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
	case <-time.After(3 * time.Second):
		t.Fatal("no change notification for atomic replacement")
	}
}
