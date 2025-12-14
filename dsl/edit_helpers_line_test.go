package dsl

import "testing"

func TestLineStart(t *testing.T) {
	t.Parallel()

	src := []byte("a\nbc\ndef")
	if got := lineStart(src, 0); got != 0 {
		t.Fatalf("got %d", got)
	}
	if got := lineStart(src, 1); got != 0 {
		t.Fatalf("got %d", got)
	}
	if got := lineStart(src, 2); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := lineStart(src, len(src)); got != 5 {
		t.Fatalf("got %d", got)
	}
}

func TestLineEnd(t *testing.T) {
	t.Parallel()

	src := []byte("a\nbc\ndef")
	if got := lineEnd(src, 0); got != 1 {
		t.Fatalf("got %d", got)
	}
	if got := lineEnd(src, 2); got != 4 {
		t.Fatalf("got %d", got)
	}
	if got := lineEnd(src, 5); got != len(src) {
		t.Fatalf("got %d", got)
	}
}
