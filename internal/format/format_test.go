package format

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogsExamples(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "logs")
	in, err := os.ReadFile(filepath.Join(dir, "input.go"))
	if err != nil {
		t.Fatal(err)
	}
	got := FormatFile(in)
	want, err := os.ReadFile(filepath.Join(dir, "output.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
