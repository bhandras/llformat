package ast

import "testing"

func TestParseExpr(t *testing.T) {
	tests := []struct {
		input   string
		wantNil bool
	}{
		{
			"foo()",
			false,
		},
		{
			"x + y",
			false,
		},
		{
			"T{a: 1}",
			false,
		},
		{
			"invalid syntax !!!",
			true,
		},
		{
			"",
			true,
		},
	}
	for _, tt := range tests {
		got := ParseExpr(tt.input)
		if (got == nil) != tt.wantNil {
			t.Errorf("ParseExpr(%q) nil=%v, want nil=%v", tt.input,
				got == nil, tt.wantNil)
		}
	}
}

func TestIsCallExpr(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{
			"foo()",
			true,
		},
		{
			"pkg.Func(x, y)",
			true,
		},
		{
			"(fn())",
			true,
		},
		{
			"T{}",
			false,
		},
		{
			"x + y",
			false,
		},
		{
			"foo",
			false,
		},
	}
	for _, tt := range tests {
		got := IsCallExpr(tt.input)
		if got != tt.want {
			t.Errorf("IsCallExpr(%q) = %v, want %v", tt.input, got,
				tt.want)
		}
	}
}

func TestIsCompositeLit(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{
			"T{}",
			true,
		},
		{
			"[]int{1, 2, 3}",
			true,
		},
		{
			"map[string]int{}",
			true,
		},
		{
			"(T{})",
			true,
		},
		{
			"foo()",
			false,
		},
		{
			"x + y",
			false,
		},
	}
	for _, tt := range tests {
		got := IsCompositeLit(tt.input)
		if got != tt.want {
			t.Errorf("IsCompositeLit(%q) = %v, want %v", tt.input,
				got, tt.want)
		}
	}
}

func TestHasNestedCall(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{
			"foo(bar())",
			true,
		},
		{
			"foo(x, bar())",
			true,
		},
		{
			"foo((bar()))",
			true,
		},
		{
			"foo(a, b)",
			false,
		},
		{
			"foo()",
			false,
		},
		{
			"foo(T{})",
			false,
		},
	}
	for _, tt := range tests {
		got := HasNestedCall(tt.input)
		if got != tt.want {
			t.Errorf("HasNestedCall(%q) = %v, want %v", tt.input,
				got, tt.want)
		}
	}
}
