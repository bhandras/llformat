package formatter

// NoopFormatter returns its input unchanged. It is used as a stage placeholder
// when a pipeline disables a stage without falling back to legacy behavior.
type NoopFormatter struct{}

func (NoopFormatter) FormatFile(src []byte) []byte {
	return src
}
