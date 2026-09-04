package sessions

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	prURLRe     = regexp.MustCompile(`https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/(\d+)`)
	linearURLRe = regexp.MustCompile(`https?://linear\.app/[A-Za-z0-9_-]+/issue/([A-Z][A-Z0-9]{1,9}-\d{1,6})`)
	linearKeyRe = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9})-(\d{1,6})\b`)
	branchKeyRe = regexp.MustCompile(`(?i)(?:^|/)([a-z][a-z0-9]{1,9})-(\d{1,6})(?:-|$)`)
)

// linearKeyDenylist holds identifier-looking prefixes that are never Linear teams.
var linearKeyDenylist = map[string]bool{
	"UTF": true, "ISO": true, "SHA": true, "MD": true, "GPT": true, "RFC": true, "CVE": true,
	"HTTP": true, "HTTPS": true, "PR": true, "PRS": true, "ID": true, "UUID": true, "TS": true,
	"TODO": true, "JSON": true, "YAML": true, "AWS": true, "GCP": true, "API": true, "URL": true,
	"P": true, "T": true, "V": true, "X": true, "Y": true, "Z": true, "N": true, "GH": true,
	"E": true, "TCP": true, "UDP": true, "IP": true, "OAUTH": true, "JWT": true, "PG": true,
	"ADR": true, "GMT": true, "UTC": true, "PST": true, "PDT": true, "EST": true, "EDT": true, "CEST": true,
	"ISO8601": true, "RFC3339": true, "ECMA": true, "IEEE": true, "EPSG": true, "COVID": true, "GPT4": true,
}

// ExtractRefs finds pull request and Linear issue references in free text.
// PRs are normalized to "owner/repo#N"; issues to "KEY-N".
func ExtractRefs(text string) []Link {
	seen := map[string]bool{}
	var out []Link
	add := func(kind, ref string) {
		k := kind + ":" + ref
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, Link{Kind: kind, Ref: ref, Source: LinkSourceInferred})
	}
	for _, m := range prURLRe.FindAllStringSubmatch(text, -1) {
		add(LinkPR, fmt.Sprintf("%s/%s#%s", m[1], strings.TrimSuffix(m[2], ".git"), m[3]))
	}
	for _, m := range linearURLRe.FindAllStringSubmatch(text, -1) {
		add(LinkLinearIssue, m[1])
	}
	for _, m := range linearKeyRe.FindAllStringSubmatch(text, -1) {
		if linearKeyDenylist[m[1]] {
			continue
		}
		add(LinkLinearIssue, m[1]+"-"+m[2])
	}
	return out
}

// RefsFromBranch derives a Linear issue reference from a branch or worktree
// slug such as "rlep-3488-review-fixes" or "user/rletd-465-fix".
func RefsFromBranch(branch string) []Link {
	m := branchKeyRe.FindStringSubmatch(branch)
	if m == nil {
		return nil
	}
	key := strings.ToUpper(m[1])
	if linearKeyDenylist[key] {
		return nil
	}
	return []Link{{Kind: LinkLinearIssue, Ref: key + "-" + m[2], Source: LinkSourceInferred}}
}
