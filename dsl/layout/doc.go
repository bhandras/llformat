package layout

// Doc is a tiny pretty-printing document tree.
//
// This is intentionally minimal: it's a foundation for gradually migrating
// string-based heuristic formatters to a single stable layout engine.
//
// The model is:
// - Text emits literal text.
// - Line is either a space (flat mode) or a newline+indent (break mode).
// - Group tries to render flat, otherwise renders broken.
// - Concat concatenates docs.
// - Nest increases indentation for any broken lines within its child.
// - IfBreak chooses between 2 docs based on mode.
// - Align indents broken lines to the current column.
type Doc interface {
	isDoc()
}

type Text string

func (Text) isDoc() {}

type Line struct{}

func (Line) isDoc() {}

// SoftLine is either "" (flat mode) or a newline+indent (break mode). This is
// useful for tokens where inserting a space in flat mode would be invalid, e.g.
// selector chains: `foo.\n\tBar()` vs `foo.Bar()`.
type SoftLine struct{}

func (SoftLine) isDoc() {}

type Concat []Doc

func (Concat) isDoc() {}

type Group struct{ Doc Doc }

func (Group) isDoc() {}

type Nest struct {
	Indent string
	Doc    Doc
}

func (Nest) isDoc() {}

// IfBreak selects between two docs based on the rendering mode.
// In flat mode, Flat is rendered; in break mode, Broken is rendered.
type IfBreak struct {
	Broken Doc
	Flat   Doc
}

func (IfBreak) isDoc() {}

// Align renders its child with the indentation set to the current column
// (spaces) for any broken lines within its child.
//
// This is primarily used for alignment-based indentation (e.g. after an opening
// paren). It intentionally uses spaces to precisely match the measured column
// width, even when the surrounding code uses tabs.
type Align struct {
	Doc Doc
}

func (Align) isDoc() {}

func T(s string) Doc { return Text(s) }
func L() Doc         { return Line{} }
func SL() Doc        { return SoftLine{} }
func G(d Doc) Doc    { return Group{Doc: d} }
func N(indent string, d Doc) Doc {
	return Nest{Indent: indent, Doc: d}
}

func C(docs ...Doc) Doc { return Concat(docs) }

func IB(broken, flat Doc) Doc { return IfBreak{Broken: broken, Flat: flat} }
func A(d Doc) Doc             { return Align{Doc: d} }
