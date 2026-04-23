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

	ordered := make([]int, 0, len(versions))
	for version := range versions {
		ordered = append(ordered, version)
	}
	sort.Ints(ordered)

	expected := ordered[0]
	for _, version := range ordered {
		if version != expected {
			t.Fatalf("migration versions must be contiguous: missing %03d before %03d", expected, version)
		}
		expected++

		current := versions[version]
		if current.up != 1 || current.down != 1 {
			sort.Strings(current.files)
			t.Fatalf(
				"migration %03d must have exactly one up and one down file, found up=%d down=%d in %s",
				version,
				current.up,
				current.down,
				strings.Join(current.files, ", "),
			)
		}
	}
}
