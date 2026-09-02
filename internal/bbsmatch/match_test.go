package bbsmatch

import (
	"os"
	"testing"

	"github.com/stagwoodink/pico-launcher/internal/bbsindex"
)

var idx = []bbsindex.BBSCart{
	{ID: "oswald_the_lucky_rabbit_000-1", Title: "Oswald the Lucky Rabbit", Author: "isaymatato", TID: 159025, PNGURL: "https://x/oswald.p8.png"},
	{ID: "pongalkhan-0", Title: "PONG", Author: "Alkhan", TID: 159015, PNGURL: "https://x/pong.p8.png"},
}

func TestMatchExact(t *testing.T) {
	best, score, ok := Match("Oswald the Lucky Rabbit", "isaymatato", idx)
	if !ok || best.ID != "oswald_the_lucky_rabbit_000-1" || score < 1.0 {
		t.Fatalf("want exact match, got %+v score=%v ok=%v", best, score, ok)
	}
}

func TestMatchFuzzy(t *testing.T) {
	// one typo, still above threshold
	best, score, ok := Match("Osvald the Lucky Rabbit", "isaymatato", idx)
	if !ok || best.ID != "oswald_the_lucky_rabbit_000-1" {
		t.Fatalf("want fuzzy match, got %+v score=%v ok=%v", best, score, ok)
	}
}

func TestMatchNone(t *testing.T) {
	_, score, ok := Match("Completely Unrelated Cart Title", "nobody", idx)
	if ok {
		t.Fatalf("want no match below threshold, got score=%v", score)
	}
}

func TestMatchEmptyTitle(t *testing.T) {
	_, _, ok := Match("", "", idx)
	if ok {
		t.Fatalf("want no match for empty title")
	}
}

func TestParseP8Meta(t *testing.T) {
	dir := t.TempDir()

	// no __lua__ marker at all: not a real cart, should read as empty.
	path := dir + "/cart.p8"
	content := "pico-8 cartridge // http://www.pico-8.com\nversion 42\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	title, author := ParseP8Meta(path)
	if title != "" || author != "" {
		t.Fatalf("want empty meta with no __lua__ marker, got title=%q author=%q", title, author)
	}

	// real cart shape: fixed header, __lua__ marker, then title/byline.
	path2 := dir + "/cart2.p8"
	content2 := "pico-8 cartridge // http://www.pico-8.com\nversion 8\n__lua__\n-- oswald the lucky rabbit\n-- by isaymatato\nx=1\n"
	if err := os.WriteFile(path2, []byte(content2), 0o644); err != nil {
		t.Fatal(err)
	}
	title, author = ParseP8Meta(path2)
	if title != "oswald the lucky rabbit" || author != "isaymatato" {
		t.Fatalf("got title=%q author=%q", title, author)
	}

	// __lua__ present but no title/byline comments right after it.
	path3 := dir + "/cart3.p8"
	content3 := "pico-8 cartridge // http://www.pico-8.com\nversion 8\n__lua__\nx=1\n"
	if err := os.WriteFile(path3, []byte(content3), 0o644); err != nil {
		t.Fatal(err)
	}
	title, author = ParseP8Meta(path3)
	if title != "" || author != "" {
		t.Fatalf("want empty meta with no comments after __lua__, got title=%q author=%q", title, author)
	}
}
