package dsl

import "testing"

func TestEditBuilderApply(t *testing.T) {
	t.Parallel()

	src := []byte("abc123xyz")

	var b EditBuilder
	b.Replace(3, 6, []byte("DEF")) // 123 -> DEF
	b.Insert(0, []byte("!"))

	got, changed, err := b.Apply(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if string(got) != "!abcDEFxyz" {
		t.Fatalf("got %q", string(got))
	}
}

func TestEditBuilderOverlapErrors(t *testing.T) {
	t.Parallel()

	src := []byte("abcdef")
	var b EditBuilder
	b.Replace(1, 3, []byte("X"))
	b.Replace(2, 4, []byte("Y"))

	_, _, err := b.Apply(src)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestEditBuilderNoOpsFiltered(t *testing.T) {
	t.Parallel()

	src := []byte("hello")
	var b EditBuilder
	b.Replace(0, 5, []byte("hello")) // no-op
	b.Insert(5, nil)                 // no-op

	got, changed, err := b.Apply(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatalf("expected changed=false")
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", string(got))
	}
}
