package bbsreplace

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
	backup := filepath.Join(dir, backupDirName, "mycart.p8")
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}
