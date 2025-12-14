package dsl

// continuationIndentBytes returns the standard llformat continuation indentation
// sequence used by many DSL actions: newline + original indent + one tab.
func continuationIndentBytes(indent string) []byte {
	return []byte("\n" + indent + "\t")
}

