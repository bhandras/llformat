package text

// IsIdentifierStart returns true if c can start a Go identifier.
func IsIdentifierStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// IsIdentifierChar returns true if c can be part of a Go identifier.
func IsIdentifierChar(c byte) bool {
	return IsIdentifierStart(c) || (c >= '0' && c <= '9')
}

// goKeywords is the set of Go keywords that cannot be function names.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true,
	"continue": true, "default": true, "defer": true, "else": true,
	"fallthrough": true, "for": true, "func": true, "go": true,
	"goto": true, "if": true, "import": true, "interface": true,
	"map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true,
	"var": true,
}

// IsKeyword returns true if s is a Go keyword.
func IsKeyword(s string) bool {
	return goKeywords[s]
}
