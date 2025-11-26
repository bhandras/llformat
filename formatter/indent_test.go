package formatter

import "testing"

func TestIndentFromLine(t *testing.T) {
	tests := []struct {
		src       string
		lineStart int
		tabStop   int
		wantBase  string
		wantWidth int
	}{
		{"\t\thello", 0, 8, "\t\t", 16},
		{"  hello", 0, 8, "  ", 2},
		{"hello", 0, 8, "", 0},
		{"\n\t\tcode", 1, 8, "\t\t", 16},
		{"abc\n  def", 4, 8, "  ", 2},
		{"\t hello", 0, 4, "\t ", 5}, // tab(4) + space(1) = 5
	}
	for _, tt := range tests {
		indent := IndentFromLine([]byte(tt.src), tt.lineStart, tt.tabStop)
		if indent.Base != tt.wantBase {
			t.Errorf("IndentFromLine(%q, %d, %d).Base = %q, want %q",
				tt.src, tt.lineStart, tt.tabStop, indent.Base, tt.wantBase)
		}
		if indent.Width() != tt.wantWidth {
			t.Errorf("IndentFromLine(%q, %d, %d).Width() = %d, want %d",
				tt.src, tt.lineStart, tt.tabStop, indent.Width(), tt.wantWidth)
		}
	}
}

func TestIndentFromSource(t *testing.T) {
	src := "first\n\t\tsecond line"
	indent := IndentFromSource([]byte(src), 10, 8) // pos 10 is in "second"
	if indent.Base != "\t\t" {
		t.Errorf("Base = %q, want %q", indent.Base, "\t\t")
	}
	if indent.Width() != 16 {
		t.Errorf("Width() = %d, want 16", indent.Width())
	}
}

func TestIndentContinuation(t *testing.T) {
	indent := NewIndent("\t", 8)
	cont := indent.Continuation()

	if cont.Base != "\t\t" {
		t.Errorf("Continuation().Base = %q, want %q", cont.Base, "\t\t")
	}
	if cont.Width() != 16 {
		t.Errorf("Continuation().Width() = %d, want 16", cont.Width())
	}
	// Original should be unchanged
	if indent.Base != "\t" {
		t.Errorf("original Base changed to %q, want %q", indent.Base, "\t")
	}
}

func TestIndentWithExtra(t *testing.T) {
	indent := NewIndent("\t", 8)
	extra := indent.WithExtra("  ")

	if extra.Base != "\t  " {
		t.Errorf("WithExtra(\"  \").Base = %q, want %q", extra.Base, "\t  ")
	}
	// tab to 8, then 2 spaces = 10
	if extra.Width() != 10 {
		t.Errorf("WithExtra(\"  \").Width() = %d, want 10", extra.Width())
	}
}

func TestIndentWithSpaces(t *testing.T) {
	indent := NewIndent("", 8)
	spaced := indent.WithSpaces(4)

	if spaced.Base != "    " {
		t.Errorf("WithSpaces(4).Base = %q, want %q", spaced.Base, "    ")
	}
	if spaced.Width() != 4 {
		t.Errorf("WithSpaces(4).Width() = %d, want 4", spaced.Width())
	}
}

func TestIndentIsEmpty(t *testing.T) {
	empty := NewIndent("", 8)
	if !empty.IsEmpty() {
		t.Error("IsEmpty() = false for empty indent")
	}

	nonEmpty := NewIndent("\t", 8)
	if nonEmpty.IsEmpty() {
		t.Error("IsEmpty() = true for non-empty indent")
	}
}

func TestIndentString(t *testing.T) {
	indent := NewIndent("\t  ", 8)
	if indent.String() != "\t  " {
		t.Errorf("String() = %q, want %q", indent.String(), "\t  ")
	}
}

func TestIndentBytes(t *testing.T) {
	indent := NewIndent("\t", 8)
	if string(indent.Bytes()) != "\t" {
		t.Errorf("Bytes() = %q, want %q", indent.Bytes(), "\t")
	}
}

func TestIndentFitsContent(t *testing.T) {
	indent := NewIndent("\t", 8) // width 8
	cfg := NewBaseConfig(80, 8)

	// 8 (indent) + 70 = 78, fits
	long70 := "1234567890123456789012345678901234567890123456789012345678901234567890"
	if !indent.FitsContent(long70, cfg) {
		t.Error("FitsContent should return true for 70 chars with 8-width indent")
	}

	// 8 (indent) + 73 = 81, doesn't fit
	long73 := "1234567890123456789012345678901234567890123456789012345678901234567890123"
	if indent.FitsContent(long73, cfg) {
		t.Error("FitsContent should return false for 73 chars with 8-width indent")
	}
}

func TestIndentColumn(t *testing.T) {
	indent := NewIndent("\t\t", 8)
	if indent.Column() != 16 {
		t.Errorf("Column() = %d, want 16", indent.Column())
	}
}
