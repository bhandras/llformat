package formatter

import (
	"strings"
	"testing"

	llwidth "github.com/bhandras/llformat/width"
	"github.com/stretchr/testify/require"
)

func TestBuildSplitQuoted_DoesNotEmitDanglingPlusWhenIndentTooDeep(
	t *testing.T) {

	// When the indentation itself exceeds the configured width, splitting a
	// string literal cannot make it fit. We should still emit valid Go (no
	// dangling '+' followed by a comma/newline in the caller).
	out := buildSplitQuoted("%s", 200, "\t\t", 48)
	trimmed := strings.TrimRight(out, " \t")
	require.NotEmpty(t, trimmed)
	require.NotEqual(
		t, '+', trimmed[len(trimmed)-1],
		"split output must not end with '+'",
	)
	require.NotContains(
		t, out, "+\n			,",
		"must not end with a dangling '+', then comma on the next line",
	)
}

func TestBuildSplitQuoted_DoesNotShardWordsWhenIndentLeavesNoUsefulRoom(
	t *testing.T) {

	out := buildSplitQuotedForCallArg(
		"batch %s in expiry index but not in batches map", 72,
		"							"+
			"	", 80, true,
	)

	require.Equal(
		t, `"batch %s in expiry index but not in batches map"`, out,
	)
	require.NotContains(t, out, `"exp"+`)
	require.NotContains(t, out, `"iry"+`)
}

func TestBuildSplitQuoted_UsesGofmtCompatibleSpacingAroundPlus(t *testing.T) {
	t.Run(
		"space_split",
		func(t *testing.T) {
			out := buildSplitQuoted(
				"this is a very long string that should "+
					"split on spaces", 0, "	", 36,
			)
			require.Contains(t, out, "\" +\n\t\t\"")
			require.NotContains(
				t, out, "\"+\n", "must not emit a quote "+
					"immediately followed by '+' and "+
					"newline",
			)
		},
	)

	t.Run(
		"hard_split_no_spaces",
		func(t *testing.T) {
			out := buildSplitQuoted(
				"this_is_a_very_long_string_with_no_spaces_s"+
					"o_it_must_hard_split", 0, "	", 36,
			)
			require.Contains(t, out, "\" +\n\t\t\"")
			require.NotContains(
				t, out, "\"+\n", "must not emit a quote "+
					"immediately followed by '+' and "+
					"newline",
			)
		},
	)
}

func TestBuildSplitQuotedForCallArg_UsesGofmtCompatibleSpacingWhenTrailingArgs(
	t *testing.T) {

	// When a split string literal is a call argument and there are
	// additional args after it, gofmt may elide the space before '+' when
	// the segment ends with a space. We mirror that behavior for
	// idempotence and to preserve one extra column of budget.
	out := buildSplitQuotedForCallArg(
		"unable to lookup peer alias: %v with a tail that forces "+
			"wrapping", 0, "	", 36, true,
	)
	require.Contains(
		t, out, "\"+\n		\"", "expected at least one join "+
			"rendered as `\"+\\n` when the segment ends with a "+
			"space",
	)
}

func TestFormatCallPackedMultiLine_DoesNotOverBreakAfterSplitStringUnderDeepIndent(
	t *testing.T) {

	// Regression test: when a string arg is split across multiple lines,
	// the packed multiline call formatter must compute the "current column"
	// after the split based on the split string's last line (which already
	// includes its own indentation), otherwise we can break subsequent args
	// unnecessarily under deeper indentation.
	call := []byte(
		`ProcessData(ctx, "some long string argument that makes the line exceed the configured limit", 42, true)`,
	)
	out := FormatCallPackedMultiLine(call, "\t\t", 80, 8)

	// When the following args fit, keep them on the same line as the final
	// string segment.
	require.Contains(t, out, `", 42, true,`)
	require.NotContains(
		t, out, "\",\n			42",
		"must not break before 42 when it fits after the split string",
	)
}

