package layout

import (
	"strings"
)

type mode int

const (
	modeFlat mode = iota
	modeBreak
)

type frame struct {
	doc    Doc
	indent string
	mode   mode
}

// Render renders a document into a string with the given column limit.
// indent is the initial indentation for any broken lines.
func Render(doc Doc, colLimit int, indent string) string {
	var out strings.Builder
	col := 0

	stack := []frame{{doc: doc, indent: indent, mode: modeBreak}}

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch d := f.doc.(type) {
		case Text:
			s := string(d)
			out.WriteString(s)
			col += len(s)
		case Line:
			if f.mode == modeFlat {
				out.WriteByte(' ')
				col++
			} else {
				out.WriteByte('\n')
				out.WriteString(f.indent)
				col = len(f.indent)
			}
		case Concat:
			// Push in reverse so we render in order.
			for i := len(d) - 1; i >= 0; i-- {
				stack = append(stack, frame{doc: d[i], indent: f.indent, mode: f.mode})
			}
		case Nest:
			stack = append(stack, frame{doc: d.Doc, indent: f.indent + d.Indent, mode: f.mode})
		case Group:
			// Try flat: if it doesn't fit, render broken.
			if fits(d.Doc, colLimit-col, f.indent) {
				stack = append(stack, frame{doc: d.Doc, indent: f.indent, mode: modeFlat})
			} else {
				stack = append(stack, frame{doc: d.Doc, indent: f.indent, mode: modeBreak})
			}
		default:
			// Unknown node: ignore.
		}
	}

	return out.String()
}

func fits(doc Doc, remaining int, indent string) bool {
	// Conservative fit check: any broken line forces failure.
	stack := []frame{{doc: doc, indent: indent, mode: modeFlat}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch d := f.doc.(type) {
		case Text:
			remaining -= len(string(d))
			if remaining < 0 {
				return false
			}
		case Line:
			// Flat line becomes a single space.
			remaining--
			if remaining < 0 {
				return false
			}
		case Concat:
			for i := len(d) - 1; i >= 0; i-- {
				stack = append(stack, frame{doc: d[i], indent: f.indent, mode: modeFlat})
			}
		case Nest:
			stack = append(stack, frame{doc: d.Doc, indent: f.indent + d.Indent, mode: modeFlat})
		case Group:
			stack = append(stack, frame{doc: d.Doc, indent: f.indent, mode: modeFlat})
		default:
			// Unknown node: ignore.
		}
	}
	return true
}
