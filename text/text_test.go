package text

import "testing"

func TestLeadingWhitespace(t *testing.T) {
	tests := []struct {
		input     string
		lineStart int
		want      string
	}{
		{
			"\t\thello",
			0,
			"\t\t",
		},
		{
			"  hello",
			0,
			"  ",
		},
		{
			"hello",
			0,
			"",
		},
		{
			"\n\t\tcode",
			1,
			"\t\t",
		},
		{
			"abc\n  def",
			4,
			"  ",
		},
	}
	for _, tt := range tests {
		got := string(LeadingWhitespace([]byte(tt.input), tt.lineStart))
		if got != tt.want {
			t.Errorf("LeadingWhitespace(%q, %d) = %q, want %q",
				tt.input, tt.lineStart, got, tt.want)
		}
	}
}

func TestLastLineStart(t *testing.T) {
	tests := []struct {
		input string
		pos   int
		want  int
	}{
		{
			"hello",
			3,
			0,
		},
		{
			"hello\nworld",
			8,
			6,
		},
		{
			"a\nb\nc",
			4,
			4,
		},
		{
			"first\nsecond\nthird",
			15,
			13,
		},
	}
	for _, tt := range tests {
		got := LastLineStart([]byte(tt.input), tt.pos)
		if got != tt.want {
			t.Errorf("LastLineStart(%q, %d) = %d, want %d",
				tt.input, tt.pos, got, tt.want)
		}
	}
}

func TestIsIdentifierStart(t *testing.T) {
	tests := []struct {
		input byte
		want  bool
	}{
		{
			'a',
			true,
		},
		{
			'z',
			true,
		},
		{
			'A',
			true,
		},
		{
			'Z',
			true,
		},
		{
			'_',
			true,
		},
		{
			'0',
			false,
		},
		{
			'9',
			false,
		},
		{
			' ',
			false,
		},
		{
			'.',
			false,
		},
	}
	for _, tt := range tests {
		got := IsIdentifierStart(tt.input)
		if got != tt.want {
			t.Errorf("IsIdentifierStart(%q) = %v, want %v",
				tt.input, got, tt.want)
		}
	}
}

func TestIsIdentifierChar(t *testing.T) {
	tests := []struct {
		input byte
		want  bool
	}{
		{
			'a',
			true,
		},
		{
			'Z',
			true,
		},
		{
			'_',
			true,
		},
		{
			'0',
			true,
		},
		{
			'9',
			true,
		},
		{
			' ',
			false,
		},
		{
			'.',
			false,
		},
	}
	for _, tt := range tests {
		got := IsIdentifierChar(tt.input)
		if got != tt.want {
			t.Errorf("IsIdentifierChar(%q) = %v, want %v", tt.input,
				got, tt.want)
		}
	}
}

func TestIsKeyword(t *testing.T) {
	keywords := []string{"break", "case", "chan", "const", "continue", "default",
		"defer", "else", "fallthrough", "for", "func", "go", "goto", "if",
		"import", "interface", "map", "package", "range", "return", "select",
		"struct", "switch", "type", "var"}

	for _, kw := range keywords {
		if !IsKeyword(kw) {
			t.Errorf("IsKeyword(%q) = false, want true", kw)
		}
	}

	nonKeywords := []string{"foo", "Bar", "myFunc", "println", "main"}
	for _, nk := range nonKeywords {
		if IsKeyword(nk) {
			t.Errorf("IsKeyword(%q) = true, want false", nk)
		}
	}
}
