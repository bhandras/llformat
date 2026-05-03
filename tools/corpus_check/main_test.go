package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width         int
		originalWidth int
		changed       bool
		want          string
	}{
		{
			name:          "new overflow",
			width:         88,
			originalWidth: 79,
			changed:       true,
			want:          "new_overflow",
		},
		{
			name:          "touched overflow",
			width:         88,
			originalWidth: 86,
			changed:       true,
			want:          "touched_overflow",
		},
		{
			name:          "shifted overflow",
			width:         88,
			originalWidth: 79,
			changed:       false,
			want:          "shifted_overflow",
		},
		{
			name:          "unchanged overflow",
			width:         88,
			originalWidth: 86,
			changed:       false,
			want:          "unchanged_overflow",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				got := classifyCase(
					test.width, test.originalWidth,
					test.changed, 80,
				)
				if got != test.want {
					t.Fatalf(
						"got %q, want %q", got,
						test.want,
					)
				}
			},
		)
	}
}

func TestGoFilesUnderExcludesDirsAndSuffixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep.go"), "package p\n")
	mustWriteFile(t, filepath.Join(root, "skip.pb.go"), "package p\n")
	mustMkdirAll(t, filepath.Join(root, "vendor"))
	mustWriteFile(
		t, filepath.Join(root, "vendor", "skip.go"),
		"package p\n",
	)
	mustMkdirAll(t, filepath.Join(root, "nested", "generated"))
	mustWriteFile(
		t, filepath.Join(root, "nested", "generated", "skip.go"),
		"package p\n",
	)

	got, err := goFilesUnder(
		root, []string{"vendor", "nested/generated"},
		[]string{".pb.go"},
	)
	if err != nil {
		t.Fatalf("goFilesUnder returned error: %v", err)
	}

	want := []string{filepath.Join(root, "keep.go")}
	if !equalStrings(got, want) {
		t.Fatalf("unexpected files:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestChangedLines(t *testing.T) {
	t.Parallel()

	got := changedLines([]byte("a\nb\nc\n"), []byte("a\nbb\nc\n"), 0)
	if !got[2] || got[1] || got[3] {
		t.Fatalf("unexpected changed lines: %#v", got)
	}
}

func TestFindOverflowsSkipsLLNolint(t *testing.T) {
	t.Parallel()

	src := []byte(
		strings.Join(
			[]string{
				"package p",
				"var a = \"this line is intentionally long\" //nolint:ll",
				"var b = \"this line is intentionally long too\" //nolint:gofmt,ll",
				"//nolint:ll",
				"var c = \"this line is intentionally long too\"",
				"var d = \"this line is still too long\"",
				"",
			}, "\n",
		),
	)

	got := findOverflows(src, 20, 8)
	if len(got) != 1 {
		t.Fatalf("got %d overflows, want 1: %#v", len(got), got)
	}
	if got[0].Line != 6 {
		t.Fatalf("got line %d, want 6", got[0].Line)
	}
}

func TestBuildClusters(t *testing.T) {
	t.Parallel()

	cases := []caseRecord{
		{
			Kind:        "new_overflow",
			Syntax:      "call",
			NodeKind:    "*ast.CallExpr",
			ClusterKey:  "new_overflow|call|*ast.CallExpr",
			Repo:        "repo",
			File:        "a.go",
			Line:        10,
			Width:       90,
			ColumnLimit: 80,
		},
		{
			Kind:        "new_overflow",
			Syntax:      "call",
			NodeKind:    "*ast.CallExpr",
			ClusterKey:  "new_overflow|call|*ast.CallExpr",
			Repo:        "repo",
			File:        "b.go",
			Line:        20,
			Width:       100,
			ColumnLimit: 80,
		},
	}

	clusters := buildClusters(cases, 80)
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(clusters))
	}
	if clusters[0].Count != 2 {
		t.Fatalf("got count %d, want 2", clusters[0].Count)
	}
	if clusters[0].MaxWidth != 100 {
		t.Fatalf("got max width %d, want 100", clusters[0].MaxWidth)
	}
	if clusters[0].AvgExcess != 15 {
		t.Fatalf(
			"got avg excess %.1f, want 15.0", clusters[0].AvgExcess,
		)
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
