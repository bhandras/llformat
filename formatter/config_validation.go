package formatter

import (
	"fmt"
	"sort"
	"strings"
)

// ValidatePipelineConfig performs best-effort validation of PipelineConfig.
//
// This is intended to catch specification/config mistakes early (unknown style
// values) and provide actionable errors for CLI callers.
//
// NewPipeline remains best-effort; callers that want strictness should validate
// up front.
func ValidatePipelineConfig(cfg PipelineConfig) error {
	var issues []configIssue

	validateOptionalEnum(
		&issues, "CommentMode", cfg.CommentMode,
		[]string{"prose", "overflow", "off"},
	)
	validateOptionalEnum(
		&issues, "DSLMultiLineStyle", cfg.DSLMultiLineStyle,
		allowedDSLMultiLineStyles(),
	)
	validateOptionalEnum(
		&issues, "DSLSigsStyle", cfg.DSLSigsStyle,
		allowedDSLSigsStyles(),
	)

	validateOptionalEnum(
		&issues, "DSLExprLogicalStyle", cfg.DSLExprLogicalStyle,
		allowedDSLExprStyles(),
	)
	validateOptionalEnum(
		&issues, "DSLExprArithmeticStyle", cfg.DSLExprArithmeticStyle,
		allowedDSLExprStyles(),
	)
	validateOptionalEnum(
		&issues, "DSLExprCaseClauseStyle", cfg.DSLExprCaseClauseStyle,
		allowedDSLExprStyles(),
	)
	validateOptionalEnum(
		&issues, "DSLExprSelectorChainStyle",
		cfg.DSLExprSelectorChainStyle, allowedDSLExprStyles(),
	)

	if cfg.StagePlanOverride != nil {
		validateStageMode(
			&issues, "StagePlanOverride.Comments",
			cfg.StagePlanOverride.Comments,
		)
		validateStageMode(
			&issues, "StagePlanOverride.LogCalls",
			cfg.StagePlanOverride.LogCalls,
		)
		validateStageMode(
			&issues, "StagePlanOverride.Expressions",
			cfg.StagePlanOverride.Expressions,
		)
		validateStageMode(
			&issues, "StagePlanOverride.MultiLineCalls",
			cfg.StagePlanOverride.MultiLineCalls,
		)
		validateStageMode(
			&issues, "StagePlanOverride.Signatures",
			cfg.StagePlanOverride.Signatures,
		)
		validateStageMode(
			&issues, "StagePlanOverride.BlankLines",
			cfg.StagePlanOverride.BlankLines,
		)
	}

	// Today Allow+Auto is well-defined (Allow wins), so this is not an
	// error. Still, it's almost always a mistake, so surface a warning-like
	// issue.
	if cfg.AllowDSLCallArgs && cfg.AutoDSLCallArgs {
		issues = append(
			issues, configIssue{
				field:   "AllowDSLCallArgs/AutoDSLCallArgs",
				value:   "both true",
				message: "both are enabled; AllowDSLCallArgs will override AutoDSLCallArgs",
			},
		)
	}

	if len(issues) == 0 {
		return nil
	}

	return configError{issues: issues}
}

type configIssue struct {
	field   string
	value   string
	message string
}

type configError struct {
	issues []configIssue
}

func (e configError) Error() string {
	var b strings.Builder
	b.WriteString("invalid llformat configuration:\n")
	for _, it := range e.issues {
		if it.value == "" {
			fmt.Fprintf(&b, "- %s: %s\n", it.field, it.message)
			continue
		}
		fmt.Fprintf(&b, "- %s=%q: %s\n", it.field, it.value, it.message)
	}

	return strings.TrimRight(b.String(), "\n")
}

func validateOptionalEnum(issues *[]configIssue, field string, value string,
	allowed []string) {

	if value == "" {
		return
	}
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	*issues = append(
		*issues, configIssue{
			field: field,
			value: value,
			message: fmt.Sprintf("unknown value (allowed: %s)",
				strings.Join(allowed, "|")),
		},
	)
}

func validateStageMode(issues *[]configIssue, field string, mode StageMode) {
	switch mode {
	case StageModeOff, StageModeDSL:
		return

	default:
		*issues = append(
			*issues, configIssue{
				field: field,
				value: string(mode),
				message: fmt.Sprintf("unknown stage mode "+
					"(allowed: %s|%s)", StageModeOff,
					StageModeDSL),
			},
		)
	}
}

func allowedDSLMultiLineStyles() []string {
	styles := []string{
		"packed",
		"packed-chain",
		"packed-chain-layout",
		"layout-chain",
		"layout-args",
		"layout-args-groups-pairs",
		"layout-all",
		"layout-all-groups-pairs",
	}
	sort.Strings(styles)

	return styles
}

func allowedDSLSigsStyles() []string {
	return []string{"legacy", "dsl"}
}

func allowedDSLExprStyles() []string {
	return []string{"legacy", "layout"}
}
