package bbsreplace

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stagwoodink/pico-launcher/internal/carts"
)

func TestReplace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake png bytes"))
	}))
	defer srv.Close()

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new png bytes"))
	}))
	defer srv.Close()

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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake png bytes"))
	}))
	defer srv.Close()

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
