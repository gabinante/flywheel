package codereview

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	prURLRe   = regexp.MustCompile(`(?i)https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/(\d+)`)
	prShortRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)#(\d+)$`)
)

// ParsePRRefs extracts (repo, number) pairs from free text containing GitHub PR
// URLs or owner/repo#N references, de-duplicated in order of appearance.
func ParsePRRefs(text string) []struct {
	Repo   string
	Number int
} {
	type ref = struct {
		Repo   string
		Number int
	}
	seen := map[string]bool{}
	var out []ref
	add := func(owner, name, num string) {
		n, err := strconv.Atoi(num)
		if err != nil || n <= 0 {
			return
		}
		r := ref{Repo: owner + "/" + strings.TrimSuffix(name, ".git"), Number: n}
		k := r.Repo + "#" + num
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	for _, m := range prURLRe.FindAllStringSubmatch(text, -1) {
		add(m[1], m[2], m[3])
	}
	for _, tok := range strings.FieldsFunc(text, func(r rune) bool { return r == ' ' || r == '\n' || r == ',' || r == ';' || r == '\t' }) {
		if m := prShortRe.FindStringSubmatch(strings.TrimSpace(tok)); m != nil {
			add(m[1], m[2], m[3])
		}
	}
	return out
}

// PRURL builds the canonical GitHub URL for a PR.
func PRURL(repo string, number int) string {
	return fmt.Sprintf("https://github.com/%s/pull/%d", repo, number)
}
