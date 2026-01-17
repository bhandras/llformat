package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGoFilesUnder_ExcludeSuffix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	mustWriteFile(t, filepath.Join(root, "a.go"), "package p\n")
	mustWriteFile(t, filepath.Join(root, "a.pb.go"), "package p\n")
	mustWriteFile(t, filepath.Join(root, "x.txt"), "nope\n")

	got, err := goFilesUnder(root, nil, stringSliceFlag{".pb.go"})
	if err != nil {
		t.Fatalf("goFilesUnder returned error: %v", err)
	}

	want := []string{
		filepath.Join(root, "a.go"),
	}
	if !equalStrings(got, want) {
		t.Fatalf("unexpected files:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestGoFilesUnder_ExcludeDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep.go"), "package p\n")
	mustMkdirAll(t, filepath.Join(root, "vendor"))
	mustWriteFile(
		t, filepath.Join(root, "vendor", "skip.go"), "package p\n",
	)

	got, err := goFilesUnder(root, stringSliceFlag{"vendor"})
	if err != nil {
		t.Fatalf("goFilesUnder returned error: %v", err)
	}
	want := []string{
		filepath.Join(root, "keep.go"),
	}
	if !equalStrings(got, want) {
		t.Fatalf("unexpected files:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestChangedLines_Replace(t *testing.T) {
	t.Parallel()

	original := []byte("a\nb\nc\n")
	formatted := []byte("a\nb2\nc\n")

	got := changedLines(original, formatted, 0)
	if !got[2] || got[1] || got[3] {
		t.Fatalf("unexpected changed lines map: %#v", got)
	}
}

func TestChangedLines_ReplaceWithContextRadius(t *testing.T) {
	t.Parallel()

	original := []byte("a\nb\nc\nd\n")
	formatted := []byte("a\nb2\nc\nd\n")

	got := changedLines(original, formatted, 1)
	if !got[1] || !got[2] || !got[3] || got[4] {
		t.Fatalf("unexpected changed lines map: %#v", got)
	}
}

func TestResolveOutput_MarkdownFile(t *testing.T) {
	t.Parallel()

	perBug, outDir, err := resolveOutput(
		"BUG_REPORTS.md", time.Unix(0, 0).UTC(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if perBug {
		t.Fatalf("expected perBug=false")
	}
	if outDir != "" {
		t.Fatalf("expected empty outDir, got %q", outDir)
	}
}

func TestResolveOutput_DirPrefix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	prefix := filepath.Join(root, "overflow_bug_report")
	perBug, outDir, err := resolveOutput(prefix, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !perBug {
		t.Fatalf("expected perBug=true")
	}
	if outDir == "" {
		t.Fatalf("expected non-empty outDir")
	}
	if _, err := os.Stat(outDir); err != nil {
		t.Fatalf("expected outDir to exist: %v", err)
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
