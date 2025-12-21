package formatter

import "github.com/lightninglabs/llformat/text"

func isIdentStart(c byte) bool { return text.IsIdentifierStart(c) }

func isIdentChar(c byte) bool { return text.IsIdentifierChar(c) }
