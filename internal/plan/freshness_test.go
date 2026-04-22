package plan

import (
	"testing"
	"time"
)

// --- FreshnessData tests ---

func TestFreshnessData_ContentHash_Deterministic(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	fd := &FreshnessData{
		CapturedAt: now,
		CommitSHA:  "abc123",
		StateIndexTimestamp: &now,
		ObservedEntityVersions: map[string]EntityVersion{
			"entity-1": {Version: 1, Hash: "h1"},
			"entity-2": {Version: 2, Hash: "h2"},
		},
	}

	hash1 := fd.ContentHash()
	hash2 := fd.ContentHash()
	if hash1 != hash2 {
		t.Fatalf("content hash not deterministic: %s != %s", hash1, hash2)
	}
	if hash1 == "" {
		t.Fatal("content hash should not be empty")
	}
}

func TestFreshnessData_ContentHash_DifferentOrder(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	fd1 := &FreshnessData{
		CapturedAt: now,
		CommitSHA:  "abc123",
		ObservedEntityVersions: map[string]EntityVersion{
			"a": {Version: 1}, "b": {Version: 2}, "c": {Version: 3},
		},
	}
	fd2 := &FreshnessData{
		CapturedAt: now,
		CommitSHA:  "abc123",
		ObservedEntityVersions: map[string]EntityVersion{
			"c": {Version: 3}, "a": {Version: 1}, "b": {Version: 2},
		},
	}

	if fd1.ContentHash() != fd2.ContentHash() {
		t.Fatal("content hash should be order-independent")
	}
}

func TestFreshnessData_ContentHash_NilIsEmpty(t *testing.T) {
	var fd *FreshnessData
	if fd.ContentHash() != "" {
		t.Fatal("nil FreshnessData should return empty hash")
	}
}

func TestFreshnessData_ContentHash_DifferentData(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	fd1 := &FreshnessData{CapturedAt: now, CommitSHA: "abc123"}
	fd2 := &FreshnessData{CapturedAt: now, CommitSHA: "def456"}
	if fd1.ContentHash() == fd2.ContentHash() {
		t.Fatal("different commit SHAs should produce different hashes")
	}
}

// --- StalenessConfig tests ---

func TestDefaultStalenessThresholds(t *testing.T) {
	defaults := DefaultStalenessThresholds()
	if len(defaults) != 3 {
		t.Fatalf("expected 3 defaults, got %d", len(defaults))
	}

	envs := map[string]bool{}
	for _, d := range defaults {
		envs[d.Environment] = true
	}
	for _, env := range []string{"prod", "staging", "dev"} {
		if !envs[env] {
			t.Errorf("expected default threshold for %s", env)
		}
	}
}

func TestStalenessConfig_GetThreshold_Known(t *testing.T) {
	cfg := &StalenessConfig{Thresholds: DefaultStalenessThresholds()}
	th := cfg.GetThreshold("dev")
	if th.Environment != "dev" {
		t.Fatalf("expected dev, got %s", th.Environment)
	}
	if th.MaxAge != 4*time.Hour {
		t.Fatalf("expected 4h, got %s", th.MaxAge)
	}
}

func TestStalenessConfig_GetThreshold_Unknown(t *testing.T) {
	cfg := &StalenessConfig{Thresholds: DefaultStalenessThresholds()}
	th := cfg.GetThreshold("unknown-env")
	// Should get conservative (prod-like) default.
	if !th.RequireRePlan {
		t.Fatal("unknown env should default to require re-plan (conservative)")
	}
	if th.MaxAge != 0 {
		t.Fatal("unknown env should default to MaxAge=0 (always re-plan)")
	}
}

func TestStalenessConfig_ProdAlwaysRePlans(t *testing.T) {
	cfg := &StalenessConfig{Thresholds: DefaultStalenessThresholds()}
	th := cfg.GetThreshold("prod")
	if th.MaxAge != 0 {
		t.Fatal("prod MaxAge should be 0")
	}
	if !th.RequireRePlan {
		t.Fatal("prod should require re-plan")
	}
}

// --- CheckFreshness tests ---

