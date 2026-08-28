package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	veryLongArgument1   = "arg1"
	veryLongArgument2   = "arg2"
	veryLongArgument3   = "arg3"
	veryLongArgument4   = "arg4"
	sourceSlice         = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	destinationSlice    = make([]int, 10)
	ctx                 = context.Background()
	config              = Config{}
	arg1                = "arg1"
	arg2                = "arg2"
	arg3                = "arg3"
	anotherLongArgument = "anotherLongArgument"
	yetAnotherArgument  = "yetAnotherArgument"
	database            = Database{}
	onSuccess           = func() {}
	onFailure           = func() {}
	timeout             = 5 * time.Second
	retries             = 3
	// additional example vars to satisfy references
	client        = Client{}
	db            = DB{}
	complexObject interface{}
	options       interface{}
	formatting    interface{}
	validation    interface{}
	errorHandling interface{}
	requestBody   interface{}
	headers       interface{}
	retryCount    = 1
	maxRetries    = 3
	backoffDelay  = time.Second
)

func main() {
	// Simple function call that should be wrapped
	result := someFunction(veryLongArgument1, veryLongArgument2, veryLongArgument3, veryLongArgument4)
	_ = result

	// Method call on struct
	user := CreateUser("John", "Doe", "john.doe@example.com", "123-456-7890", "Software Engineer")
	_ = user

	// Built-in function with many args
	copy(destinationSlice, sourceSlice)

	// Function with mixed argument types
	ProcessData(ctx, "some long string argument that makes the line exceed limit", 42, true, time.Now(), &config)

	// Nested function calls
	result2 := OuterFunction(InnerFunction(arg1, arg2, arg3), anotherLongArgument, yetAnotherArgument)
	_ = result2

	// Function with slice/map literals
	Configure(map[string]interface{}{"key1": "value1", "key2": "value2", "key3": "value3"}, []string{"item1", "item2", "item3", "item4", "item5", "item6", "item7"})

	// Method chaining that should be wrapped
	database.Query("SELECT * FROM users").Where("active = ?", true).OrderBy("created_at DESC").Limit(100)

	// Function with anonymous function
	ProcessAsync(func() error { return nil }, onSuccess, onFailure, timeout, retries)

	// Short calls that should NOT be wrapped
	fmt.Print("hello")
	log.Info("short")
	x := add(1, 2)
	_ = x

	// Package-qualified function calls
	json.Marshal(complexObject, options, formatting, validation, errorHandling)

	// Constructor-like calls
	server := NewServer(Config{Host: "localhost", Port: 8080, Timeout: 30 * time.Second, MaxConnections: 100})
	_ = server

	// HTTP client calls
	response := client.Post("https://api.example.com/endpoint", "application/json", requestBody, headers, timeout)
	_ = response

	// Error handling calls
	handleError(errors.New("something went wrong"), context.Background(), retryCount, maxRetries, backoffDelay)

	// Database operations
	rows := db.Query("SELECT id, name, email, created_at FROM users WHERE active = ? AND role = ?", true, "admin")
	_ = rows

	// ---- Deep nesting and additional stress cases ----
	for i := 0; i < 3; i++ {
		if i%2 == 0 {
			if i > 1 {
				ProcessData(ctx, "this is a very long string that should be split and followed by arguments neatly", i, true, time.Now(), &config)
			}
		}
	}

	// Nested function calls with composites
	res := Do2(
		Outer2(Inner2("a deeply nested long string that will likely wrap nicely")),
		map[string]interface{}{"k1": "v1", "k2": "v2", "k3": "v3"},
		[]int{1, 2, 3, 4, 5, 6, 7, 8},
	)
	_ = res

	// Multiple composites and a func literal
	Configure2(
		[]string{"alpha", "beta", "gamma", "delta", "epsilon"},
		map[string][]string{"grp": []string{"x", "y", "z"}},
		func() error { return nil },
		time.Second*5,
	)

	// Chained small call should stay inline
	chain2().Then("ok").Limit(100)
}

