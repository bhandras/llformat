// This is a very long line comment that should be wrapped by the comment
// formatter into multiple lines while preserving the indentation and markers.
// LLFORMAT_NEXT_GOLDEN_TODO: update this file by hand to match the --next pipeline output.
package main

// Another paragraph with multiple spaces and tabs that should be collapsed into
// single spaces when reflowed by the greedy algorithm.

func main() {}

package example

// - This is a list item that is quite long and should therefore be wrapped to
//   the next line with proper indentation under the dash marker.
// - Second item with tabs and multiple spaces which should be normalized when
//   wrapping takes place.
//
// A non-list paragraph should not get list indentation.

package sample

/*
 * This is a block comment that should be wrapped greedily while preserving the
 * interior star prefixes and empty lines.
 *
 * - A list item inside the block comment which is long enough to wrap to the
 *   next line and should align under the dash.
 * - Another item with extra spaces that should be collapsed when wrapping
 *   occurs.
 */

// Code below should remain unchanged: var x = 1 // trailing comment
var x = 1 // trailing comment

package tricky

// This line starts with a tab before the comment marker when rendered, and
// includes a tab character within the text. It should wrap appropriately
// considering tab stops.
//
// Emoji and wide chars: 😀😀😀 keep width accounting;
// 日本語のテキストも含めます to ensure we don't split runes or miscount visual
// width.

package deepnest

func demo() {
	// Indented standalone comment with multiple words that will be wrapped
	// at the limit and we also want to see list handling below.
	// - First item is very long and should wrap to the next line and
	//   continue properly with the correct indentation after the dash
	//   marker.
	// - Second item with tabs and wide chars 😀😀 should also behave
	//   reasonably even with visual width considerations.
	if true {
		/*
		 * Block comment inside an if, with lines that are long enough
		 * to wrap and include a list.
		 *
		 * - This item is inside a block comment and will need to wrap
		 *   to the next line with alignment.
		 * - Another item that demonstrates tabs and extra spacing which
		 *   should be normalized.
		*/
		val := "keep" // This trailing comment is extremely long and should remain exactly as-is without being wrapped by the formatter even though it exceeds the configured width significantly in practice.
		keep := 1 /* inline block comment that must remain unchanged even if it's very very very very long to test behavior */
}

package deepnest2

// CommentFormatter reflows standalone comment blocks greedily.
//
// Rules (summary):
//   - Only format pure comment lines: lines that begin with "//" after optional
//     indentation, or standalone block comments that begin with "/*" on their
//     own line and end with "*/" on their own line. Trailing comments after
//     code are left intact.
func levels() {
	if true {
		for i := 0; i < 1; i++ {
			switch i {
			case 0:
				// This is a very deeply nested comment that
				// should still be wrapped correctly even at
				// this indentation level and it should preserve
				// the structure of the text.
				// - Deep list item that needs to wrap and align
				//   properly under the dash marker as
				//   continuation lines are emitted.
				doSomething() // this trailing inline comment at deep nesting level should be moved above when inline wrapping is enabled and it is long enough to need wrapping in that mode
			}
		}
	}
}

	// A lone list section follows
	// - Single item that is so long it should definitely be wrapped across
	//   multiple lines to check continuation indenting consistency across
	//   wraps and spacing.
}