func TestCheckFreshness_NilPlanStamp(t *testing.T) {
	result := CheckFreshness(nil, nil, StalenessThreshold{}, "")
	if !result.Fresh {
		t.Fatal("nil plan stamp should be considered fresh (legacy fallback)")
	}
}

func TestCheckFreshness_CommitSHADrift(t *testing.T) {
	now := time.Now().UTC()
	planStamp := &FreshnessData{CapturedAt: now, CommitSHA: "aaa"}
	currentState := &FreshnessData{CapturedAt: now, CommitSHA: "bbb"}
	threshold := StalenessThreshold{Environment: "prod", MaxAge: 1 * time.Hour, RequireRePlan: true}

	result := CheckFreshness(planStamp, currentState, threshold, "")
	if result.Fresh {
		t.Fatal("should detect commit SHA drift")
	}
	if !result.RequiresRePlan {
		t.Fatal("should require re-plan when commit SHA drifted and RequireRePlan=true")
	}

	found := false
	for _, f := range result.StaleFields {
		if f == "commit_sha" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale fields should include commit_sha, got %v", result.StaleFields)
	}
}

func TestCheckFreshness_StateIndexDrift(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-1 * time.Hour)
	planStamp := &FreshnessData{CapturedAt: now, StateIndexTimestamp: &earlier}
	currentState := &FreshnessData{CapturedAt: now, StateIndexTimestamp: &now}
	threshold := StalenessThreshold{Environment: "staging", MaxAge: 2 * time.Hour, RequireRePlan: true}

	result := CheckFreshness(planStamp, currentState, threshold, "")
	if result.Fresh {
		t.Fatal("should detect state index drift")
	}
	found := false
	for _, f := range result.StaleFields {
		if f == "state_index_timestamp" {
			found = true
		}
	}
	if !found {
		t.Fatal("stale fields should include state_index_timestamp")
	}
}

func TestCheckFreshness_ClaimsRegistryDrift(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-30 * time.Minute)
	planStamp := &FreshnessData{CapturedAt: now, ClaimsRegistryTimestamp: &earlier}
	currentState := &FreshnessData{CapturedAt: now, ClaimsRegistryTimestamp: &now}
	threshold := StalenessThreshold{Environment: "prod", MaxAge: 1 * time.Hour, RequireRePlan: true}

	result := CheckFreshness(planStamp, currentState, threshold, "")
	if result.Fresh {
		t.Fatal("should detect claims registry drift")
	}
}

func TestCheckFreshness_EntityVersionDrift(t *testing.T) {
	now := time.Now().UTC()
	planStamp := &FreshnessData{
		CapturedAt: now,
		ObservedEntityVersions: map[string]EntityVersion{
			"svc-a": {Version: 3, Hash: "h3"},
		},
	}
	currentState := &FreshnessData{
		CapturedAt: now,
		ObservedEntityVersions: map[string]EntityVersion{
			"svc-a": {Version: 5, Hash: "h5"},
		},
	}
	threshold := StalenessThreshold{Environment: "prod", MaxAge: 1 * time.Hour, RequireRePlan: true}

	result := CheckFreshness(planStamp, currentState, threshold, "")
	if result.Fresh {
		t.Fatal("should detect entity version drift")
	}
}

func TestCheckFreshness_EntityHashDrift(t *testing.T) {
	now := time.Now().UTC()
	planStamp := &FreshnessData{
		CapturedAt: now,
		ObservedEntityVersions: map[string]EntityVersion{
			"svc-a": {Version: 3, Hash: "old-hash"},
		},
	}
	currentState := &FreshnessData{
		CapturedAt: now,
		ObservedEntityVersions: map[string]EntityVersion{
			"svc-a": {Version: 3, Hash: "new-hash"},
		},
	}
	threshold := StalenessThreshold{Environment: "prod", MaxAge: 1 * time.Hour, RequireRePlan: true}

	result := CheckFreshness(planStamp, currentState, threshold, "")
	if result.Fresh {
		t.Fatal("should detect entity hash drift even at same version")
	}
}

