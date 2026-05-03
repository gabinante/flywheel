package db_test

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var migrationFilePattern = regexp.MustCompile(`^(\d+)_.*\.(up|down)\.sql$`)

func TestMigrationFilesAreUniqueAndPaired(t *testing.T) {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}

	type counts struct {
		up    int
		down  int
		files []string
	}

	versions := map[int]*counts{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			t.Fatalf("parse migration version from %q: %v", entry.Name(), err)
		}

		current := versions[version]
		if current == nil {
			current = &counts{}
			versions[version] = current
		}

		switch matches[2] {
		case "up":
			current.up++
		case "down":
			current.down++
		default:
			t.Fatalf("unexpected migration direction in %q", entry.Name())
		}
		current.files = append(current.files, entry.Name())
	}

	if len(versions) == 0 {
		t.Fatal("no migration files found")
	}

	// Split versions into legacy numbered (< 1000) and timestamp-based (>= 1000).
	var legacyVersions, tsVersions []int
	for version := range versions {
		if version < 1000 {
			legacyVersions = append(legacyVersions, version)
		} else {
			tsVersions = append(tsVersions, version)
		}
	}
	sort.Ints(legacyVersions)
	sort.Ints(tsVersions)

	// Legacy numbered migrations must be contiguous.
	if len(legacyVersions) > 0 {
		expected := legacyVersions[0]
		for _, version := range legacyVersions {
			if version != expected {
				t.Fatalf("legacy migration versions must be contiguous: missing %03d before %03d", expected, version)
			}
			expected++
		}
	}

	// Timestamp-based migrations just need to be unique (already guaranteed by map).
	// All migrations must have exactly one up and one down.
	ordered := make([]int, 0, len(versions))
	for version := range versions {
		ordered = append(ordered, version)
	}
	sort.Ints(ordered)
	for _, version := range ordered {
		current := versions[version]
		if current.up != 1 || current.down != 1 {
			sort.Strings(current.files)
			t.Fatalf(
				"migration %d must have exactly one up and one down file, found up=%d down=%d in %s",
				version,
				current.up,
				current.down,
				strings.Join(current.files, ", "),
			)
		}
	}
}
