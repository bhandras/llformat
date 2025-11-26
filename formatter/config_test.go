package formatter

import "testing"

func TestNewBaseConfig(t *testing.T) {
	tests := []struct {
		col, tab int
		wantCol  int
		wantTab  int
	}{
		{0, 0, DefaultColumnLimit, DefaultTabStop},
		{100, 4, 100, 4},
		{-1, -1, DefaultColumnLimit, DefaultTabStop},
		{80, 0, 80, DefaultTabStop},
		{0, 4, DefaultColumnLimit, 4},
	}
	for _, tt := range tests {
		cfg := NewBaseConfig(tt.col, tt.tab)
		if cfg.ColumnLimit != tt.wantCol {
			t.Errorf("NewBaseConfig(%d, %d).ColumnLimit = %d, want %d",
				tt.col, tt.tab, cfg.ColumnLimit, tt.wantCol)
		}
		if cfg.TabStop != tt.wantTab {
			t.Errorf("NewBaseConfig(%d, %d).TabStop = %d, want %d",
				tt.col, tt.tab, cfg.TabStop, tt.wantTab)
		}
	}
}

func TestBaseConfigWidth(t *testing.T) {
	cfg := NewBaseConfig(80, 8)
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"\t", 8},
		{"\thello", 13},
		{"", 0},
	}
	for _, tt := range tests {
		got := cfg.Width(tt.input)
		if got != tt.want {
			t.Errorf("cfg.Width(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestBaseConfigWidthFrom(t *testing.T) {
	cfg := NewBaseConfig(80, 8)
	tests := []struct {
		col   int
		input string
		want  int
	}{
		{0, "hello", 5},
		{5, "world", 10},
		{3, "\t", 8},
		{8, "\t", 16},
	}
	for _, tt := range tests {
		got := cfg.WidthFrom(tt.col, tt.input)
		if got != tt.want {
			t.Errorf("cfg.WidthFrom(%d, %q) = %d, want %d",
				tt.col, tt.input, got, tt.want)
		}
	}
}

func TestBaseConfigFitsInLimit(t *testing.T) {
	cfg := NewBaseConfig(80, 8)
	tests := []struct {
		col     int
		content string
		want    bool
	}{
		{0, "hello", true},
		{75, "hello", true},  // 75 + 5 = 80, exactly at limit
		{76, "hello", false}, // 76 + 5 = 81, exceeds limit
		{0, "", true},
		{80, "", true},
	}
	for _, tt := range tests {
		got := cfg.FitsInLimit(tt.col, tt.content)
		if got != tt.want {
			t.Errorf("cfg.FitsInLimit(%d, %q) = %v, want %v",
				tt.col, tt.content, got, tt.want)
		}
	}
}

func TestBaseConfigRemaining(t *testing.T) {
	cfg := NewBaseConfig(80, 8)
	tests := []struct {
		col  int
		want int
	}{
		{0, 80},
		{40, 40},
		{80, 0},
		{100, 0},
	}
	for _, tt := range tests {
		got := cfg.Remaining(tt.col)
		if got != tt.want {
			t.Errorf("cfg.Remaining(%d) = %d, want %d", tt.col, got, tt.want)
		}
	}
}

func TestBaseConfigFirstLineWidth(t *testing.T) {
	cfg := NewBaseConfig(80, 8)
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"hello\nworld", 5},
		{"\n", 0},
		{"\thello\nworld", 13},
	}
	for _, tt := range tests {
		got := cfg.FirstLineWidth(tt.input)
		if got != tt.want {
			t.Errorf("cfg.FirstLineWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestBaseConfigLastLineWidth(t *testing.T) {
	cfg := NewBaseConfig(80, 8)
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"hello\nworld", 5},
		{"hello\n", 0},
		{"hello\n\tworld", 13},
	}
	for _, tt := range tests {
		got := cfg.LastLineWidth(tt.input)
		if got != tt.want {
			t.Errorf("cfg.LastLineWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