func TestCheckFreshness_TimeBased_Stale(t *testing.T) {
	old := time.Now().UTC().Add(-5 * time.Hour)
	planStamp := &FreshnessData{CapturedAt: old, CommitSHA: "same"}
	currentState := &FreshnessData{CapturedAt: time.Now().UTC(), CommitSHA: "same"}
	threshold := StalenessThreshold{Environment: "dev", MaxAge: 4 * time.Hour, RequireRePlan: false}

	result := CheckFreshness(planStamp, currentState, threshold, "")
	if result.Fresh {
		t.Fatal("plan older than MaxAge should be stale")
	}
	if result.RequiresRePlan {
		t.Fatal("dev with RequireRePlan=false should not require re-plan")
	}
}

func TestCheckFreshness_ProdAlwaysRePlan(t *testing.T) {
	now := time.Now().UTC()
	planStamp := &FreshnessData{CapturedAt: now, CommitSHA: "same"}
	currentState := &FreshnessData{CapturedAt: now, CommitSHA: "same"}
	// Prod: MaxAge=0, RequireRePlan=true
	threshold := StalenessThreshold{Environment: "prod", MaxAge: 0, RequireRePlan: true}

	result := CheckFreshness(planStamp, currentState, threshold, "")
	// MaxAge=0 + RequireRePlan: always stale when currentState is present.
	if result.Fresh {
		t.Fatal("prod should always detect staleness when MaxAge=0 and RequireRePlan=true")
	}
	if !result.RequiresRePlan {
		t.Fatal("prod should always require re-plan")
	}
}

func TestCheckFreshness_ApplyAnywayPolicy(t *testing.T) {
	now := time.Now().UTC()
	planStamp := &FreshnessData{CapturedAt: now, CommitSHA: "old"}
	currentState := &FreshnessData{CapturedAt: now, CommitSHA: "new"}
	threshold := StalenessThreshold{
		Environment:             "staging",
		MaxAge:                  1 * time.Hour,
		RequireRePlan:           true,
		AllowApplyAnywayClasses: []string{"staging-deploy", "config-update"},
	}

	// Matching change class should permit apply-anyway.
	result := CheckFreshness(planStamp, currentState, threshold, "staging-deploy")
	if result.Fresh {
		t.Fatal("commit SHA changed — should be stale")
	}
	if !result.CanApplyAnyway {
		t.Fatal("staging-deploy should be permitted as apply-anyway")
	}
	if result.PolicyOverride == "" {
		t.Fatal("policy override should explain why")
	}

	// Non-matching change class should not permit.
	result = CheckFreshness(planStamp, currentState, threshold, "dangerous-migration")
	if result.CanApplyAnyway {
		t.Fatal("dangerous-migration should not be permitted as apply-anyway")
	}
}

func TestCheckFreshness_NoCurrentState(t *testing.T) {
	now := time.Now().UTC()
	planStamp := &FreshnessData{CapturedAt: now, CommitSHA: "abc"}
	threshold := StalenessThreshold{Environment: "staging", MaxAge: 1 * time.Hour, RequireRePlan: true}

	result := CheckFreshness(planStamp, nil, threshold, "")
	// No current state to compare — should rely on time-based only.
	if !result.Fresh {
		t.Fatal("plan within MaxAge with no current state to compare should be fresh")
	}
}

// --- ComparePlanContent tests ---

func TestComparePlanContent_Identical(t *testing.T) {
	content := Content{
		Database: &DatabasePlan{
			MigrationName: "test",
			DDL:           "CREATE TABLE t (id INT);",
			Direction:     "up",
		},
	}
	diff := ComparePlanContent(content, content)
	if !diff.StructurallyIdentical {
		t.Fatalf("identical content should be structurally identical, got diffs: %+v", diff.Differences)
	}
}

func TestComparePlanContent_DifferentDDL(t *testing.T) {
	old := Content{
		Database: &DatabasePlan{
			MigrationName: "test",
			DDL:           "CREATE TABLE t (id INT);",
			Direction:     "up",
		},
	}
	new := Content{
		Database: &DatabasePlan{
			MigrationName: "test",
			DDL:           "CREATE TABLE t (id BIGINT, name TEXT);",
			Direction:     "up",
		},
	}
	diff := ComparePlanContent(old, new)
	if diff.StructurallyIdentical {
		t.Fatal("different DDL should not be structurally identical")
	}
	if len(diff.Differences) == 0 {
		t.Fatal("should report at least one difference")
	}
}