func TestFormatCallPackedMultiLineNext_DoesNotOverflowWhenBreakingAddsComma(
	t *testing.T) {

	// Regression test: packed multiline can decide to break *after* packing
	// multiple args onto a continuation line. When that happens, it adds a
	// comma at the end of the current line (`,\n`). We must not allow
	// packing to consume the full width budget and then overflow by 1 when
	// emitting that comma.
	call := []byte(`someFunc(aaaaa, bbbbb, c)`)
	out := FormatCallPackedMultiLineNext(call, "", 20, 8)

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		require.LessOrEqual(
			t, llwidth.VisualLenWithTab(line, 8), 20, "line "+
				"exceeds configured column limit: %q", line,
		)
	}
}

func TestFormatCallPackedMultiLineNext_BreaksBeforeMultilineCallArg(
	t *testing.T) {

	call := []byte(
		`outerCall(firstArgumentExtraXX, secondArgumentExtraYY, innerCallWithExtraLongName[
	TypeOne,
	TypeTwo,
](), fourthArgumentName)`,
	)
	out := FormatCallPackedMultiLineNext(call, "\t", 80, 8)

	require.NotContains(
		t, out, "secondArgumentExtraYY, innerCallWithExtraLongName[",
	)

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		require.LessOrEqual(
			t, llwidth.VisualLenWithTab(line, 8), 80, "line "+
				"exceeds configured column limit: %q", line,
		)
	}
}

func TestFormatCallPackedMultiLineNext_BreaksKeyedCompositeValueAfterAlign(
	t *testing.T) {

	call := []byte(
		`client.Receive(
	context.Background(), &SendItemsRequest{
		Recipients: []MessageRecipient{testMessageRecipient(400000)},
		ServiceFee: 1000,
		DustLimit: 546,
		RoutingKey: testRoutingKey(),
		RetryDelay: 144,
	},
)`,
	)

	out := FormatCallPackedMultiLineNext(call, "\t", 80, 8)
	require.Contains(
		t, out, "Recipients: "+
			"[]MessageRecipient{"+
			"\n"+
			"				testMessageRecipien"+
			"t(400000),\n			},",
	)
}

func TestFormatCallPackedMultiLineNext_BreaksOverflowingBinaryArg(
	t *testing.T) {

	call := []byte(`uint64(senderID*msgsPerSender + j)`)

	out := FormatCallPackedMultiLineNext(call, "\t\t\t\t\t\t\t", 80, 8)
	require.Contains(
		t, out, "senderID "+
			"*"+
			"\n"+
			"						"+
			"		msgsPerSender "+
			"+"+
			"\n"+
			"						"+
			"		j,",
	)
	for _, line := range strings.Split(out, "\n") {
		require.LessOrEqual(t, visualLen(line), 80, line)
	}
}

func TestFormatCallPackedMultiLineNext_BreaksBeforeCallArgWhenPairOverflows(
	t *testing.T) {

	call := []byte(
		`EncodeStandardMessageTemplate(clientKey.PubKey(), serviceKey.PubKey(), 144)`,
	)

	out := FormatCallPackedMultiLineNext(call, "\t\t\t\t\t", 80, 8)
	require.Contains(
		t, out, "clientKey.PubKey(),"+
			"\n"+
			"						ser"+
			"viceKey.PubKey(),",
	)
	for _, line := range strings.Split(out, "\n") {
		require.LessOrEqual(t, visualLen(line), 80, line)
	}
}

