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
type Doc interface {
	isDoc()
}

type Text string

func (Text) isDoc() {}

type Line struct{}

func (Line) isDoc() {}

type Concat []Doc

func (Concat) isDoc() {}

type Group struct{ Doc Doc }

func (Group) isDoc() {}

type Nest struct {
	Indent string
	Doc    Doc
}

func (Nest) isDoc() {}

func T(s string) Doc { return Text(s) }
func L() Doc         { return Line{} }
func G(d Doc) Doc    { return Group{Doc: d} }
func N(indent string, d Doc) Doc {
	return Nest{Indent: indent, Doc: d}
}

func C(docs ...Doc) Doc { return Concat(docs) }