func TestComparePlanContent_DifferentBackends(t *testing.T) {
	old := Content{
		Database: &DatabasePlan{
			MigrationName: "test",
			DDL:           "CREATE TABLE t (id INT);",
			Direction:     "up",
		},
	}
	new := Content{
		Shell: &ShellPlan{
			Commands: []ShellCommand{{Command: "echo hello"}},
		},
	}
	diff := ComparePlanContent(old, new)
	if diff.StructurallyIdentical {
		t.Fatal("completely different backends should not be identical")
	}
}

func TestComparePlanContent_TerraformResourceChanges(t *testing.T) {
	old := Content{
		Terraform: &TerraformPlan{
			PlanJSON: `{"version":4}`,
			ResourceChanges: []ResourceChange{
				{Address: "aws_s3_bucket.main", Type: "aws_s3_bucket", Name: "main", ChangeAction: "create"},
			},
			Provider: "aws",
		},
	}
	new := Content{
		Terraform: &TerraformPlan{
			PlanJSON: `{"version":4}`,
			ResourceChanges: []ResourceChange{
				{Address: "aws_s3_bucket.main", Type: "aws_s3_bucket", Name: "main", ChangeAction: "create"},
				{Address: "aws_iam_role.extra", Type: "aws_iam_role", Name: "extra", ChangeAction: "create"},
			},
			Provider: "aws",
		},
	}
	diff := ComparePlanContent(old, new)
	if diff.StructurallyIdentical {
		t.Fatal("different resource changes should not be identical")
	}
}

func TestComparePlanContent_EmptyContent(t *testing.T) {
	diff := ComparePlanContent(Content{}, Content{})
	if !diff.StructurallyIdentical {
		t.Fatal("two empty contents should be identical")
	}
}

// --- EvaluateRePlan tests ---

func TestEvaluateRePlan_Fresh(t *testing.T) {
	now := time.Now().UTC()
	p := &Plan{
		FreshnessData: &FreshnessData{CapturedAt: now, CommitSHA: "abc"},
		Content: Content{
			Database: &DatabasePlan{MigrationName: "t", DDL: "CREATE TABLE t(id INT);", Direction: "up"},
		},
	}
	currentState := &FreshnessData{CapturedAt: now, CommitSHA: "abc"}
	threshold := StalenessThreshold{Environment: "dev", MaxAge: 1 * time.Hour, RequireRePlan: false}

	result := EvaluateRePlan(p, currentState, nil, threshold, "")
	if result.Decision != RePlanNotNeeded {
		t.Fatalf("expected not_needed, got %s: %s", result.Decision, result.Message)
	}
}

func TestEvaluateRePlan_StaleButDevAllows(t *testing.T) {
	old := time.Now().UTC().Add(-5 * time.Hour)
	p := &Plan{
		FreshnessData: &FreshnessData{CapturedAt: old, CommitSHA: "abc"},
		Content: Content{
			Database: &DatabasePlan{MigrationName: "t", DDL: "CREATE TABLE t(id INT);", Direction: "up"},
		},
	}
	currentState := &FreshnessData{CapturedAt: time.Now().UTC(), CommitSHA: "abc"}
	threshold := StalenessThreshold{Environment: "dev", MaxAge: 4 * time.Hour, RequireRePlan: false}

	result := EvaluateRePlan(p, currentState, nil, threshold, "")
	if result.Decision != RePlanNotNeeded {
		t.Fatalf("dev should not require re-plan even when stale, got %s: %s", result.Decision, result.Message)
	}
}

func TestEvaluateRePlan_StaleRePlanIdentical(t *testing.T) {
	now := time.Now().UTC()
	content := Content{
		Database: &DatabasePlan{MigrationName: "t", DDL: "CREATE TABLE t(id INT);", Direction: "up"},
	}
	p := &Plan{
		FreshnessData: &FreshnessData{CapturedAt: now, CommitSHA: "old"},
		Content:       content,
	}
	currentState := &FreshnessData{CapturedAt: now, CommitSHA: "new"}
	threshold := StalenessThreshold{Environment: "staging", MaxAge: 1 * time.Hour, RequireRePlan: true}

	// Re-plan produced identical content.
	result := EvaluateRePlan(p, currentState, &content, threshold, "")
	if result.Decision != RePlanIdentical {
		t.Fatalf("expected identical, got %s: %s", result.Decision, result.Message)
	}
}

