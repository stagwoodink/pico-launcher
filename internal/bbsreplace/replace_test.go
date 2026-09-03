package bbsreplace

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stagwoodink/pico-launcher/internal/carts"
	"github.com/stagwoodink/pico-launcher/internal/httpfetch"
)

// newTestServer starts an httptest TLS server (Get refuses plain http) and
// points httpfetch.Transport at its trusted client for the duration of t.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	prev := httpfetch.Transport
	httpfetch.Transport = srv.Client().Transport
	t.Cleanup(func() { httpfetch.Transport = prev })
	return srv
}

func TestReplace(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake png bytes"))
	})

	dir := t.TempDir()
	cartPath := filepath.Join(dir, "mycart.p8")
	if err := os.WriteFile(cartPath, []byte("-- mycart\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	newPath, err := Replace(cartPath, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if newPath != filepath.Join(dir, "mycart.p8.png") {
		t.Fatalf("unexpected new path: %s", newPath)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new cart file missing: %v", err)
	}
	if _, err := os.Stat(cartPath); !os.IsNotExist(err) {
		t.Fatalf("original .p8 should be gone from carts dir")
	}
	backup := filepath.Join(dir, carts.BackupDirName, "mycart.p8")
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestReplacePNGSource(t *testing.T) {
	// Shift+Tab can force-open the picker on a cart that already has real
	// art (.p8.png) — Replace must handle that path shape too.
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new png bytes"))
	})

	dir := t.TempDir()
	cartPath := filepath.Join(dir, "mycart.p8.png")
	if err := os.WriteFile(cartPath, []byte("old png bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	newPath, err := Replace(cartPath, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if newPath != filepath.Join(dir, "mycart.p8.png") {
		t.Fatalf("unexpected new path: %s", newPath)
	}
	got, err := os.ReadFile(newPath)
	if err != nil || string(got) != "new png bytes" {
		t.Fatalf("new cart content wrong: %q, err=%v", got, err)
	}
	backup := filepath.Join(dir, carts.BackupDirName, "mycart.p8.png")
	if b, err := os.ReadFile(backup); err != nil || string(b) != "old png bytes" {
		t.Fatalf("backup wrong: %q, err=%v", b, err)
	}
}

func TestUndo(t *testing.T) {
	dir := t.TempDir()
	cartPath := filepath.Join(dir, "mycart.p8")
	if err := os.WriteFile(cartPath, []byte("-- mycart\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake png bytes"))
	})

	newPath, err := Replace(cartPath, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := Undo(cartPath, newPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("downloaded .p8.png should be gone after undo")
	}
	got, err := os.ReadFile(cartPath)
	if err != nil || string(got) != "-- mycart\n" {
		t.Fatalf("original .p8 not restored: %q, err=%v", got, err)
	}
}

// TestUndoSamePath covers Shift+Tab replacing a cart that was already a
// .p8.png: cartPath and newPath are the same file, so Undo must remove
// before restoring or the restored original gets deleted right back out.
func TestUndoSamePath(t *testing.T) {
	dir := t.TempDir()
	cartPath := filepath.Join(dir, "mycart.p8.png")
	if err := os.WriteFile(cartPath, []byte("old png bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new png bytes"))
	})

	newPath, err := Replace(cartPath, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if newPath != cartPath {
		t.Fatalf("expected same path, got %s vs %s", newPath, cartPath)
	}
	if err := Undo(cartPath, newPath); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(cartPath)
	if err != nil || string(got) != "old png bytes" {
		t.Fatalf("original .p8.png not restored: %q, err=%v", got, err)
	}
}
