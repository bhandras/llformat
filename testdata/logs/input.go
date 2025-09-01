package main

import (
	"fmt"
)

// Minimal logger to make this test file compile.
type Logger struct{}

func (Logger) Infof(string, ...interface{})  {}
func (Logger) Debugf(string, ...interface{}) {}
func (Logger) Tracef(string, ...interface{}) {}
func (Logger) Errorf(string, ...interface{}) {}
func (Logger) Warnf(string, ...interface{})  {}
func (Logger) Info(...interface{})           {}

var (
	log                                                                                         Logger
	things                                                                                      []int
	longVariableNameHere                                                                        = struct{ Field int }{Field: 1}
	thisIsAReallyLongIdentifierNameThatGoesOnAndOnAndProbablyExceedsEightyColumnsWhenPrintedOut = 123
)

// Provide an entry point so this single file builds as an executable.
func main() {}

// example1: Long error format string followed by one expression.
//   - Exercises splitting a single long text arg and keeping the following expr
//     on the same line when possible.
func example1() error {
	return fmt.Errorf("this is a long error message with a couple (%d) place holders", len(things))
}

// example2: Input already broken across lines.
//   - Exercises reflow to the formatter's preferred layout: head on one line,
//     continuation indent with a tab, and packing next expr if it fits.
func example2() error {
	for i := 0; i < 10; i++ {
		log.Debugf(
			"Something happened here that we need to log: %v",
			longVariableNameHere,
		)
	}
	return nil
}

// example3: Concatenated string literal as first argument plus several expr
// args.
//   - Exercises flattening of string concatenation, strategic splitting to
//     leave room for trailing exprs on the same line, and tail merging
//     heuristics.
func example3(condition bool, userName, actionName string, timestamp,
	result interface{}) {

	if condition {
		log.Infof(
			"This is a long message "+
				"that we expect to break into multiple lines User %s performed action %s at %v with result %v.",
			userName, actionName, timestamp, result,
		)
	}
}

// example4: Simple fmt.Printf without wrapping pressure.
// - Should stay a single line after formatting.
func example4(someValue int) {
	fmt.Printf(
		"This is a formatted string with value: %d",
		someValue,
	)
}

// example5: Long single literal without convenient spaces.
// - Exercises forced cut with space preservation across "+" joins.
// - Ensures the "+" placement never exceeds the 80-column limit.
func example5() {
	log.Infof(
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	)
}

// example6: Many short expressions after a long message.
// - Exercises reservation for trailing exprs to keep several on the same line.
// - Packs as many expressions as fit without exceeding 80 columns.
func example6(userName string, a, b, c, d, e int) {
	log.Infof(
		"User %s did things with a bunch of values:",
		userName, a, b, c, d, e,
	)
}

// example7: Tabs in leading indent.
// - Exercises continuation indent as wsIndent + "\t".
// - Visual width treats tabs as 8-column stops.
func example7TabbedIndent() {
	log.Infof(
		"This message starts under a double-tab indent and should wrap nicely with a tab continuation.",
		42,
	)
}

// example8: Non-whitespace prefix before call (return + call).
// - Exercises continuation alignment to leading whitespace, not after "return".
func example8ReturnPrefix(x int) int {
	if x > 0 {
		// Using fmt.Sprintf to keep this an expression for formatting
		// purposes.
		return len(fmt.Sprintf(
			"Returning something with a long descriptive message that probably wraps nicely %d %d %d",
			x, x+1, x+2,
		))
	}
	return 0
}

// example9: Nested call as argument.
// - Exercises rewriting only the outer targeted call.
// - Inner call argument is printed as-is.
func example9Nested(i int) {
	log.Infof(
		"Outer %s",
		fmt.Sprintf("Inner number: %d with some trailing words that may wrap", i),
	)
}

// example10: Raw string literal input.
// - Exercises normalization to a quoted string and wrapping.
// - Verifies escaping of backslashes and quotes.
func example10Raw() {
	log.Infof(
		`path\\to\\file "quoted" with spaces and maybe enough length to require wrapping at some point`,
	)
}

// example11: Comments inside argument list.
// - Documents that comments inside call args are dropped by AST printing.
// - Golden should reflect comment loss.
func example11Comments(a, b int) {
	log.Warnf(
		"Message with args", // trailing comment after first arg
		a /* inline between args */, b,
	)
}

// example12: Similar names not in allowlist.
// - Ensures mylog.Infof and log.Info remain unchanged.
// - Only explicit targets are reformatted.
func example12NonTargets(mylog interface{ Infof(string, ...interface{}) }) {
	mylog.Infof("should remain untouched by formatter: %s %d", "x", 1)
	log.Info("non-f variant should be ignored, even if long long long long long long long long long long")
}