func TestEvaluateRePlan_StaleRePlanDiverged(t *testing.T) {
	now := time.Now().UTC()
	oldContent := Content{
		Database: &DatabasePlan{MigrationName: "t", DDL: "CREATE TABLE t(id INT);", Direction: "up"},
	}
	newContent := Content{
		Database: &DatabasePlan{MigrationName: "t", DDL: "CREATE TABLE t(id BIGINT, name TEXT);", Direction: "up"},
	}
	p := &Plan{
		FreshnessData: &FreshnessData{CapturedAt: now, CommitSHA: "old"},
		Content:       oldContent,
	}
	currentState := &FreshnessData{CapturedAt: now, CommitSHA: "new"}
	threshold := StalenessThreshold{Environment: "prod", MaxAge: 0, RequireRePlan: true}

	result := EvaluateRePlan(p, currentState, &newContent, threshold, "")
	if result.Decision != RePlanDiverged {
		t.Fatalf("expected diverged, got %s: %s", result.Decision, result.Message)
	}
	if result.Diff == nil {
		t.Fatal("diverged result should include diff")
	}
}

func TestEvaluateRePlan_DivergedButApplyAnyway(t *testing.T) {
	now := time.Now().UTC()
	oldContent := Content{
		Deploy: &DeployPlan{ArtifactHash: "sha256:old", Target: "staging", DeployStrategy: "rolling"},
	}
	newContent := Content{
		Deploy: &DeployPlan{ArtifactHash: "sha256:new", Target: "staging", DeployStrategy: "rolling"},
	}
	p := &Plan{
		FreshnessData: &FreshnessData{CapturedAt: now, CommitSHA: "old"},
		Content:       oldContent,
	}
	currentState := &FreshnessData{CapturedAt: now, CommitSHA: "new"}
	threshold := StalenessThreshold{
		Environment:             "staging",
		MaxAge:                  30 * time.Minute,
		RequireRePlan:           true,
		AllowApplyAnywayClasses: []string{"staging-deploy"},
	}

	result := EvaluateRePlan(p, currentState, &newContent, threshold, "staging-deploy")
	if result.Decision != RePlanApplyAnyway {
		t.Fatalf("expected apply_anyway, got %s: %s", result.Decision, result.Message)
	}
}

func TestEvaluateRePlan_NilFreshnessData(t *testing.T) {
	// Plan without freshness data should be treated as fresh (legacy behavior).
	p := &Plan{
		Content: Content{
			Database: &DatabasePlan{MigrationName: "t", DDL: "CREATE TABLE t(id INT);", Direction: "up"},
		},
	}
	threshold := StalenessThreshold{Environment: "prod", MaxAge: 0, RequireRePlan: true}

	result := EvaluateRePlan(p, nil, nil, threshold, "")
	if result.Decision != RePlanNotNeeded {
		t.Fatalf("nil freshness data should be treated as fresh, got %s", result.Decision)
	}
}

func TestEvaluateRePlan_NeedRePlanNoContent(t *testing.T) {
	now := time.Now().UTC()
	p := &Plan{
		FreshnessData: &FreshnessData{CapturedAt: now, CommitSHA: "old"},
		Content: Content{
			Database: &DatabasePlan{MigrationName: "t", DDL: "CREATE TABLE t(id INT);", Direction: "up"},
		},
	}
	currentState := &FreshnessData{CapturedAt: now, CommitSHA: "new"}
	threshold := StalenessThreshold{Environment: "prod", MaxAge: 0, RequireRePlan: true}

	// No new content provided — signals re-plan is needed.
	result := EvaluateRePlan(p, currentState, nil, threshold, "")
	if result.Decision != RePlanDiverged {
		t.Fatalf("expected diverged when no new content, got %s", result.Decision)
	}
}
