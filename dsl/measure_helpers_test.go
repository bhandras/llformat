package dsl

import "testing"

func TestPrefixWidthAt(t *testing.T) {
	t.Parallel()

	src := []byte("\tfoo()\n\tbar()")
	// At the 'f' in "foo()", prefix is "\t" which has width 8.
	if got := prefixWidthAt(src, 1, 8); got != 8 {
		t.Fatalf("got %d", got)
	}
	// Prefix "\tfoo" is 8 + 3.
	if got := prefixWidthAt(src, 4, 8); got != 11 {
		t.Fatalf("got %d", got)
	}
}

func TestCollapsedSingleLineLen(t *testing.T) {
	t.Parallel()

	s := "a\t  b\nc"
	// Collapses to "a b c" which has length 5 in visual width.
	if got := collapsedSingleLineLen(s, 8); got != 5 {
		t.Fatalf("got %d", got)
	}
}

func TestCollapsedLineLenAt(t *testing.T) {
	t.Parallel()

	src := []byte("\tX := ")
	if got := collapsedLineLenAt(src, len(src), "a  b", 8); got != 8+5+3 {
		// "\tX := " is visual width 8+5 = 13, plus collapsed("a b") ==
		// 3.
		t.Fatalf("got %d", got)
	}
}
