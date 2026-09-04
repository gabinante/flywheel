package codereview

import (
	"bufio"
	"strconv"
	"strings"
)

// DiffIndex records which RIGHT-side lines of each file appear in a unified diff.
// GitHub only accepts inline review comments on lines that are part of the diff.
type DiffIndex struct {
	files map[string]map[int]bool
}

// ParseUnifiedDiff builds a DiffIndex from `git diff` / `gh pr diff` output.
func ParseUnifiedDiff(diff string) *DiffIndex {
	idx := &DiffIndex{files: map[string]map[int]bool{}}
	sc := bufio.NewScanner(strings.NewReader(diff))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var file string
	var newLine int
	inHunk := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			file = ""
			inHunk = false
		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimPrefix(line, "+++ ")
			p = strings.TrimPrefix(p, "b/")
			if p == "/dev/null" {
				file = ""
			} else {
				file = p
				if idx.files[file] == nil {
					idx.files[file] = map[int]bool{}
				}
			}
		case strings.HasPrefix(line, "@@"):
			// @@ -a,b +c,d @@
			parts := strings.Fields(line)
			if len(parts) >= 3 && strings.HasPrefix(parts[2], "+") {
				start := strings.TrimPrefix(parts[2], "+")
				if i := strings.Index(start, ","); i >= 0 {
					start = start[:i]
				}
				newLine, _ = strconv.Atoi(start)
				inHunk = true
			}
		default:
			if !inHunk || file == "" {
				continue
			}
			if len(line) == 0 {
				// Blank context line inside a hunk.
				idx.files[file][newLine] = true
				newLine++
				continue
			}
			switch line[0] {
			case '+', ' ':
				idx.files[file][newLine] = true
				newLine++
			case '-':
				// removed line: no RIGHT-side number
			case '\\':
				// "\ No newline at end of file"
			default:
				inHunk = false
			}
		}
	}
	return idx
}

// Contains reports whether path:line is commentable on the RIGHT side.
func (d *DiffIndex) Contains(path string, line int) bool {
	if d == nil {
		return false
	}
	lines, ok := d.files[path]
	return ok && lines[line]
}

// HasFile reports whether the diff touches path.
func (d *DiffIndex) HasFile(path string) bool {
	if d == nil {
		return false
	}
	_, ok := d.files[path]
	return ok
}

// Files lists touched files.
func (d *DiffIndex) Files() []string {
	out := make([]string, 0, len(d.files))
	for f := range d.files {
		out = append(out, f)
	}
	return out
}
