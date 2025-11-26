package scanner

import (
	"reflect"
	"testing"
)

func TestScanString(t *testing.T) {
	tests := []struct {
		input string
		start int
		want  int
	}{
		{`"hello"`, 0, 7},
		{`"hello\"world"`, 0, 14},
		{"`raw string`", 0, 12},
		{`prefix "str" suffix`, 7, 12},
		{`"unterminated`, 0, 13},
	}
	for _, tt := range tests {
		got := ScanString([]byte(tt.input), tt.start)
		if got != tt.want {
			t.Errorf("ScanString(%q, %d) = %d, want %d", tt.input, tt.start, got, tt.want)
		}
	}
}

func TestScanLineComment(t *testing.T) {
	tests := []struct {
		input string
		start int
		want  int
	}{
		{"// comment\ncode", 0, 11},
		{"// comment", 0, 10},
		{"code // comment\nmore", 5, 16},
	}
	for _, tt := range tests {
		got := ScanLineComment([]byte(tt.input), tt.start)
		if got != tt.want {
			t.Errorf("ScanLineComment(%q, %d) = %d, want %d", tt.input, tt.start, got, tt.want)
		}
	}
}

func TestScanBlockComment(t *testing.T) {
	tests := []struct {
		input string
		start int
		want  int
	}{
		{"/* comment */code", 0, 13},
		{"/* multi\nline */x", 0, 16},
		{"/* unterminated", 0, 15},
	}
	for _, tt := range tests {
		got := ScanBlockComment([]byte(tt.input), tt.start)
		if got != tt.want {
			t.Errorf("ScanBlockComment(%q, %d) = %d, want %d", tt.input, tt.start, got, tt.want)
		}
	}
}

func TestScanBalancedParen(t *testing.T) {
	tests := []struct {
		input string
		open  int
		want  int
	}{
		{"(a, b)", 0, 5},
		{"(a, (b, c))", 0, 10},
		{"(a, \")\", b)", 0, 10},
		{"((()))", 0, 5},
		{"(unclosed", 0, -1},
	}
	for _, tt := range tests {
		got := ScanBalancedParen([]byte(tt.input), tt.open)
		if got != tt.want {
			t.Errorf("ScanBalancedParen(%q, %d) = %d, want %d", tt.input, tt.open, got, tt.want)
		}
	}
}

func TestSplitTopLevel(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a, b, c", []string{"a", "b", "c"}},
		{"a, (b, c), d", []string{"a", "(b, c)", "d"}},
		{"a, b[1,2], c", []string{"a", "b[1", "2]", "c"}}, // brackets NOT respected by design
		{`a, "b,c", d`, []string{"a", `"b,c"`, "d"}},
	}
	for _, tt := range tests {
		got := SplitTopLevel(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitTopLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSplitTopLevelAny(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a, b, c", []string{"a", "b", "c"}},
		{"a, (b, c), d", []string{"a", "(b, c)", "d"}},
		{"a, b[1,2], c", []string{"a", "b[1,2]", "c"}}, // brackets respected
		{"a, {b, c}, d", []string{"a", "{b, c}", "d"}}, // braces respected
		{`a, "b,c", d`, []string{"a", `"b,c"`, "d"}},
	}
	for _, tt := range tests {
		got := SplitTopLevelAny(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitTopLevelAny(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