// example13: Multiple calls on one line.
// - Exercises independent detection and formatting of each call.
// - Both calls should wrap as needed.
func example13MultiCalls(n int) {
	log.Infof("short")
	log.Infof("Another message with a value %d that may require wrapping due to length", n)
}

// example14: Target patterns inside strings and comments.
// - Ensures scanner skips strings and comments; no formatting changes expected.
func example14FalsePositives() {
	// log.Infof("not actually a call")
	_ = "fmt.Printf( not a call either )"
}

// example15: Unicode and emoji in text.
// - Documents that visual width is byte-based; wrapping may look off visually.
func example15Unicode() {
	log.Infof("測試 with 😅 emoji and wide chars mixed in the sentence to approach the column limit")
}

// example16: Very long expression argument.
//   - Exercises keeping a long expr intact even if it exceeds width on a fresh
//     line.
func example16LongExpr() {
	log.Debugf(
		"Message with a very long identifier next",
		thisIsAReallyLongIdentifierNameThatGoesOnAndOnAndProbablyExceedsEightyColumnsWhenPrintedOut,
	)
}

// example17: Pre-existing string concatenations.
// - Exercises flattening then re-splitting to house style.
// - Preserves spaces at joins across segments.
func example17Concat() {
	log.Errorf(
		"foo " + "bar baz " + "qux quux corge grault garply waldo fred plugh xyzzy thud",
	)
}

// example18: Deeply nested parentheses inside args.
// - Exercises balanced parenthesis scanner to find the call end reliably.
// - Formats only the call layout; leaves inner expressions intact.
func example18Parens() {
	log.Tracef(
		"complex (%s)",
		((1 + (2 * (3 + 4))) * (5 + 6)),
	)
}

// example19: Multiple consecutive spaces inside a string.
//   - Exercises preservation of multiple spaces across wrapping and
//     concatenation joins.
//   - Ensures boundary handling does not collapse or duplicate spaces.
func example19MultiSpaces() {
	log.Infof(
		"This  message   contains    multiple   spaces that should   be preserved  across wrapping and joining when formatted properly with numbers %d and %d",
		1, 2,
	)
}

// example20: Nested call within nested call (two levels).
// - Exercises nested targeted calls and outer placement using head length.
func example20NestedTwice(i int) {
	log.Infof(
		"Nested: %s",
		fmt.Sprintf(
			"L1 %s",
			fmt.Sprintf("L2 number=%d with some trailing words that cause wrapping", i),
		),
	)
}

// example21: Deeper nesting of fmt.Sprintf chains.
// - Exercises multiple nesting levels and placeholder-heavy tails.
func example21DeepNesting() {
	log.Errorf(
		"Deep %s",
		fmt.Sprintf(
			"A %s",
			fmt.Sprintf(
				"B %s",
				fmt.Sprintf("C %d %d %d %d with a description that likely wraps here", 1, 2, 3, 4),
			),
		),
	)
}

// example22: Call inside nested if block.
// - Ensures indentation and continuation work inside nested scopes.
func example22NestedIf(a, b int) {
	if a > 0 {
		if b > 0 {
			log.Warnf(
				"Nested if says values are positive: a=%d and b=%d with a longer comment tail to wrap",
				a, b,
			)
		}
	}
}

// example23: Call inside for loop and if.
// - Ensures detection within control structures.
func example23Loop(threshold int) {
	for i := 0; i < 3; i++ {
		if i >= threshold {
			log.Debugf(
				"Index %d reached threshold; long note that should wrap nicely with identifier: %v",
				i, longVariableNameHere,
			)
		}
	}
}

// example24: Return with long fmt.Errorf.
// - Ensures return + call formatting aligns generically.
func example24ReturnErr(x int) error {
	if x < 0 {
		return fmt.Errorf(
			"cannot proceed because x=%d is below zero and that is not allowed in this context; please choose a different value", x,
		)
	}
	return nil
}

// example25: Deeper code nesting with loops and conditionals.
// - Exercises formatter inside multiple nested blocks.
func example25DeepNesting(vals []int, threshold int) {
	for i := 0; i < len(vals); i++ {
		if vals[i] > 0 {
			for j := 0; j < 2; j++ {
				if j%2 == 0 {
					log.Infof(
						"Deep nesting i=%d j=%d with a message that likely needs wrapping and should remain readable across continuation lines",
						i, j,
					)
				} else {
					log.Warnf(
						"Alternate path i=%d j=%d with another long message to exercise the formatter and ensure continuity",
						i, j,
					)
				}
			}
		}
	}
}

// example26: Tricky concatenations with placeholders split across pieces.
// - Exercises flattening while preserving spaces exactly.
func example26TrickyConcat(user string, action string, when interface{}) {
	log.Debugf(
		"User "+
			"%s "+
			"performed "+
			"%s at "+
			"%v with flags A and B set", user, action, when,
	)
}

