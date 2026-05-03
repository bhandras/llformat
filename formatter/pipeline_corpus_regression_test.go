package formatter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineNext_Corpus_DoesNotWrapTRunWhenHeaderAndBodyFit(t *testing.T) {

	const in = `package p

import "testing"

type Terms struct {
	RegistrationTimeout        int
	ApprovalCollectionTimeout  int
	LeaseHoldDuration          int
}

const DefaultLeaseHoldDuration = 1

func (t *Terms) ValidateLeaseHoldDuration() error { return nil }

func TestTerms(t *testing.T) {
	t.Run("sufficient duration accepted", func(t *testing.T) {
		t.Parallel()

		terms := &Terms{
			RegistrationTimeout:        30,
			ApprovalCollectionTimeout:  30,
			LeaseHoldDuration:          DefaultLeaseHoldDuration,
		}
		err := terms.ValidateLeaseHoldDuration()
		if err != nil {
			t.Fatal(err)
		}
	})
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out,
		`t.Run("sufficient duration accepted", func(t *testing.T) {`,
	)
	require.NotContains(t, out, "t.Run(\n")
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksFuncLitArgAfterLongString(t *testing.T) {
	const in = `package p

import "testing"

func TestEncoding(t *testing.T) {
	t.Run("works without registration since encoder only needs type tag", func(
		t *testing.T) {

		t.Parallel()
	})
}
`

	out := formatWithDefaultNext(t, in)

	require.NotContains(
		t, out,
		`t.Run("works without registration since encoder only needs type tag", func(`,
	)
	require.Contains(
		t, out, "t.Run(\n		\"works without "+
			"registration since encoder only needs type "+
			"tag\",\n		func",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_DoesNotIndentConfigLiteralConstructorCall(
	t *testing.T) {

	const in = `package p

type PublisherConfig struct {
	InputSource            string
	MessageStore           string
	Log                    string
	MaxCostRate            int
	IncrementalRetryCost   int
	PreSubmitPreviewAccept bool
}

type Actor struct {
	publisher any
}

func NewRetryPublisher(PublisherConfig) any { return nil }

func f(cfg PublisherConfig) *Actor {
	return &Actor{
		publisher: NewRetryPublisher(PublisherConfig{
			InputSource:                    cfg.InputSource,
			MessageStore:                   cfg.MessageStore,
			Log:                            cfg.Log,
			MaxCostRate:                    cfg.MaxCostRate,
			IncrementalRetryCost:           cfg.IncrementalRetryCost,
			PreSubmitPreviewAccept:        cfg.PreSubmitPreviewAccept,
		}),
	}
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:           80,
		TabStop:               8,
		MaxPipelineIterations: 3,
	})
	outBytes := p.Format([]byte(in))
	requireParseableGo(t, outBytes)
	requireASTEquivalent(t, []byte(in), outBytes)
	out := string(outBytes)

	require.Contains(
		t, out, "publisher: NewRetryPublisher(PublisherConfig{",
	)
	require.NotContains(t, out, "publisher: NewRetryPublisher(\n")
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_DoesNotPackTestifyMessagePastLimit(t *testing.T) {

	const in = `package p

type T struct{}

type Harness struct{}

func (h Harness) Bridge() Bridge { return Bridge{} }

type Bridge struct{}

func (Bridge) PendingMessageCount(string) int { return 1 }

type Client struct{}

func (Client) ClientID() string { return "" }

var require requireT

type requireT struct{}

func (requireT) Equal(*T, any, any, ...any) {}

func f(t *T, h Harness, victim Client) {
	require.Equal(t, 1, h.Bridge().PendingMessageCount(victim.ClientID()),
		"client request should remain buffered")
}
`

	out := formatWithDefaultNext(t, in)

	require.NotContains(
		t, out,
		`h.Bridge().PendingMessageCount(victim.ClientID()), "client request should remain buffered",`,
	)
	require.Contains(
		t, out, "require.Equal(\n		t, "+
			"1,"+
			"\n"+
			"		h.Bridge().PendingMessageCount(vict"+
			"im.ClientID()),\n		\"client request "+
			"should remain buffered\",\n	)",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_DoesNotMergeFunctionReturnPastLimit(t *testing.T) {

	const in = `package p

import (
	queueprotocol "example.com/queueprotocol"
	streamgateway "example.com/streamgateway"
)

type Server struct{}
type QueuedServiceClient struct{}

func (s *Server) buildEventDispatchers(
	edge QueuedServiceClient,
) map[queueprotocol.ServiceMethod]streamgateway.EnvelopeDispatcher {
	return nil
}
`

	out := formatWithDefaultNext(t, in)

	require.NotContains(
		t, out, "edge QueuedServiceClient) "+
			"map[queueprotocol.ServiceMethod]streamgateway.Envel"+
			"opeDispatcher {",
	)
	require.Contains(
		t, out, "func (s *Server) "+
			"buildEventDispatchers(\n	edge "+
			"QueuedServiceClient,\n) "+
			"map[queueprotocol.ServiceMethod]streamgateway.Envel"+
			"opeDispatcher {",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_DoesNotHardSplitShortFormatString(t *testing.T) {

	const in = `package p

import "fmt"

type Event struct {
	Nested Nested
}

type Nested struct {
	Further Further
}

type Further struct {
	ExtraLongBatchIdentifier string
}

func f(i int) Event {
	return Event{
		Nested: Nested{
			Further: Further{
				ExtraLongBatchIdentifier: fmt.Sprintf("batch-for-%d", i),
			},
		},
	}
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit:           80,
		TabStop:               8,
		MaxPipelineIterations: 3,
	})
	out := string(p.Format([]byte(in)))
	requireParseableGo(t, []byte(out))
	requireASTEquivalent(t, []byte(in), []byte(out))

	out2 := string(p.Format([]byte(out)))

	require.NotContains(t, out, `"batch-fo"+`)
	require.Contains(t, out, "\"batch-for-%d\"")
	require.Equal(t, out, out2)
}

func TestPipelineNext_Corpus_ReservesReturnTupleSuffixForTargetCall(
	t *testing.T) {

	const in = `package p

import "fmt"

func f(i int) (string, bool) {
	return fmt.Sprintf("record[%d] expected encoded output missing", i), false
}
`

	out := formatWithDefaultNext(t, in)

	require.NotContains(
		t, out,
		`return fmt.Sprintf("record[%d] expected encoded output missing", i), false`,
	)
	require.Contains(
		t, out, "return fmt.Sprintf(\"record[%d] expected encoded "+
			"output missing\",\n		i), false",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_ReservesCommaAfterPackedStringArg(t *testing.T) {

	const in = `package p

import "context"

type Logger struct{}

func (Logger) WarnS(context.Context, string, ...any) {}

func f(ctx context.Context, log Logger, batchID string) {
	if batchID != "" {
		log.WarnS(ctx, "Update detected for unknown batch", nil,
			"batch_id", batchID)
	}
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "log.WarnS(\n			ctx, \"Update "+
			"detected for unknown batch\", "+
			"nil,\n			\"batch_id\", "+
			"batchID,\n		)",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksCompositeValueAfterKeyOverflow(
	t *testing.T) {

	const in = `package p

import "context"

type MessageRecipient struct{}

type SendItemsRequest struct {
	Recipients []MessageRecipient
	ServiceCost int
	QuotaLimit  int
	RouteLabel  string
	RetryDelay int
}

type ClientAPI struct{}

type Response struct{}

func testMessageRecipient(int) MessageRecipient { return MessageRecipient{} }
func testRouteLabel() string { return "" }

func (ClientAPI) Receive(context.Context, *SendItemsRequest) Response {
	return Response{}
}

func f(client ClientAPI) Response {
	result := client.Receive(
		context.Background(), &SendItemsRequest{
			Recipients: []MessageRecipient{testMessageRecipient(400000)},
			ServiceCost: 1000,
			QuotaLimit:  546,
			RouteLabel:  testRouteLabel(),
			RetryDelay: 144,
		},
	)
	return result
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "Recipients: "+
			"[]MessageRecipient{"+
			"\n"+
			"				testMessageRecipien"+
			"t(400000),\n			},",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_ReservesCommaAfterFinalStringArg(t *testing.T) {
	const in = `package p

type T struct{}

var require requireT

type requireT struct{}

func (requireT) True(*T, bool, ...any) {}

func f(t *T, numMessages, numDL int) {
	require.True(t, numMessages >= 1 || numDL >= 1,
		"message should either be in store for retry or in dead letters")
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	out := string(p.Format([]byte(in)))
	requireParseableGo(t, []byte(out))

	require.Contains(t, out, `"message should either "+`)
	require.Contains(t, out, `"be in store for retry or in dead letters",`)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksLongGenericCallHead(t *testing.T) {
	const in = `package p

type ActorMessage[T any] struct{}
type HandlerResult[A, B, C any] struct{}
type InputEvent struct{}
type OutputEvent struct{}
type RuntimeEnv struct{}

type Worker struct{}

func NewWorkerKey[A, B any](name string) Worker { return Worker{} }

func f() Worker {
	return NewWorkerKey[ActorMessage[InputEvent], HandlerResult[InputEvent, OutputEvent, RuntimeEnv]]("worker")
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(t, out, "NewWorkerKey[\n")
	require.Contains(t, out, "ActorMessage[InputEvent],")
	require.Contains(
		t, out, "HandlerResult[InputEvent, OutputEvent, RuntimeEnv],",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksLastParamBeforeLongFuncReturn(t *testing.T) {
	const in = `package p

type InputEvent struct{}
type VeryLongWireMessage struct{}
type VeryLongHandlerResult struct{}

func makeHandler(method string, newEvent func() InputEvent) func(VeryLongWireMessage) (VeryLongHandlerResult, error) {
	return nil
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(t, out, "newEvent func() InputEvent,\n)")
	require.Contains(
		t, out,
		") func(VeryLongWireMessage) (VeryLongHandlerResult, error) {",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_ReservesCommaInSmallReturnList(t *testing.T) {
	const in = `package p

type Processor struct{}
type VeryLongHashValue struct{}
type VeryLongPreimageToken struct{}
type ProcessedItemInfo struct{}

func (p *Processor) observeItem(ctx context.Context, requestFingerprint external.VeryLongHashValue, payloadScript []byte) (*external.VeryLongPreimageToken, *ProcessedItemInfo, error) {
	return nil, nil, nil
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "requestFingerprint external.VeryLongHashValue, "+
			"payloadScript []byte) "+
			"(\n	*external.VeryLongPreimageToken, "+
			"*ProcessedItemInfo, error) {",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_PreservesInlineNolintHeaderComment(t *testing.T) {

	const in = `package p

type Router struct{}
type HandlerKey[A, B any] struct{}
type Request struct{}
type Response struct{}

func RegisterHandlers( //nolint:funlen
	router *Router,
	handlerKey HandlerKey[Request, Response]) {
}
`

	p := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})
	outBytes := p.Format([]byte(in))
	requireParseableGo(t, outBytes)
	requireASTEquivalent(t, []byte(in), outBytes)
	out := string(outBytes)

	require.Contains(t, out, "func RegisterHandlers( //nolint:funlen")
	require.NotContains(t, out, ",,,,,")
	require.Equal(t, out, string(p.Format(outBytes)))
}

func TestPipelineNext_Corpus_BreaksStringConcatArgInNestedCall(t *testing.T) {
	const in = `package p

import "errors"

type Task struct{}

func (Task) MessageType() string { return "" }

func wrap(error) error { return nil }

func f(task Task) error {
	return wrap(
		errors.New(
			"operation could not be delivered: " + task.MessageType()),
	)
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "\"operation could not be delivered: \" "+
			"+\n			task.MessageType()",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksFuncLitParamBeforeReturn(t *testing.T) {
	const in = `package p

type InputEventWithExtraLongName struct{}
type HandlerResultWithExtraLongName struct{}

func NewMappedRef(any, func(InputEventWithExtraLongName) HandlerResultWithExtraLongName) any {
	return nil
}

func f(ref any) any {
	return NewMappedRef(
		ref,
		func(evt InputEventWithExtraLongName) HandlerResultWithExtraLongName {
			return HandlerResultWithExtraLongName{}
		},
	)
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "func(\n			evt "+
			"InputEventWithExtraLongName,\n		) "+
			"HandlerResultWithExtraLongName {",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_PreservesMultilineGenericParamTypeArgs(
	t *testing.T) {

	const in = `package p

type Service struct{}
type Ref[A, B any] struct{}
type TellOnlyRef[A any] struct{}
type InputMessage struct{}
type OutputMessage struct{}
type WorkMessage struct{}
type WorkResult struct{}
type TimeoutMessage struct{}
type ManagerMessage struct{}
type Worker struct{}

func (s *Service) start(ctx context.Context,
	sourceRef Ref[
		InputMessage, OutputMessage,
	],
	workerRef Ref[
		WorkMessage, WorkResult,
	],
	timeoutRef TellOnlyRef[TimeoutMessage],
	manager TellOnlyRef[ManagerMessage]) (
	*Worker, error) {

	return nil, nil
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "sourceRef Ref[\n		InputMessage, "+
			"OutputMessage,\n	],\n	workerRef Ref[",
	)
	require.NotContains(t, out, "OutputMessage], workerRef")
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksPartiallyMultilineGenericParamType(
	t *testing.T) {

	const in = `package p

type Service[A, B, C any] struct{}
type State[A, B, C any] struct{}
type InputEventWithLongName struct{}
type OutputEventWithLongName struct{}
type Env struct{}

func (s *Service[InputEventWithLongName, OutputEventWithLongName, Env]) apply(
	ctx context.Context, currentState State[InputEventWithLongName, OutputEventWithLongName,
		Env], event InputEventWithLongName) (State[InputEventWithLongName, OutputEventWithLongName, Env], []OutputEventWithLongName, error) {

	panic("not implemented")
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "currentState "+
			"State["+
			"\n"+
			"		InputEventWithLongName,"+
			"\n"+
			"		OutputEventWithLongName,"+
			"\n		Env,\n	]",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksDeepCallArgPairWhenPairOverflows(
	t *testing.T) {

	const in = `package p

func example(items []Item, privKey, operatorKey Key) {
	for i := range items {
		items[i] = Item{
			Value: 1,
			Build: func() []byte {
				result, err := module.
					EncodeStandardRecordTemplate(
						privKey.PubKey(),
						operatorKey.PubKey(), 144,
					)
				_ = err
				return result
			}(),
		}
	}
}

type Item struct { Value int; Build []byte }
type Key struct{}
func (Key) PubKey() any { return nil }
var module M
type M struct{}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "privKey.PubKey(),"+
			"\n"+
			"						ope"+
			"ratorKey.PubKey(), 144,",
	)
	require.NotContains(t, out, "privKey.PubKey(), operatorKey.PubKey()")
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksGenericCallHeadWithAssignPrefix(
	t *testing.T) {

	const in = `package p

func RegisterEvents() Subscriber[InternalEvent, OutboxEvent, Env] {
	subscriber := fn.NewEventReceiver[State[InternalEvent, OutboxEvent, Env]](10)
	return subscriber
}

type Subscriber[A, B, C any] *Receiver[State[A, B, C]]
type State[A, B, C any] struct{}
type InternalEvent struct{}
type OutboxEvent struct{}
type Env struct{}
type receiverFactory struct{}
var fn receiverFactory
type Receiver[T any] struct{}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "subscriber := "+
			"fn.NewEventReceiver[State["+
			"\n"+
			"		InternalEvent,"+
			"\n		OutboxEvent,\n		Env,\n	]](",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_BreaksGenericCompositeCallArgType(t *testing.T) {

	const in = `package p

func handle(err error) Result {
	if err != nil {
		for i := 0; i < 1; i++ {
			if i == 0 {
				return NewResult(
					ActorResponse[InternalEvent, OutboxEvent, Env]{},
					err,
				)
			}
		}
	}
	return Result{}
}

type Result struct{}
type ActorResponse[A, B, C any] struct{}
type InternalEvent struct{}
type OutboxEvent struct{}
type Env struct{}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "ActorResponse["+
			"\n"+
			"						Int"+
			"ernalEvent,"+
			"\n"+
			"						Out"+
			"boxEvent,"+
			"\n"+
			"						Env"+
			",\n					]{},",
	)
	requireNoLineLongerThan(t, out, 80)
}

func TestPipelineNext_Corpus_ReservesReturnSuffixForSplitFormatString(
	t *testing.T) {

	const in = `package p

import "fmt"

func validate(expectedRecordSummaries []int, actualRecordSummaries []int) (
	string, bool) {

	if len(expectedRecordSummaries) != len(actualRecordSummaries) {
		return fmt.Sprintf("quote entries %d != expected "+
			"items %d", len(expectedRecordSummaries), len(actualRecordSummaries)), false
	}
	return "", true
}
`

	out := formatWithDefaultNext(t, in)

	require.Contains(
		t, out, "return fmt.Sprintf(\"quote entries %d != expected "+
			"\"+\n			\"items %d\", "+
			"len(expectedRecordSummaries),"+
			"\n"+
			"			len(actualRecordSummaries))"+
			", false",
	)
	requireNoLineLongerThan(t, out, 80)
}

func formatWithDefaultNext(t *testing.T, in string) string {
	t.Helper()

	p := NewPipeline(PipelineConfig{
		ColumnLimit: 80,
		TabStop:     8,
	})

	out := p.Format([]byte(in))
	requireParseableGo(t, out)
	requireASTEquivalent(t, []byte(in), out)

	return string(out)
}

func requireNoLineLongerThan(t *testing.T, src string, limit int) {
	t.Helper()

	for idx, line := range strings.Split(src, "\n") {
		if line == "" {
			continue
		}
		if got := NewBaseConfig(limit, 8).Width(line); got > limit {
			t.Fatalf(
				"line %d is %d columns, want <= %d:\n%s", idx+1,
				got, limit, line,
			)
		}
	}
}
