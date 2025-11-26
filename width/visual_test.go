package width

import "testing"

func TestVisualLenWithTab(t *testing.T) {
	tests := []struct {
		input   string
		tabStop int
		want    int
	}{
		{"hello", 8, 5},
		{"\t", 8, 8},
		{"\t\t", 8, 16},
		{"a\tb", 8, 9}, // a at 0, tab to 8, b at 8
		{"", 8, 0},
		{"\t", 4, 4},
		{"ab\t", 4, 4}, // ab at 0-2, tab to 4
	}
	for _, tt := range tests {
		got := VisualLenWithTab(tt.input, tt.tabStop)
		if got != tt.want {
			t.Errorf("VisualLenWithTab(%q, %d) = %d, want %d", tt.input, tt.tabStop, got, tt.want)
		}
	}
}

func TestAdvanceColsWithTab(t *testing.T) {
	tests := []struct {
		startCol int
		s        string
		tabStop  int
		want     int
	}{
		{0, "hello", 8, 5},
		{5, "world", 8, 10},
		{0, "a\nb", 8, 1}, // newline resets to 0, then 'b'
		{3, "\t", 8, 8},   // tab from col 3 goes to 8
		{8, "\t", 8, 16},  // tab from col 8 goes to 16
		{0, "\t\t", 8, 16}, // two tabs
		{1, "\t", 4, 4},   // tab from col 1 goes to 4 with tabStop=4
	}
	for _, tt := range tests {
		got := AdvanceColsWithTab(tt.startCol, tt.s, tt.tabStop)
		if got != tt.want {
			t.Errorf("AdvanceColsWithTab(%d, %q, %d) = %d, want %d", tt.startCol, tt.s, tt.tabStop, got, tt.want)
		}
	}
}

func TestFirstLineLenWithTab(t *testing.T) {
	tests := []struct {
		input   string
		tabStop int
		want    int
	}{
		{"hello", 8, 5},
		{"hello\nworld", 8, 5},
		{"\n", 8, 0},
		{"ab\ncd\nef", 8, 2},
		{"\thello", 8, 13}, // tab to 8, then hello (5)
	}
	for _, tt := range tests {
		got := FirstLineLenWithTab(tt.input, tt.tabStop)
		if got != tt.want {
			t.Errorf("FirstLineLenWithTab(%q, %d) = %d, want %d", tt.input, tt.tabStop, got, tt.want)
		}
	}
}

func TestLastLineLenWithTab(t *testing.T) {
	tests := []struct {
		input   string
		tabStop int
		want    int
	}{
		{"hello", 8, 5},
		{"hello\nworld", 8, 5},
		{"hello\n", 8, 0},
		{"ab\ncd\nef", 8, 2},
		{"hello\n\tworld", 8, 13}, // tab to 8, then world (5)
	}
	for _, tt := range tests {
		got := LastLineLenWithTab(tt.input, tt.tabStop)
		if got != tt.want {
			t.Errorf("LastLineLenWithTab(%q, %d) = %d, want %d", tt.input, tt.tabStop, got, tt.want)
		}
	}
}

func TestRuneWidth(t *testing.T) {
	tests := []struct {
		input rune
		want  int
	}{
		{'a', 1},
		{'Z', 1},
		{' ', 1},
		{0, 0},   // null
		{'\t', 0}, // tab is a control char, width is 0 (handled separately by AdvanceCols)
		{'中', 2}, // CJK character
		{'日', 2}, // Japanese
		{'한', 2}, // Korean
	}
	for _, tt := range tests {
		got := RuneWidth(tt.input)
		if got != tt.want {
			t.Errorf("RuneWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
