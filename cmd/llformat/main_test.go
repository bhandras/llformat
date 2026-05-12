package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunWriteMultiplePaths(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "one.go"),
		filepath.Join(dir, "two.go"),
	}
	src := []byte(`package p

import "fmt"

func f() error {
	return fmt.Errorf("this is a deliberately long error message that should split")
}
`)

	for _, path := range paths {
		require.NoError(t, os.WriteFile(path, src, 0o600))
	}

	var stdout, stderr bytes.Buffer
	code := run(
		[]string{"-w", "--col", "50", paths[0], paths[1]}, &stdout,
		&stderr,
	)

	require.Equal(t, 0, code, stderr.String())
	require.Empty(t, stdout.String())
	for _, path := range paths {
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Contains(t, string(got), "\"this is a \" +")
		require.Contains(
			t, string(got),
			"\"deliberately long error \" +",
		)
	}
}

func TestRunMultiplePathsRequireWrite(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.go")
	two := filepath.Join(dir, "two.go")
	require.NoError(t, os.WriteFile(one, []byte("package p\n"), 0o600))
	require.NoError(t, os.WriteFile(two, []byte("package p\n"), 0o600))

	var stdout, stderr bytes.Buffer
	code := run([]string{one, two}, &stdout, &stderr)

	require.Equal(t, 2, code)
	require.Empty(t, stdout.String())
	require.Contains(
		t, stderr.String(),
		"multiple paths require -w/--write",
	)
}

func TestRunVersionCommand(t *testing.T) {
	oldVersion, oldCommit := buildVersion, buildCommit
	buildVersion, buildCommit = "v1.2.3-test", "abcdef123456"
	t.Cleanup(func() {
		buildVersion, buildCommit = oldVersion, oldCommit
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr)

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	require.Equal(
		t, "llformat version v1.2.3-test\ncommit abcdef123456\n",
		stdout.String(),
	)
}

func TestRunVersionFlag(t *testing.T) {
	oldVersion, oldCommit := buildVersion, buildCommit
	buildVersion, buildCommit = "v1.2.3-test", "abcdef123456"
	t.Cleanup(func() {
		buildVersion, buildCommit = oldVersion, oldCommit
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr)

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	require.Equal(
		t, "llformat version v1.2.3-test\ncommit abcdef123456\n",
		stdout.String(),
	)
}

func TestShortCommit(t *testing.T) {
	require.Equal(t, "abcdef123456", shortCommit("abcdef1234567890"))
	require.Equal(t, "abc", shortCommit("abc"))
}
