package carts

import "testing"

func TestApplyOrder(t *testing.T) {
	cs := []Cart{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}}

	t.Run("empty order is a no-op", func(t *testing.T) {
		got := ApplyOrder(cs, nil)
		if len(got) != 4 || got[0].Name != "a" || got[3].Name != "d" {
			t.Fatalf("unexpected: %+v", got)
		}
	})

	t.Run("reorders named carts, appends unmentioned ones", func(t *testing.T) {
		got := ApplyOrder(cs, []string{"c", "a"})
		want := []string{"c", "a", "b", "d"}
		for i, w := range want {
			if got[i].Name != w {
				t.Fatalf("got %v, want %v", names(got), want)
			}
		}
	})

	t.Run("stale name in order is skipped, not crashed on", func(t *testing.T) {
		got := ApplyOrder(cs, []string{"nonexistent", "b"})
		want := []string{"b", "a", "c", "d"}
		for i, w := range want {
			if got[i].Name != w {
				t.Fatalf("got %v, want %v", names(got), want)
			}
		}
	})
}

func names(cs []Cart) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}
