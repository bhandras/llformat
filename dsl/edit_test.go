package dsl

import (
	"testing"
)

func TestApplyEdits(t *testing.T) {
	t.Parallel()

	t.Run("replace", func(t *testing.T) {
		t.Parallel()

		got, err := ApplyEdits([]byte("hello world"), []Edit{
			{Start: 6, End: 11, Replace: []byte("gophers")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "hello gophers" {
			t.Fatalf("got %q", string(got))
		}
	})

	t.Run("insert", func(t *testing.T) {
		t.Parallel()

		got, err := ApplyEdits([]byte("helo"), []Edit{
			{Start: 2, End: 2, Replace: []byte("l")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "hello" {
			t.Fatalf("got %q", string(got))
		}
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		got, err := ApplyEdits([]byte("hello cruel world"), []Edit{
			{Start: 5, End: 11, Replace: nil}, // remove " cruel"
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "hello world" {
			t.Fatalf("got %q", string(got))
		}
	})

	t.Run("multiple_unsorted", func(t *testing.T) {
		t.Parallel()

		got, err := ApplyEdits([]byte("abc123xyz"), []Edit{
			{Start: 3, End: 6, Replace: []byte("DEF")},
			{Start: 0, End: 3, Replace: []byte("ABC")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "ABCDEFxyz" {
			t.Fatalf("got %q", string(got))
		}
	})

	t.Run("overlap_error", func(t *testing.T) {
		t.Parallel()

		_, err := ApplyEdits([]byte("abcdef"), []Edit{
			{Start: 1, End: 3, Replace: []byte("X")},
			{Start: 2, End: 4, Replace: []byte("Y")},
		})
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("bounds_error", func(t *testing.T) {
		t.Parallel()

		_, err := ApplyEdits([]byte("abc"), []Edit{
			{Start: 0, End: 4, Replace: []byte("X")},
		})
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

