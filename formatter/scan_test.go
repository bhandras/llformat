package formatter

import "testing"

func TestScannerBasics(t *testing.T) {
	src := []byte("hello world")
	s := NewScanner(src)

	if s.Pos() != 0 {
		t.Errorf("initial Pos() = %d, want 0", s.Pos())
	}
	if s.AtEnd() {
		t.Error("AtEnd() = true at start")
	}
	if s.Peek() != 'h' {
		t.Errorf("Peek() = %q, want 'h'", s.Peek())
	}
	if s.PeekAt(1) != 'e' {
		t.Errorf("PeekAt(1) = %q, want 'e'", s.PeekAt(1))
	}

	b := s.Advance()
	if b != 'h' {
		t.Errorf("Advance() = %q, want 'h'", b)
	}
	if s.Pos() != 1 {
		t.Errorf("Pos() after Advance = %d, want 1", s.Pos())
	}

	s.AdvanceBy(5)
	if s.Pos() != 6 {
		t.Errorf("Pos() after AdvanceBy(5) = %d, want 6", s.Pos())
	}

	s.SetPos(0)
	if s.Pos() != 0 {
		t.Errorf("Pos() after SetPos(0) = %d, want 0", s.Pos())
	}
}

func TestScannerSlicing(t *testing.T) {
	src := []byte("hello world")
	s := NewScanner(src)

	s.SetPos(6)
	slice := s.Slice(0, 5)
	if string(slice) != "hello" {
		t.Errorf("Slice(0, 5) = %q, want %q", slice, "hello")
	}

	sliceFrom := s.SliceFrom(0)
	if string(sliceFrom) != "hello " {
		t.Errorf("SliceFrom(0) = %q, want %q", sliceFrom, "hello ")
	}

	rem := s.Remaining()
	if string(rem) != "world" {
		t.Errorf("Remaining() = %q, want %q", rem, "world")
	}
}

func TestScannerSkipLiteral(t *testing.T) {
	tests := []struct {
		input   string
		wantPos int
		wantOK  bool
	}{
		{
			`"hello" rest`,
			7,
			true,
		},
		{
			"`raw` rest",
			5,
			true,
		},
		{
			"// comment\nrest",
			11,
			true,
		},
		{
			"/* block */ rest",
			11,
			true,
		},
		{
			"regular code",
			0,
			false,
		},
	}
	for _, tt := range tests {
		s := NewScanner([]byte(tt.input))
		ok := s.SkipLiteral()
		if ok != tt.wantOK {
			t.Errorf("SkipLiteral(%q) = %v, want %v", tt.input, ok,
				tt.wantOK)
		}
		if ok && s.Pos() != tt.wantPos {
			t.Errorf("SkipLiteral(%q) pos = %d, want %d", tt.input,
				s.Pos(), tt.wantPos)
		}
	}
}

func TestScannerMatch(t *testing.T) {
	src := []byte("log.Infof(")
	s := NewScanner(src)

	if !s.Match("log.Infof(") {
		t.Error("Match(\"log.Infof(\") = false, want true")
	}
	if s.Match("log.Debugf(") {
		t.Error("Match(\"log.Debugf(\") = true, want false")
	}

	s.SetPos(4)
	if !s.Match("Infof(") {
		t.Error("Match(\"Infof(\") at pos 4 = false, want true")
	}
}

func TestScannerIsAt(t *testing.T) {
	tests := []struct {
		input         string
		wantString    bool
		wantLineComm  bool
		wantBlockComm bool
	}{
		{
			`"hello"`,
			true,
			false,
			false,
		},
		{
			"`raw`",
			true,
			false,
			false,
		},
		{
			"// comment",
			false,
			true,
			false,
		},
		{
			"/* block */",
			false,
			false,
			true,
		},
		{
			"code",
			false,
			false,
			false,
		},
	}
	for _, tt := range tests {
		s := NewScanner([]byte(tt.input))
		if got := s.IsAtString(); got != tt.wantString {
			t.Errorf("IsAtString(%q) = %v, want %v", tt.input, got,
				tt.wantString)
		}
		if got := s.IsAtLineComment(); got != tt.wantLineComm {
			t.Errorf("IsAtLineComment(%q) = %v, want %v", tt.input,
				got, tt.wantLineComm)
		}
		if got := s.IsAtBlockComment(); got != tt.wantBlockComm {
			t.Errorf("IsAtBlockComment(%q) = %v, want %v", tt.input,
				got, tt.wantBlockComm)
		}
	}
}

