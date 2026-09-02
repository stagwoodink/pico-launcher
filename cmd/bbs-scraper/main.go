// Command bbs-scraper crawls the PICO-8 BBS cart listing and writes a
// carts.json index of {id, title, author, tid, png_url}. Standalone by
// design: never imported by or run from the launcher app itself, only from
// the CI cron workflow (see .github/workflows/bbs-scrape.yml).
//
// The BBS carts page embeds its listing as a JS array literal (pdat=[...])
// rather than JSON or clean HTML, so this parses that literal directly: it
// isn't valid JSON (backtick/single/double-quoted strings, trailing commas,
// unquoted numbers), so parsing is a small depth- and quote-aware splitter
// rather than encoding/json.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stagwoodink/pico-launcher/internal/bbsindex"
)

const (
	baseURL    = "https://www.lexaloffle.com"
	listingURL = baseURL + "/bbs/?cat=7&mode=carts&page=%d"
	maxPages   = 500 // safety cap, not an expected real page count
)

func main() {
	out := flag.String("out", "carts.json", "output JSON path")
	delay := flag.Duration("delay", 500*time.Millisecond, "delay between page requests")
	pages := flag.Int("pages", maxPages, "safety cap on pages to crawl")
	flag.Parse()

	var all []bbsindex.BBSCart
	seen := map[string]bool{}
	for page := 1; page <= *pages; page++ {
		rows, err := fetchPage(page)
		if err != nil {
			fmt.Fprintf(os.Stderr, "page %d: %v\n", page, err)
			break // stop, don't fail the whole run over one bad page
		}
		if len(rows) == 0 {
			break // reached the end of the listing
		}
		for _, c := range rows {
			if !seen[c.ID] {
				seen[c.ID] = true
				all = append(all, c)
			}
		}
		time.Sleep(*delay)
	}

	all = latestPerGame(all)

	if err := writeIndex(*out, all); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d carts to %s\n", len(all), *out)
}

// latestPerGame collapses same title+author entries down to the one with
// the highest thread id (BBS thread ids are assigned sequentially, so the
// highest is the newest). Many creators re-post a new BBS thread per
// update rather than editing their original in place, so without this the
// index would carry every stale prior-version thread alongside the current
// one.
func latestPerGame(carts []bbsindex.BBSCart) []bbsindex.BBSCart {
	newest := map[string]bbsindex.BBSCart{}
	for _, c := range carts {
		key := normalizeKey(c.Title) + "|" + normalizeKey(c.Author)
		if cur, ok := newest[key]; !ok || c.TID > cur.TID {
			newest[key] = c
		}
	}
	out := make([]bbsindex.BBSCart, 0, len(newest))
	for _, c := range newest {
		out = append(out, c)
	}
	return out
}

func normalizeKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// writeIndex refuses to overwrite a healthy existing index with an empty or
// suspiciously small one — a partial scrape failure should leave yesterday's
// good data in place rather than clobber it.
func writeIndex(path string, carts []bbsindex.BBSCart) error {
	if len(carts) == 0 {
		return fmt.Errorf("scraped 0 carts, refusing to write %s", path)
	}
	if existing, err := os.ReadFile(path); err == nil {
		var prev []bbsindex.BBSCart
		if json.Unmarshal(existing, &prev) == nil && len(prev) > 0 {
			if len(carts) < len(prev)/2 {
				return fmt.Errorf("scraped %d carts, less than half of existing %d, refusing to overwrite %s", len(carts), len(prev), path)
			}
		}
	}

	body, err := json.MarshalIndent(carts, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fetchPage(page int) ([]bbsindex.BBSCart, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(fmt.Sprintf(listingURL, page))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parsePDat(string(body))
}

// parsePDat extracts the `pdat=[ ... ];` literal and parses each row into a
// BBSCart. Rows this scraper can't make sense of are skipped, not fatal —
// one malformed listing row shouldn't sink the whole page.
func parsePDat(html string) ([]bbsindex.BBSCart, error) {
	marker := "pdat=["
	start := strings.Index(html, marker)
	if start == -1 {
		return nil, fmt.Errorf("pdat array not found")
	}
	start += len(marker) - 1 // point at the '['
	content, end := bracketedContent(html, start)
	if end == -1 {
		return nil, fmt.Errorf("pdat array not closed")
	}

	rows := splitTopLevel(content)
	out := make([]bbsindex.BBSCart, 0, len(rows))
	for _, row := range rows {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		if c, ok := parseRow(row); ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// bracketedContent returns the content between the '[' at s and its
// matching ']', tracking nested brackets and quoted strings so commas and
// brackets inside tag arrays/strings don't confuse the depth count.
func bracketedContent(s string, start int) (string, int) {
	depth := 0
	var quote rune
	for i := start; i < len(s); i++ {
		r := rune(s[i])
		switch {
		case quote != 0:
			if r == quote && s[i-1] != '\\' {
				quote = 0
			}
		case r == '\'' || r == '"' || r == '`':
			quote = r
		case r == '[':
			depth++
		case r == ']':
			depth--
			if depth == 0 {
				return s[start+1 : i], i
			}
		}
	}
	return "", -1
}

// splitTopLevel splits s on commas that sit outside any nested [...] and
// outside any quoted string.
func splitTopLevel(s string) []string {
	var out []string
	depth := 0
	var quote rune
	last := 0
	for i := 0; i < len(s); i++ {
		r := rune(s[i])
		switch {
		case quote != 0:
			if r == quote && s[i-1] != '\\' {
				quote = 0
			}
		case r == '\'' || r == '"' || r == '`':
			quote = r
		case r == '[':
			depth++
		case r == ']':
			depth--
		case r == ',' && depth == 0:
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	if last < len(s) {
		out = append(out, s[last:])
	}
	return out
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		q := s[0]
		if (q == '\'' || q == '"' || q == '`') && s[len(s)-1] == q {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseRow turns one pdat row into a BBSCart. Field layout (0-indexed), as
// observed in the live BBS listing markup:
//
//	0 pid  1 tid  2 title  3 thumb_url  4 width  5 height  6 date
//	7 author_uid  8 author  9 date2  ...  18 tags  ...
//
// The cart's actual downloadable id ("lid") isn't its own field — it's
// embedded in the thumb filename ("/bbs/thumbs/pico8_<lid>.png"), and that's
// also what the real cart-file URL is built from (verified against a live
// BBS post page: /bbs/cposts/<lid[:2]>/<lid>.p8.png).
func parseRow(row string) (bbsindex.BBSCart, bool) {
	row = strings.TrimSpace(row)
	if !strings.HasPrefix(row, "[") || !strings.HasSuffix(row, "]") {
		return bbsindex.BBSCart{}, false
	}
	fields := splitTopLevel(row[1 : len(row)-1])
	if len(fields) < 9 {
		return bbsindex.BBSCart{}, false
	}

	tid, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
	title := unquote(fields[2])
	thumb := unquote(fields[3])
	author := unquote(fields[8])

	lid := lidFromThumb(thumb)
	if lid == "" || title == "" {
		return bbsindex.BBSCart{}, false
	}

	prefix := lid
	if len(lid) > 2 {
		prefix = lid[:2]
	}
	pngURL := fmt.Sprintf("%s/bbs/cposts/%s/%s.p8.png", baseURL, prefix, lid)

	return bbsindex.BBSCart{
		ID:     lid,
		Title:  title,
		Author: author,
		TID:    tid,
		PNGURL: pngURL,
	}, true
}

// lidFromThumb extracts "<lid>" from a thumb URL shaped like
// "/bbs/thumbs/pico8_<lid>.png".
func lidFromThumb(thumbURL string) string {
	base := thumbURL
	if i := strings.LastIndex(base, "/"); i != -1 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".png")
	base = strings.TrimPrefix(base, "pico8_")
	return base
}
