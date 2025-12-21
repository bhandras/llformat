package formatter

import "testing"

func TestNewStage(t *testing.T) {
	// Create a mock formatter
	formatter := NoopFormatter{}

	stage := NewStage("test-stage", formatter)

	if stage.Name != "test-stage" {
		t.Errorf("Name = %q, want %q", stage.Name, "test-stage")
	}
	if stage.Formatter == nil {
		t.Error("Formatter is nil")
	}
	if stage.Requires != nil {
		t.Error("Requires should be nil by default")
	}
}

func TestStageWithRequires(t *testing.T) {
	formatter := NoopFormatter{}
	stage := NewStage("test-stage", formatter)

	// Add dependencies
	newStage := stage.WithRequires("dep1", "dep2")

	// Original should be unchanged
	if len(stage.Requires) != 0 {
		t.Error("Original stage modified")
	}

	// New stage should have dependencies
	if len(newStage.Requires) != 2 {
		t.Errorf("len(Requires) = %d, want 2", len(newStage.Requires))
	}
	if newStage.Requires[0] != "dep1" {
		t.Errorf("Requires[0] = %q, want %q", newStage.Requires[0],
			"dep1")
	}
}

func TestStageOrder(t *testing.T) {
	formatter := NoopFormatter{}

	stages := []Stage{
		NewStage("stage1", formatter),
		NewStage("stage3", formatter).WithRequires("stage2"),
		NewStage("stage2", formatter).WithRequires("stage1"),
	}

	ordered, err := StageOrder(stages)
	if err != nil {
		t.Errorf("StageOrder error: %v", err)
	}
	if len(ordered) != 3 {
		t.Errorf("len(ordered) = %d, want 3", len(ordered))
	}
	if ordered[0].Name != "stage1" || ordered[1].Name != "stage2" ||
		ordered[2].Name != "stage3" {

		t.Fatalf(
			"unexpected order: %q, %q, %q", ordered[0].Name,
			ordered[1].Name, ordered[2].Name,
		)
	}
}

func TestStageOrderMissingDep(t *testing.T) {
	formatter := NoopFormatter{}
	stages := []Stage{
		NewStage("stage1", formatter).WithRequires("missing"),
	}
	_, err := StageOrder(stages)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestStageOrderCycle(t *testing.T) {
	formatter := NoopFormatter{}
	stages := []Stage{
		NewStage("a", formatter).WithRequires("b"),
		NewStage("b", formatter).WithRequires("a"),
	}
	_, err := StageOrder(stages)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestDefaultStages(t *testing.T) {
	cfg := NewBaseConfig(80, 8)
	stages := DefaultStages(cfg, false, nil)

	if len(stages) != 6 {
		t.Errorf("len(stages) = %d, want 6", len(stages))
	}

	// Check stage names
	expectedNames := []string{
		"comments",
		"compact-calls",
		"expressions",
		"multiline-calls",
		"signatures",
		"blank-lines",
	}

	for i, expected := range expectedNames {
		if stages[i].Name != expected {
			t.Errorf("stages[%d].Name = %q, want %q", i,
				stages[i].Name, expected)
		}
	}

	// Check dependencies are set
	if len(stages[0].Requires) > 0 {
		t.Error("First stage should have no dependencies")
	}
	if len(stages[1].Requires) == 0 {
		t.Error("Second stage should have dependencies")
	}
}

func TestStageChaining(t *testing.T) {
	formatter := NoopFormatter{}

	// Test that chaining works
	stage := NewStage("test", formatter).
		WithRequires("dep1").
		WithRequires("dep2")

	// Should have 2 dependencies (each WithRequires adds to existing)
	if len(stage.Requires) != 2 {
		t.Errorf("len(Requires) = %d, want 2", len(stage.Requires))
	}
}
