package dsl

import "testing"

func TestSkipHorizontalWhitespace(t *testing.T) {
	t.Parallel()

	src := []byte(" \t\tabc")
	if got := skipHorizontalWhitespace(src, 0); got != 3 {
		t.Fatalf("got %d", got)
	}
	if got := skipHorizontalWhitespace(src, 3); got != 3 {
		t.Fatalf("got %d", got)
	}
}

func TestApplyContinuationIndent(t *testing.T) {
	t.Parallel()

	src := []byte("a  b")
	out, changed, err := applyContinuationIndent(src, 1, 3, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if string(out) != "a\n\tb" {
		t.Fatalf("got %q", string(out))
	}

	// No-op when the span already equals the replacement.
	out2, changed2, err := applyContinuationIndent(out, 1, 3, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed2 {
		t.Fatalf("expected changed=false")
	}
	if string(out2) != string(out) {
		t.Fatalf("got %q", string(out2))
	}
}

