package formatter

// DefaultMultilineExcludes returns function name substrings that the generic
// multiline call formatter should exclude from formatting.
//
// These are calls handled by the log/printf stage, and keeping this list in one
// place avoids mismatches between stages (e.g. auto call-arg expression edits).
func DefaultMultilineExcludes() []string {
	return []string{
		"log.Infof", "log.Debugf", "log.Tracef", "log.Errorf", "log.Warnf",
		"fmt.Printf", "fmt.Sprintf", "fmt.Errorf",
	}
}
