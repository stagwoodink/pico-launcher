// Package bbsmatch matches a local cart's title/author against the BBS
// index. Pure functions only — no network, no file writes — so the harder
// side effects (download, file swap) can stay in their own package and be
// tested separately.
package bbsmatch

import (
	"bufio"
	"os"
	"sort"
	"strings"

	"github.com/stagwoodink/pico-launcher/internal/bbsindex"
)

// Threshold is the minimum similarity score (0..1) for an automatic match.
// Below it, callers should show the "?" badge instead of auto-replacing.
const Threshold = 0.82

// ParseP8Meta reads a .p8 file's first two Lua comment lines, which by
// PICO-8 convention hold the cart's title and byline, e.g.:
//
//	-- oswald the lucky rabbit
//	-- by isaymatato
//
// Returns "" for either field if the file doesn't start with comments.
func ParseP8Meta(path string) (title, author string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := make([]string, 0, 2)
	for len(lines) < 2 && scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "--") {
			break
		}
		lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "--")))
	}
	if len(lines) > 0 {
		title = lines[0]
	}
	if len(lines) > 1 {
		author = strings.TrimSpace(strings.TrimPrefix(lines[1], "by "))
	}
	return title, author
}

// normalize lowercases and strips everything but letters/digits, so
// "Oswald the Lucky Rabbit!" and "oswald_the_lucky_rabbit" compare equal.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Match finds the best BBS index entry for a local cart's title/author.
// ok is true only when the best candidate clears Threshold; below that,
// callers get no candidate at all from this call (see MatchCandidates for
// the on-demand picker case).
func Match(title, author string, index []bbsindex.BBSCart) (best bbsindex.BBSCart, score float64, ok bool) {
	nt := normalize(title)
	if nt == "" {
		return bbsindex.BBSCart{}, 0, false
	}
	na := normalize(author)

	bestScore := -1.0
	var bestCart bbsindex.BBSCart
	for _, c := range index {
		s := titleScore(nt, normalize(c.Title))
		if na != "" && normalize(c.Author) == na {
			s += 0.05 // small tiebreaker boost, not required for a match
		}
		if s > bestScore {
			bestScore = s
			bestCart = c
		}
	}
	if bestScore < 0 {
		return bbsindex.BBSCart{}, 0, false
	}
	return bestCart, bestScore, bestScore >= Threshold
}

// Candidates returns the top n index entries by title score, regardless of
// Threshold — for the on-demand "[Tab]" resolution picker, which needs
// options to choose from even when nothing cleared the auto-match bar.
func Candidates(title, author string, index []bbsindex.BBSCart, n int) []bbsindex.BBSCart {
	nt := normalize(title)
	if nt == "" || len(index) == 0 {
		return nil
	}
	na := normalize(author)

	type scored struct {
		cart  bbsindex.BBSCart
		score float64
	}
	all := make([]scored, len(index))
	for i, c := range index {
		s := titleScore(nt, normalize(c.Title))
		if na != "" && normalize(c.Author) == na {
			s += 0.05
		}
		all[i] = scored{c, s}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })

	if n > len(all) {
		n = len(all)
	}
	out := make([]bbsindex.BBSCart, n)
	for i := 0; i < n; i++ {
		out[i] = all[i].cart
	}
	return out
}

// titleScore is 1.0 for an exact normalized match, else a Levenshtein-based
// similarity in [0,1).
func titleScore(a, b string) float64 {
	if a == b {
		return 1.0
	}
	dist := levenshtein(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 0
	}
	return 1 - float64(dist)/float64(maxLen)
}

// levenshtein is the standard edit-distance DP over two rows.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min(del, min(ins, sub))
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