// Simple nested calls should not be over-expanded.
func simpleNestedCalls(row Row, quote Quote) error {
	maxPrepayRoutingFee := getMaxRoutingFee(
		btcutil.Amount(
			quote.PrepayAmtSat,
		),
	)
	_ = maxPrepayRoutingFee

	err := finalizedHtlcTx.Deserialize(
		bytes.NewReader(
			row.FinalizedHtlcTx,
		),
	)
	return err
}

// A simple notification argument should stay with the call.
func simpleNotifier(notifCtx context.Context) error {
	blockHeightChan, errEpochChan, err := f.cfg.ChainNotifier.
		RegisterBlockEpochNtfn(
			notifCtx,
		)
	_, _ = blockHeightChan, errEpochChan
	return err
}

// Mock returns should pack args.Error(2) when it fits.
func mockReturn(mock Args) (chan *chainntnfs.TxConfirmation, chan error, error) {
	args := m.Called(ctx, txid, pkScript, numConfs, heightHint)

	return args.Get(0).(chan *chainntnfs.TxConfirmation), args.Get(1).(chan error), args.Error(
		2,
	)
}

func someFunction(arg1, arg2, arg3, arg4 string) string {
	return ""
}

func CreateUser(firstName, lastName, email, phone, role string) User {
	return User{}
}

func ProcessData(ctx context.Context, data string, count int, enabled bool, timestamp time.Time, config *Config) {
}

func OuterFunction(inner string, arg2, arg3 string) string {
	return ""
}

func InnerFunction(arg1, arg2, arg3 string) string {
	return ""
}

func Configure(config map[string]interface{}, items []string) {
}

func ProcessAsync(fn func() error, onSuccess, onFailure func(), timeout time.Duration, retries int) {
}

func add(a, b int) int {
	return a + b
}

func handleError(err error, ctx context.Context, retryCount, maxRetries int, backoffDelay time.Duration) {
}

type User struct{}
type Config struct {
	Host           string
	Port           int
	Timeout        time.Duration
	MaxConnections int
}

// Example with comments in arguments
func ExampleWithComments() {
	CallWithComments(
		arg1, // First argument
		arg2, // Second argument
		arg3, // Third argument
	)
}

func CallWithComments(arg1, arg2, arg3 string) {
}

// ---- Stubs to make this golden file compile ----

// json stub to accept variadic arguments like in examples.
var json = struct {
	Marshal func(...interface{}) ([]byte, error)
}{
	Marshal: func(...interface{}) ([]byte, error) { return nil, nil },
}

// Minimal logger with Info method to match usage.
type Logger struct{}

func (Logger) Info(...interface{}) {}

var log Logger

// Database and query chain stubs
type Query struct{}

func (*Query) Where(string, ...interface{}) *Query { return &Query{} }
func (*Query) OrderBy(string) *Query               { return &Query{} }
func (*Query) Limit(int) *Query                    { return &Query{} }

type Database struct{}

func (Database) Query(string) *Query { return &Query{} }

// HTTP client stub
type Response struct{}
type Client struct{}

func (Client) Post(...interface{}) *Response { return &Response{} }

// DB query stub
type Rows struct{}
type DB struct{}

func (DB) Query(string, ...interface{}) *Rows { return &Rows{} }

// Server constructor stub
type Server struct{}

func NewServer(_ Config) *Server { return &Server{} }

// Additional stubs for deep nesting examples
func Do2(...interface{}) interface{}     { return nil }
func Outer2(interface{}) interface{}     { return nil }
func Inner2(string) interface{}          { return nil }
func Configure2(...interface{})          {}
type chainType2 struct{}
func chain2() *chainType2                { return &chainType2{} }
func (*chainType2) Then(string) *chainType2 { return &chainType2{} }
func (*chainType2) Limit(int) *chainType2   { return &chainType2{} }
