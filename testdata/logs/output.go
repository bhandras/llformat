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

// example1: Long error format string followed by one expression.
//   - Exercises splitting a single long text arg and keeping the following expr
//     on the same line when possible.
func example1() error {
	return fmt.Errorf("this is a long error message with a couple (%d) "+
		"place holders", len(things))
}

// example2: Input already broken across lines.
//   - Exercises reflow to the formatter's preferred layout: head on one line,
//     continuation indent with a tab, and packing next expr if it fits.
func example2() error {
	for i := 0; i < 10; i++ {
		log.Debugf("Something happened here that we need to log: %v",
			longVariableNameHere)
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
		log.Infof("This is a long message that we expect to break "+
			"into multiple lines User %s performed action %s at "+
			"%v with result %v.", userName, actionName, timestamp,
			result)
	}
}

// example4: Simple fmt.Printf without wrapping pressure.
// - Should stay a single line after formatting.
func example4(someValue int) {
	fmt.Printf("This is a formatted string with value: %d", someValue)
}

// example5: Long single literal without convenient spaces.
// - Exercises forced cut with space preservation across "+" joins.
// - Ensures the "+" placement never exceeds the 80-column limit.
func example5() {
	log.Infof("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAA")
}

// example6: Many short expressions after a long message.
// - Exercises reservation for trailing exprs to keep several on the same line.
// - Packs as many expressions as fit without exceeding 80 columns.
func example6(userName string, a, b, c, d, e int) {
	log.Infof("User %s did things with a bunch of values:", userName, a, b,
		c, d, e)
}

// example7: Tabs in leading indent.
// - Exercises continuation indent as wsIndent + "\t".
// - Visual width treats tabs as 8-column stops.
func example7TabbedIndent() {
	log.Infof("This message starts under a double-tab indent and should "+
		"wrap nicely with a tab continuation.", 42)
}

// example8: Non-whitespace prefix before call (return + call).
// - Exercises continuation alignment to leading whitespace, not after "return".
func example8ReturnPrefix(x int) int {
	if x > 0 {
		// Using fmt.Sprintf to keep this an expression for formatting
		// purposes.
		return len(fmt.Sprintf("Returning something with a long "+
			"descriptive message that probably wraps nicely %d "+
			"%d %d", x, x+1, x+2))
	}
	return 0
}

// example9: Nested call as argument.
// - Exercises rewriting only the outer targeted call.
// - Inner call argument is printed as-is.
func example9Nested(i int) {
	log.Infof("Outer %s", fmt.Sprintf("Inner number: %d with some "+
		"trailing words that may wrap", i))
}

// example10: Raw string literal input.
// - Exercises normalization to a quoted string and wrapping.
// - Verifies escaping of backslashes and quotes.
func example10Raw() {
	log.Infof(`path\\to\\file "quoted" with spaces and maybe enough length to require wrapping at some point`)
}

// example11: Comments inside argument list.
// - Documents that comments inside call args are dropped by AST printing.
// - Golden should reflect comment loss.
func example11Comments(a, b int) {
	log.Warnf("Message with args", // trailing comment after first arg
		a /* inline between args */, b)
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
	log.Infof("Another message with a value %d that may require wrapping "+
		"due to length", n)
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
	log.Infof("測試 with 😅 emoji and wide chars mixed in the sentence " +
		"to approach the column limit")
}

// example16: Very long expression argument.
//   - Exercises keeping a long expr intact even if it exceeds width on a fresh
//     line.
func example16LongExpr() {
	log.Debugf("Message with a very long identifier next",
		thisIsAReallyLongIdentifierNameThatGoesOnAndOnAndProbablyExceedsEightyColumnsWhenPrintedOut)
}

// example17: Pre-existing string concatenations.
// - Exercises flattening then re-splitting to house style.
// - Preserves spaces at joins across segments.
func example17Concat() {
	log.Errorf("foo bar baz qux quux corge grault garply waldo fred " +
		"plugh xyzzy thud")
}

// example18: Deeply nested parentheses inside args.
// - Exercises balanced parenthesis scanner to find the call end reliably.
// - Formats only the call layout; leaves inner expressions intact.
func example18Parens() {
	log.Tracef("complex (%s)", ((1 + (2 * (3 + 4))) * (5 + 6)))
}

// example19: Multiple consecutive spaces inside a string.
//   - Exercises preservation of multiple spaces across wrapping and
//     concatenation joins.
//   - Ensures boundary handling does not collapse or duplicate spaces.
func example19MultiSpaces() {
	log.Infof("This  message   contains    multiple   spaces that should "+
		"  be preserved  across wrapping and joining when formatted "+
		"properly with numbers %d and %d", 1, 2)
}

// example20: Nested call within nested call (two levels).
// - Exercises nested targeted calls and outer placement using head length.
func example20NestedTwice(i int) {
	log.Infof("Nested: %s", fmt.Sprintf("L1 %s", fmt.Sprintf("L2 "+
		"number=%d with some trailing words that cause wrapping", i)))
}

// example21: Deeper nesting of fmt.Sprintf chains.
// - Exercises multiple nesting levels and placeholder-heavy tails.
func example21DeepNesting() {
	log.Errorf("Deep %s", fmt.Sprintf("A %s", fmt.Sprintf("B %s",
		fmt.Sprintf("C %d %d %d %d with a description that likely "+
			"wraps here", 1, 2, 3, 4))))
}

// example22: Call inside nested if block.
// - Ensures indentation and continuation work inside nested scopes.
func example22NestedIf(a, b int) {
	if a > 0 {
		if b > 0 {
			log.Warnf("Nested if says values are positive: a=%d "+
				"and b=%d with a longer comment tail to wrap",
				a, b)
		}
	}
}

// example23: Call inside for loop and if.
// - Ensures detection within control structures.
func example23Loop(threshold int) {
	for i := 0; i < 3; i++ {
		if i >= threshold {
			log.Debugf("Index %d reached threshold; long note "+
				"that should wrap nicely with identifier: %v",
				i, longVariableNameHere)
		}
	}
}

// example24: Return with long fmt.Errorf.
// - Ensures return + call formatting aligns generically.
func example24ReturnErr(x int) error {
	if x < 0 {
		return fmt.Errorf("cannot proceed because x=%d is below zero "+
			"and that is not allowed in this context; please "+
			"choose a different value", x)
	}
	return nil
}

// Provide an entry point so this single file builds as an executable.
func main() {}