func TestFormatCallPackedMultiLineNext_BreaksGenericCalleeTypeArgs(
	t *testing.T) {

	call := []byte(
		`actor.NewServiceKey[ActorMessage[InternalEvent], ActorResponse[InternalEvent, OutboxEvent, Env]]("name")`,
	)

	out := FormatCallPackedMultiLineNext(call, "\t", 80, 8)
	require.Contains(
		t, out, "actor.NewServiceKey["+
			"\n"+
			"		ActorMessage[InternalEvent],"+
			"\n		ActorResponse[InternalEvent, "+
			"OutboxEvent, Env],\n	](\n",
	)
	for _, line := range strings.Split(out, "\n") {
		require.LessOrEqual(t, visualLen(line), 80, line)
	}
}

func TestFormatCallPackedMultiLineNext_BreaksNestedGenericCalleeTypeArgs(
	t *testing.T) {

	call := []byte(
		`receiver.NewSubscriberWithLongName[State[InputEvent, OutputEvent, RuntimeEnv]]("name")`,
	)

	out := FormatCallPackedMultiLineNext(call, "\t", 80, 8)
	require.Contains(
		t, out, "receiver.NewSubscriberWithLongName[State["+
			"\n"+
			"		InputEvent,"+
			"\n"+
			"		OutputEvent,"+
			"\n		RuntimeEnv,\n	]](\n",
	)
	for _, line := range strings.Split(out, "\n") {
		require.LessOrEqual(t, visualLen(line), 80, line)
	}
}

func TestFormatCallGreedyNext_DoesNotOverflowWhenBreakingAfterExactFitArgAddsComma(
	t *testing.T) {

	const colLimit = 30
	const tabStop = 8

	// This call is crafted so that the first non-string argument would
	// "fit" exactly at the column limit under AllowExactFit, but then we
	// immediately need to break before the next arg. That break appends a
	// trailing comma to the current line (`,\n`), so we must leave room for
	// it.
	//
	// Prior to reserving 1 column for the potential trailing comma on
	// expression args, this could produce a line of length colLimit+1.
	call := []byte(`f("123456789012", abcdefghijkl, b)`)
	out := FormatCallGreedyNext(call, "", 0, colLimit, tabStop)

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		require.LessOrEqual(
			t, llwidth.VisualLenWithTab(line, tabStop), colLimit,
			"line exceeds configured column limit: %q", line,
		)
	}
}

func TestFormatCallPackedMultiLine_DoesNotReflowNestedLenCallWhenItFitsOnItsOwnLine(
	t *testing.T) {

	// Regression test: nested calls like len(...) should not be recursively
	// reformatted into packed multiline when they would fit on their own
	// continuation line. This avoids ugly results like: len( x, )
	call := []byte(`make([][]byte, 0, len(chanBackupsProtos.ChanBackups))`)
	out := FormatCallPackedMultiLineNext(call, "\t\t", 60, 8)
	require.Contains(t, out, "len(chanBackupsProtos.ChanBackups)")
	require.NotContains(
		t, out, "len(\n", "must not expand len(...) into a nested "+
			"multiline call when it fits as-is",
	)
}

func TestFormatCallPackedMultiLine_DoesNotReflowNestedSlogAttrWhenItFits(
	t *testing.T) {

	call := []byte(
		`log.Info("event", slog.Int("tx_count", len(signers)), ` +
			`slog.Int("cosigner_count", len(index)))`,
	)

	out := FormatCallPackedMultiLineNext(call, "\t", 80, 8)

	require.Contains(t, out, `slog.Int("tx_count", len(signers)),`)
	require.Contains(t, out, `slog.Int("cosigner_count", len(index)),`)
	require.NotContains(t, out, "slog.Int(\n")
	requireNoLineLongerThan(t, out, 80)
}

