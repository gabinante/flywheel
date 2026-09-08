package report

import (
	"strings"
	"testing"
	"time"
)

func TestRenderProjectUpdateAndHealth(t *testing.T) {
	d := ProjectData{
		ProjectName: "Synthetic task generation for Buckeye",
		Since:       time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC),
		Until:       time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC),
		Done:        []TicketLine{{Identifier: "OPS-3626", Title: "Environment setting", URL: "https://linear.app/x/issue/OPS-3626", State: "closed"}},
		MergedPRs:   []PRLine{{Repo: "example-org/example-app", Number: 21017, Title: "Make the setting", URL: "https://github.com/example-org/example-app/pull/21017"}},
		Blocked:     []TicketLine{{Identifier: "OPS-3631", Title: "Loss mode"}},
		Activity:    []HarnessLine{{Harness: "codex", Sessions: 3, TokensIn: 1_200_000, TokensOut: 8_000}},
	}
	body := RenderProjectUpdate(d)
	for _, want := range []string{"## Update — Sep 4", "**Done (1)**", "[OPS-3626](https://linear.app/x/issue/OPS-3626)", "**Merged (1)**", "[example-app#21017]", "**Blocked or waiting on a decision (1)**", "Codex: 3 sessions, 1.2M in / 8k out"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if h := SuggestHealth(d, HealthOnTrack); h != HealthAtRisk {
		t.Errorf("blocked work should be atRisk, got %s", h)
	}
	quiet := ProjectData{Since: d.Since.AddDate(0, 0, -10), Until: d.Until}
	if h := SuggestHealth(quiet, ""); h != HealthOffTrack {
		t.Errorf("nothing moving for >7d should be offTrack, got %s", h)
	}
}

func TestRenderWeeklyRoundupAndPrepend(t *testing.T) {
	start, end := WeekWindow(time.Date(2026, 9, 4, 12, 0, 0, 0, time.Local))
	if start.Weekday() != time.Monday || end.Sub(start) != 7*24*time.Hour {
		t.Fatalf("bad window %s → %s", start, end)
	}
	w := WeekData{Start: start, End: end,
		MergedPRs: []PRLine{{Repo: "example-org/data-pipeline", Number: 369, Title: "deid: check | output", URL: "u", When: start.AddDate(0, 0, 2)}},
		Projects:  []ProjectData{{ProjectName: "Buckeye", InReview: []TicketLine{{Identifier: "OPS-3634", Title: "Launch briefs"}}}},
	}
	section := RenderWeeklyRoundup(w)
	for _, want := range []string{"## Week of " + start.Format("Jan 2, 2006"), "### Shipped (1 PRs merged)", "deid: check \\| output", "### Buckeye", "| OPS-3634 | Launch briefs | In review |"} {
		if !strings.Contains(section, want) {
			t.Errorf("missing %q in:\n%s", want, section)
		}
	}
	doc := "# Weekly Roundups (Gabriel)\n\n## Week of Aug 25, 2026\nold\n"
	out := PrependSection(doc, section)
	if !strings.HasPrefix(out, "# Weekly Roundups (Gabriel)\n\n## Week of "+start.Format("Jan 2, 2006")) || !strings.Contains(out, "## Week of Aug 25, 2026\nold") {
		t.Errorf("prepend wrong:\n%s", out)
	}
	// Re-posting the same week replaces the section instead of duplicating it.
	again := PrependSection(out, section)
	if strings.Count(again, "## Week of "+start.Format("Jan 2, 2006")) != 1 {
		t.Errorf("duplicate week section:\n%s", again)
	}
}
