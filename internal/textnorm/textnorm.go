// Package textnorm normalizes cart titles/names for comparison — shared by
// the launcher's BBS matcher and the standalone scraper, which otherwise
// stay decoupled (see internal/bbsindex doc comment).
package textnorm

import "strings"

// Normalize lowercases s and strips everything but letters/digits, so
// "Oswald the Lucky Rabbit!" and "oswald_the_lucky_rabbit" compare equal.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