func TestFormatCallGreedy_DoesNotSplitShortFormatStringToFitCommaSpace(
	t *testing.T) {

	// Regression test for "early break" in deeply indented return
	// statements: when the whole call cannot fit on the current line, but
	// the short argument list fits on a continuation line, prefer a
	// multiline call over splitting the string into `"..." +\n"...", err`.
	call := []byte(`fmt.Errorf("error parsing psbt: %w", err)`)

	// Choose a baseLen such that the quoted format string ends at column
	// 79, leaving room for a comma but not for ", " (comma+space).
	//
	// In formatCallGreedy: curLen starts at baseLen + len("fmt.Errorf") +
	// 1. The quoted string literal here is 24 columns wide.
	//
	// len("fmt.Errorf") is 10, so curLen = baseLen + 11. We want curLen +
	// 24 == 79 => curLen == 55 => baseLen == 44.
	out := FormatCallGreedy(call, "\t\t\t\t\t", 44, 80, 8)

	require.Contains(t, out, "fmt.Errorf(\n")
	require.Contains(t, out, "\n					"+
		"	\"error parsing psbt: %w\", err,")
	require.NotContains(
		t, out, "\" +\n", "must not split a short format string "+
			"just to make room for a following space",
	)
}

func TestFormatCallGreedyNext_JoinAwareSplit_PreservesSpaceBeforePlus(
	t *testing.T) {

	// Regression test for "peer alias" getting collapsed to "peeralias" due
	// to overly conservative width accounting around `+` when there are
	// trailing call args.
	//
	// With a tight column limit, we want the first split to be able to end
	// at a space boundary and still fit the join when gofmt would render
	// `"..." +` as `"..." +` or `"..." +` vs `"..." +` (context-sensitive).
	call := []byte(
		`fmt.Errorf("unable to lookup peer alias more: %v", err)`,
	)
	out := FormatCallGreedyNext(call, "", 0, 36, 8)

	// Ensure the split preserves the space boundary before `+`, allowing
	// the next segment to start with "alias..." rather than joining words.
	require.Contains(
		t, out, "\"unable to lookup peer \"+\n	\"alias",
		"expected join-aware split at the `peer ` boundary",
	)
	require.NotContains(
		t, out, "\"peer\"+\n	\"alias",
		"must not drop the trailing space at the split point",
	)
}

func TestFormatCallGreedyNext_BreaksOverflowingBinaryStringArg(t *testing.T) {
	call := []byte(
		`errors.New("operation could not be delivered: " + task.MessageType())`,
	)

	out := FormatCallGreedyNext(call, "\t\t\t\t", 40, 80, 8)

	require.Contains(
		t, out, "\"operation could not be delivered: \" "+
			"+"+
			"\n"+
			"					task.Messag"+
			"eType()",
	)
	for _, line := range strings.Split(out, "\n") {
		require.LessOrEqual(t, visualLen(line), 80, line)
	}
}

func TestFormatCallPackedMultiLineNext_DoesNotReflowFuncLitBodyCalls(
	t *testing.T) {

	call := []byte(
		"openServer(handler.Wrap(func(w Writer, r Request) {\n	_, " +
			"err := " +
			"w.Write([]byte(" +
			"\n		`{\"message\":\"operation " +
			"failed\",` +\n			`\"details\":{` " +
			"+" +
			"\n" +
			"			`\"code\":\"not-ready\"}}}`," +
			"\n	))\n	check(err)\n}))",
	)

	out := FormatCallPackedMultiLineNext(call, "\t", 80, 8)

	require.Contains(
		t, out,
		"handler.Wrap(func(w Writer, r Request) {",
	)
	require.NotContains(t, out, "handler.Wrap(\n")
	for _, line := range strings.Split(out, "\n") {
		require.LessOrEqual(t, visualLen(line), 80, line)
	}
}

func TestFormatCallPackedMultiLineNext_BreaksOverflowingSelectorCallArg(
	t *testing.T) {

	call := []byte(`Attr("key", resource.Handle.KeyString())`)

	out := FormatCallPackedMultiLineNext(call, "\t\t\t\t\t\t\t", 80, 8)

	require.Contains(
		t, out, "resource.Handle.\n"+
			strings.Repeat("\t", 8)+"KeyString()",
	)
	requireNoLineLongerThan(t, out, 80)
}
