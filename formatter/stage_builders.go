package formatter

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	llast "github.com/lightninglabs/llformat/ast"
	"github.com/lightninglabs/llformat/dsl"
)

func dslBudgetNext() dsl.RewriteBudget {
	// Large enough to never trigger for normal formatting runs, but small enough
	// to act as a safety valve against pathological rules.
	return dsl.RewriteBudget{
		MaxOutputBytes:   2 << 20, // 2 MiB
		MaxBytesIncrease: 1 << 20, // 1 MiB
	}
}

func ownedSpansForCalls(src []byte, match func(*ast.CallExpr) bool) llast.OffsetSpanSet {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", src, parser.AllErrors)
	if err != nil || file == nil {
		return llast.OffsetSpanSet{}
	}

	var spans []llast.OffsetSpan
	addSpan := func(startPos, endPos token.Pos) {
		if startPos == token.NoPos || endPos == token.NoPos {
			return
		}
		start := fset.Position(startPos).Offset
		end := fset.Position(endPos).Offset
		if start < 0 || end < 0 || start >= end {
			return
		}
		if start >= len(src) {
			return
		}
		if end > len(src) {
			end = len(src)
		}
		spans = append(spans, llast.OffsetSpan{Start: start, End: end})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call == nil {
			return true
		}
		if !match(call) {
			return true
		}

		// Own the call expression and its argument list so earlier stages do not
		// rewrite within regions that this stage will later format.
		addSpan(call.Pos(), call.End())
		if call.Lparen != token.NoPos && call.Rparen != token.NoPos {
			addSpan(call.Lparen, call.Rparen+1)
		}
		return true
	})

	return llast.NewOffsetSpanSet(spans)
}

func callNameForExcludes(call *ast.CallExpr) string {
	if call == nil {
		return ""
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if fun == nil {
			return ""
		}
		return fun.Name
	case *ast.SelectorExpr:
		return selectorExprName(fun)
	default:
		return ""
	}
}

func selectorExprName(sel *ast.SelectorExpr) string {
	if sel == nil || sel.Sel == nil {
		return ""
	}
	if sel.X == nil {
		return sel.Sel.Name
	}
	switch x := sel.X.(type) {
	case *ast.Ident:
		if x == nil {
			return sel.Sel.Name
		}
		return x.Name + "." + sel.Sel.Name
	case *ast.SelectorExpr:
		prefix := selectorExprName(x)
		if prefix == "" {
			return sel.Sel.Name
		}
		return prefix + "." + sel.Sel.Name
	default:
		// For cases like `foo().Bar()`, a full "pkg.Func" style name isn't
		// meaningful; fall back to the terminal selector name so exclude lists
		// can still match on suffixes.
		return sel.Sel.Name
	}
}

func excludedByName(call *ast.CallExpr, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}
	name := callNameForExcludes(call)
	if name == "" {
		return false
	}
	for _, ex := range excludes {
		if ex == "" {
			continue
		}
		if strings.Contains(name, ex) {
			return true
		}
	}
	return false
}

func buildCommentStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.Comments != StageModeDSL {
		return NoopFormatter{}
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.Comments.Rules,
		Trace:         opts.DSL.Trace,
		TraceReasons:  opts.DSL.TraceReasons,
		NodeOrder:     bundle.Comments.NodeOrder,
		MaxIterations: bundle.Comments.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetNext(),
	})
}

func buildCompactCallStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.LogCalls != StageModeDSL {
		return NoopFormatter{}
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.LogCalls.Rules,
		Trace:         opts.DSL.Trace,
		TraceReasons:  opts.DSL.TraceReasons,
		NodeOrder:     bundle.LogCalls.NodeOrder,
		MaxIterations: bundle.LogCalls.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetNext(),
		OwnedSpansFunc: func(src []byte) llast.OffsetSpanSet {
			// Align ownership boundaries with the log/printf DSL stage selection.
			cond := &dsl.IsLogOrPrintfCallCond{
				Target:                 "node",
				MatchAnySelectorPrefix: true,
				IncludeNonFStringCalls: true,
			}
			return ownedSpansForCalls(src, func(call *ast.CallExpr) bool {
				return cond.Eval(dsl.Captures{"node": call}, nil)
			})
		},
	})
}

func buildExpressionStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.Expressions != StageModeDSL {
		return NoopFormatter{}
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.Expressions.Rules,
		Trace:         opts.DSL.Trace,
		TraceReasons:  opts.DSL.TraceReasons,
		NodeOrder:     bundle.Expressions.NodeOrder,
		MaxIterations: bundle.Expressions.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetNext(),
	})
}

func buildMultiLineCallStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.MultiLineCalls != StageModeDSL {
		return NoopFormatter{}
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:       cfg.ColumnLimit,
		TabStop:           cfg.TabStop,
		Rules:             bundle.MultiLineCalls.Rules,
		Trace:             opts.DSL.Trace,
		TraceReasons:      opts.DSL.TraceReasons,
		NodeOrder:         bundle.MultiLineCalls.NodeOrder,
		MaxIterations:     bundle.MultiLineCalls.MaxIterations,
		AutoMaxIterations: bundle.MultiLineCalls.AutoMaxIterations,
		DetectCycles:      bundle.MultiLineCalls.DetectCycles,
		SkipGofmt:         true,
		StageName:         stageName,
		Budget:            dslBudgetNext(),
		OwnedSpansFunc: func(src []byte) llast.OffsetSpanSet {
			logCond := &dsl.IsLogOrPrintfCallCond{
				Target:                 "node",
				MatchAnySelectorPrefix: true,
				IncludeNonFStringCalls: true,
			}
			nonFLogCond := &dsl.IsNonFLogCallCond{Target: "node"}

			return ownedSpansForCalls(src, func(call *ast.CallExpr) bool {
				caps := dsl.Captures{"node": call}

				// Exclude printf-style calls handled by the log/printf stage.
				if logCond.Eval(caps, nil) {
					return false
				}

				// Also exclude non-printf log calls (e.g. log.Info(...)) to avoid
				// rewriting logger calls with the generic multiline-call stage.
				if nonFLogCond.Eval(caps, nil) {
					return false
				}

				// Respect the explicit multiline-exclude list.
				if excludedByName(call, opts.Style.Excludes) {
					return false
				}

				return true
			})
		},
	})
}

func buildSignatureStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.Signatures != StageModeDSL {
		return NoopFormatter{}
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:       cfg.ColumnLimit,
		TabStop:           cfg.TabStop,
		Rules:             bundle.Signatures.Rules,
		Trace:             opts.DSL.Trace,
		TraceReasons:      opts.DSL.TraceReasons,
		NodeOrder:         bundle.Signatures.NodeOrder,
		MaxIterations:     bundle.Signatures.MaxIterations,
		AutoMaxIterations: bundle.Signatures.AutoMaxIterations,
		DetectCycles:      bundle.Signatures.DetectCycles,
		SkipGofmt:         true,
		StageName:         stageName,
		Budget:            dslBudgetNext(),
	})
}

func buildBlankLineStageFormatter(stageName string, cfg BaseConfig, opts StageOptions, plan StagePlan, bundle DSLBundle) Formatter {
	if plan.BlankLines != StageModeDSL {
		return NoopFormatter{}
	}

	return NewDSLExprFormatter(DSLExprConfig{
		ColumnLimit:   cfg.ColumnLimit,
		TabStop:       cfg.TabStop,
		Rules:         bundle.BlankLines.Rules,
		Trace:         opts.DSL.Trace,
		TraceReasons:  opts.DSL.TraceReasons,
		NodeOrder:     bundle.BlankLines.NodeOrder,
		MaxIterations: bundle.BlankLines.MaxIterations,
		SkipGofmt:     true,
		StageName:     stageName,
		Budget:        dslBudgetNext(),
	})
}
