package codereview

import (
	"testing"
)

func TestParsePRRefs(t *testing.T) {
	refs := ParsePRRefs("review https://github.com/joinhandshake/joinera/pull/20888 and joinhandshake/rle-data-processing#372, also https://github.com/joinhandshake/joinera/pull/20888/files")
	if len(refs) != 2 {
		t.Fatalf("got %d refs: %+v", len(refs), refs)
	}
	if refs[0].Repo != "joinhandshake/joinera" || refs[0].Number != 20888 || refs[1].Repo != "joinhandshake/rle-data-processing" || refs[1].Number != 372 {
		t.Fatalf("unexpected refs: %+v", refs)
	}
}

const sampleDiff = `diff --git a/src/a.go b/src/a.go
index 1..2 100644
--- a/src/a.go
+++ b/src/a.go
@@ -10,4 +10,5 @@ func x() {
 	keep()
-	old()
+	new()
+	added()
 	tail()
diff --git a/src/new.txt b/src/new.txt
new file mode 100644
--- /dev/null
+++ b/src/new.txt
@@ -0,0 +1,2 @@
+hello
+world
`

func TestParseUnifiedDiff(t *testing.T) {
	idx := ParseUnifiedDiff(sampleDiff)
	// hunk starts at new line 10: keep(10) new(11) added(12) tail(13)
	for _, l := range []int{10, 11, 12, 13} {
		if !idx.Contains("src/a.go", l) {
			t.Errorf("expected src/a.go:%d in diff", l)
		}
	}
	if idx.Contains("src/a.go", 14) || idx.Contains("src/a.go", 9) {
		t.Errorf("lines outside the hunk must not be commentable")
	}
	if !idx.Contains("src/new.txt", 1) || !idx.Contains("src/new.txt", 2) || idx.Contains("src/new.txt", 3) {
		t.Errorf("new file lines wrong")
	}
	if !idx.HasFile("src/new.txt") || idx.HasFile("src/other.go") {
		t.Errorf("HasFile wrong")
	}
}

func TestParseReviewOutputAndVerdict(t *testing.T) {
	summary, findings, err := ParseReviewOutput([]byte("```json\n{\"summary\":\"Looks solid.\",\"findings\":[{\"severity\":\"p1\",\"title\":\"Nil deref\",\"path\":\"./src/a.go\",\"line\":11,\"body\":\"x may be nil here\"},{\"severity\":\"P3\",\"title\":\"Name\",\"path\":\"src/a.go\",\"line\":99,\"body\":\"rename\"}]}\n```"))
	if err != nil {
		t.Fatal(err)
	}
	if summary != "Looks solid." || len(findings) != 2 || findings[0].Severity != "P1" || findings[0].Path != "src/a.go" {
		t.Fatalf("parse: %q %+v", summary, findings)
	}
	if Verdict(findings) != VerdictRequestChanges {
		t.Errorf("P1 must request changes")
	}
	if Verdict(findings[1:]) != VerdictApprove {
		t.Errorf("P3-only must approve")
	}
	body, inline, inBody := ComposeReview(summary, findings, ParseUnifiedDiff(sampleDiff))
	if len(inline) != 1 || inline[0] != 0 || len(inBody) != 1 || inBody[0] != 1 {
		t.Fatalf("split wrong: inline=%v inBody=%v", inline, inBody)
	}
	for _, want := range []string{"Looks solid.", "[P3] Name", "src/a.go:99"} {
		if !containsStr(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if containsStr(body, "Flywheel") || containsStr(body, "1 P1, 1 P3") {
		t.Errorf("review body contains a generated footer: %s", body)
	}
	clean, _, _ := ComposeReview("Looks good to me.", nil, ParseUnifiedDiff(sampleDiff))
	if clean != "Looks good to me." {
		t.Errorf("clean review: %q", clean)
	}
	followup := reReviewBody(nil, 1, clean)
	if followup != "Rechecked the latest changes. No new issues. 1 earlier finding still open." {
		t.Errorf("follow-up review: %q", followup)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