func TestScannerSkipWhitespace(t *testing.T) {
	src := []byte("   \t  code")
	s := NewScanner(src)
	s.SkipWhitespace()
	if s.Pos() != 6 {
		t.Errorf("Pos() after SkipWhitespace = %d, want 6", s.Pos())
	}

	// Doesn't skip newlines
	src2 := []byte("  \ncode")
	s2 := NewScanner(src2)
	s2.SkipWhitespace()
	if s2.Pos() != 2 {
		t.Errorf("Pos() after SkipWhitespace with newline = %d, want 2",
			s2.Pos())
	}
}

func TestScannerSkipToNewline(t *testing.T) {
	src := []byte("hello\nworld")
	s := NewScanner(src)
	s.SkipToNewline()
	if s.Pos() != 5 {
		t.Errorf("Pos() after SkipToNewline = %d, want 5", s.Pos())
	}
	if s.Peek() != '\n' {
		t.Errorf("Peek() after SkipToNewline = %q, want '\\n'",
			s.Peek())
	}
}

func TestScannerSkipPastNewline(t *testing.T) {
	src := []byte("hello\nworld")
	s := NewScanner(src)
	ok := s.SkipPastNewline()
	if !ok {
		t.Error("SkipPastNewline() = false, want true")
	}
	if s.Pos() != 6 {
		t.Errorf("Pos() after SkipPastNewline = %d, want 6", s.Pos())
	}
	if s.Peek() != 'w' {
		t.Errorf("Peek() after SkipPastNewline = %q, want 'w'",
			s.Peek())
	}
}

func TestScannerBalancedParen(t *testing.T) {
	src := []byte("(a, (b, c))")
	s := NewScanner(src)
	closePos := s.ScanBalancedParen()
	if closePos != 10 {
		t.Errorf("ScanBalancedParen() = %d, want 10", closePos)
	}
	// Position should not change
	if s.Pos() != 0 {
		t.Errorf("Pos() after ScanBalancedParen = %d, want 0", s.Pos())
	}
}

func TestScannerAtEnd(t *testing.T) {
	s := NewScanner([]byte("ab"))
	s.Advance()
	s.Advance()
	if !s.AtEnd() {
		t.Error("AtEnd() = false after reading all bytes")
	}
	if s.Peek() != 0 {
		t.Errorf("Peek() at end = %d, want 0", s.Peek())
	}
	if s.Advance() != 0 {
		t.Errorf("Advance() at end = %d, want 0", s.Advance())
	}
}

func TestScannerBounds(t *testing.T) {
	s := NewScanner([]byte("abc"))

	// SetPos bounds checking
	s.SetPos(-10)
	if s.Pos() != 0 {
		t.Errorf("SetPos(-10) resulted in Pos() = %d, want 0", s.Pos())
	}

	s.SetPos(100)
	if s.Pos() != 3 {
		t.Errorf("SetPos(100) resulted in Pos() = %d, want 3", s.Pos())
	}

	// PeekAt bounds
	s.SetPos(0)
	if s.PeekAt(-1) != 0 {
		t.Errorf("PeekAt(-1) = %d, want 0", s.PeekAt(-1))
	}
	if s.PeekAt(100) != 0 {
		t.Errorf("PeekAt(100) = %d, want 0", s.PeekAt(100))
	}

	// Slice bounds
	slice := s.Slice(-5, 100)
	if string(slice) != "abc" {
		t.Errorf("Slice(-5, 100) = %q, want %q", slice, "abc")
	}
	slice = s.Slice(5, 2)
	if slice != nil {
		t.Errorf("Slice(5, 2) = %v, want nil", slice)
	}
}