// example27: fmt.Sprintf in a return statement returning string.
// - Ensures return + fmt.Sprintf gets formatted like other calls.
func example27ReturnSprintf(a, b int) string {
	return fmt.Sprintf(
		"Computing a result with two integers a=%d and b=%d while adding some longer description to wrap nicely",
		a, b,
	)
}

// example28: fmt.Sprintf returns in conditional branches.
// - Exercises mixed placement of return + call.
func example28BranchReturn(flag bool, name string) string {
	if flag {
		return fmt.Sprintf(
			"Flag true for name=%s with an extended tail that should wrap correctly here as well",
			name,
		)
	}
	return fmt.Sprintf(
		"Flag false for name=%s with a similarly long tail requiring a wrap",
		name,
	)
}

// example29: defer and goroutine with targeted calls.
// - Ensures detection in defer/go contexts.
func example29DeferGo(i int) {
	defer log.Infof(
		"Deferred logging for i=%d with a long enough sentence to need wrapping and still look good",
		i,
	)
	go log.Warnf(
		"Goroutine warning for i=%d including some more words to force wrapping on continuation",
		i,
	)
}

// example30: switch-case with long messages.
// - Exercises switch formatting inside cases.
func example30Switch(v int) {
	switch v {
	case 0:
		log.Tracef(
			"Value is zero which triggers a verbose explanation that continues across multiple lines for clarity",
		)
	case 1:
		log.Tracef(
			"Value one encountered; this message should also wrap correctly under switch-case indentation",
		)
	default:
		log.Errorf(
			"Unexpected value %d requiring attention with a description that certainly should be wrapped",
			v,
		)
	}
}

// example31: No-space very long string with wide runes and emoji
// - Hard-cut only; ensure we don't break mid-UTF-8 and width is visual.
func example31NoSpaceUnicode() {
	log.Infof("測試測試測試測試測試測試測試測試😅😅😅😅😅😅😅😅😅😅測試測試測試測試測試測試測試測試")
}

// example32: Combining marks and zero-width characters
//   - Ensure combining marks don't add width; split at space not combining
//     boundary.
func example32Combining() {
	log.Infof("Café and déjà vu occur in this sentence repeatedly to test combining width handling")
}

// example33: Non-breaking spaces should be preserved and count as width 1.
func example33NBSP() {
	log.Infof("A B C with non breaking spaces and a longer tail to wrap cleanly")
}

// example34: Mixed tabs in indent and inside string.
func example34Tabs() {
	log.Infof("Column	separated	values with tabs and a long trailer that should wrap here nicely")
}

// example35: Raw backtick string that would overflow on the head.
// - Must break before it; no splitting of raw string literal.
func example35RawOverflow() {
	log.Infof(`AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA`)
}

// example36: Nested targeted call deeply, with long head and long tail.
func example36DeepNested() {
	log.Infof("Head %s", fmt.Sprintf("Nested %s", fmt.Sprintf("Inner %s", fmt.Sprintf("Leaf %d %d %d %d and then some text that will wrap", 1, 2, 3, 4))))
}

// example37: Percent verbs near boundary.
func example37Percents(x, y int) {
	log.Infof("Values %%d= %d and %%d= %d placed near the boundary for packing", x, y)
}

// example38: Escaped quotes and backslashes.
func example38Escapes() {
	log.Infof("path=\"C:\\Program Files\\App\" arguments=--flag=\"value with a fairly long description to wrap\"")
}

// example39: Leading comment on next arg and block comment.
func example39Comments() {
	log.Warnf("First arg", // trailing comment stays
		/* block between args */ longVariableNameHere)
}

// example40: Multiple expressions after wrapped text; ensure lookahead break.
func example40Lookahead(a, b, c int) {
	log.Infof("This header will wrap because it is too long to fit on one line with the following expressions:", a, b, c)
}

// example41: Long fmt.Sprintf in return in nested position.
func example41ReturnNested() string {
	return fmt.Sprintf("outer %s", fmt.Sprintf("inner %s", fmt.Sprintf("leaf %d %d %d %d wrapped nicely here", 7, 8, 9, 10)))
}

// example42: Raw string with backticks that contains quotes and percent verbs,
// no split.
func example42RawMixed() {
	log.Infof(`raw "quoted" %d with %s that should not be split across lines even if very long raw backtick literal`)
}

// example43: Mix of wide, emoji, combining and ASCII to stress width
// accounting.
func example43Mix() {
	log.Infof("混合🙂é and A B with long tail to enforce split here and still keep expressions:", 123)
}

// example44: Nested targeted call as first argument content.
func example44NestedFirst() {
	log.Infof(fmt.Sprintf("A very very long head that will wrap inside nested call while still being attached correctly to the outer call: %d %d", 1, 2))
}

// example45: String with many consecutive spaces and tabs mixed.
func example45SpacesTabs() {
	log.Infof("one   two		three    four     five with a long sentence that ensures wrapping")
}
