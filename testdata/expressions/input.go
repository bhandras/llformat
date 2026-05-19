package main

import (
	"context"
	"time"
)

// Example 1: Long if condition with && and ||
func example1(userIsAuthenticated, userHasPermission, accountIsLocked, sessionIsValid, isAdminOverride bool) {
	if userIsAuthenticated && userHasPermission && !accountIsLocked && sessionIsValid || isAdminOverride {
		doSomething()
	}
}

// Example 2: Long assignment with binary operators
func example2(user, data, sig interface{}, bypassEnabled bool) bool {
	isValid := checkPermission(user) && validateInput(data) && verifySignature(sig) || bypassEnabled
	return isValid
}

// Example 3: Method chain
func example3() {
	result := client.WithTimeout(30*time.Second).WithRetry(3).WithHeaders(headers).Execute(ctx, request)
	_ = result
}

// Example 4: Long case statement
func example4(val int) {
	switch val {
	case TypeA, TypeB, TypeC, TypeD, TypeE, TypeF, TypeG, TypeH, TypeI, TypeJ, TypeK:
		doSomething()
	}
}

// Example 5: Long for condition
func example5(items []int, stopRequested bool, ctx context.Context, retryCount, maxRetries int) {
	for i := 0; i < len(items) && !stopRequested && ctx.Err() == nil && retryCount < maxRetries; i++ {
		process(items[i])
	}
}

// Example 6: Complex boolean in return
func example6(a, b, c, d, e, f bool) bool {
	return a && b && c || d && e && f || (a && d) || (b && e) || (c && f)
}

// Example 7: Nested binary expressions
func example7(x, y, z, w int) bool {
	return x > 0 && y > 0 && z > 0 && w > 0 && x < 100 && y < 100 && z < 100 && w < 100
}

// Example 8: Arithmetic expression
func example8(a, b, c, d, e, f, g, h int) int {
	result := a + b + c + d + e + f + g + h + someVeryLongFunctionName(a, b) + anotherLongFunction(c, d)
	return result
}

// Example 9: String concatenation (already handled by other formatter, should be untouched)
func example9() string {
	return "This is a very long string that " + "spans multiple parts " + "and is concatenated together"
}

// Example 10: Mixed operators with comparison
func example10(val, min, max int, enabled, active bool) bool {
	return val >= min && val <= max && enabled && active && someCheck(val) && anotherCheck(val)
}

// Example 11: Assignment with method call and comparison operators
func example11(f *formatter, trimmedLongVariable string, inSwitch, inInterface int) {
	lineType := f.classifyLine(trimmedLongVariable, inSwitch > 0, inInterface > 0)
	_ = lineType
}

// example 12: Deeply nested if statements with a nested function call
func example12(alpha, beta, gamma int) {
	if alpha > 0 {
		if beta > 0 {
			if gamma > 0 {
				if len(fmt.Sprintf("%d%d%d", alpha, beta, gamma)) > 10 {
					doSomething()
				}
			}
		}
	}
}

func example13() {
	superVeryLongArgumentSoWeSplit := "This is a long string parameter "
	veryveryLongArgument2 := "that should be split into multiple lines "
	_, someInteger, someVariable := testFn(veryveryLongArgument2, superVeryLongArgumentSoWeSplit)
}

func testFn(param1, param2 string) (string, int, error) {
	return param1 + param2, 0, nil
}

// Example 14: Already-multiline logical chain with overflowing continuation
func example14(itemType string) {
	for {
		if itemType != "" && itemType != "text" &&
			itemType != "allowed_input_text" && itemType != "allowed_output_text" {
			doSomething()
		}
	}
}

// Example 15: Parenthesized logical groups should stay packed
func example15(firstConditionName, secondConditionName, thirdConditionName, fourthConditionName, fallbackEnabled bool) {
	if (firstConditionName && secondConditionName) || (thirdConditionName && fourthConditionName) || fallbackEnabled {
		doSomething()
	}
}

// Example 16: If init statement with packed logical condition
func example16(val interface{}) {
	if result, ok := val.(map[string]interface{}); ok && len(result) > 0 && result["enabled"] == true {
		doSomething()
	}
}

// Example 17: For condition with packed logical chain
func example17(items []int, stopRequested bool, ctx context.Context, retryCount, maxRetries int) {
	for i := 0; i < len(items) && !stopRequested && ctx.Err() == nil && retryCount < maxRetries; i++ {
		process(items[i])
	}
}

// Example 18: Return statement with packed logical chain
func example18(x, y, z, w int, enabled, active bool) bool {
	return x > 0 && y > 0 && z > 0 && w > 0 && x < 100 && y < 100 && z < 100 && w < 100 && enabled && active
}

// Example 19: Single-element composite literals should stay compact
func example19SingleElementCompositeLiteral(swapRequest SwapRequest) {
	expectedOut := []requestResponse{
		{
			request: swapRequest,
			response: &SwapInfo{
				SwapHash: lntypes.Hash{
					1,
				},
				Marker: sampleMarker{
					"ready",
				},
			},
		},
	}
	_ = expectedOut
}

// Example 20: Keyed composite elements should not share a line
func example20KeyedElements(resId1, resId2 ReservationID) {
	requests := []ReservationRequest{
		{
			ID: resId1,
		}, {
			ID: resId2,
		},
	}
	_ = requests
}

// Example 21: Simple elided composite elements should stay packed
func example21PackedElidedElements() {
	hops := []*RouteHop{
		{}, {}, {},
	}
	preimages := []SamplePreimage{
		{1}, {2}, {3},
		{4}, {5}, {6},
		{7}, {8}, {9},
	}
	_, _ = hops, preimages
}

// Example 22: Elided composite keyed values should expand outer braces
func example22ElidedCompositeKeyedValues() {
	requiredOps := map[string][]PermissionOp{
		"/service/Action": {{
			Entity: "swap",
			Action: "execute",
		}, {
			Entity: "service",
			Action: "read",
		}},
		"/service/Read": {{
			Entity: "swap",
			Action: "read",
		}},
	}
	_ = requiredOps
}

// Stubs to make the file compile
type formatter struct{}

func (*formatter) classifyLine(string, bool, bool) string { return "" }
func doSomething()                                        {}
func checkPermission(interface{}) bool                    { return false }
func validateInput(interface{}) bool                      { return false }
func verifySignature(interface{}) bool                    { return false }
func process(int)                                         {}
func someVeryLongFunctionName(int, int) int               { return 0 }
func anotherLongFunction(int, int) int                    { return 0 }
func someCheck(int) bool                                  { return false }
func anotherCheck(int) bool                               { return false }

type SwapRequest struct{}
type SwapInfo struct {
	SwapHash interface{}
	Marker   interface{}
}
type requestResponse struct {
	request  SwapRequest
	response *SwapInfo
}
type sampleMarker []string
type RouteHop struct{}
type SamplePreimage [32]byte
type PermissionOp struct {
	Entity string
	Action string
}
type ReservationID struct{}
type ReservationRequest struct {
	ID ReservationID
}

type clientType struct{}

func (clientType) WithTimeout(time.Duration) clientType { return clientType{} }
func (clientType) WithRetry(int) clientType             { return clientType{} }
func (clientType) WithHeaders(interface{}) clientType   { return clientType{} }
func (clientType) Execute(context.Context, interface{}) interface{} {
	return nil
}

var client clientType
var headers interface{}
var ctx context.Context
var request interface{}

const (
	TypeA = iota
	TypeB
	TypeC
	TypeD
	TypeE
	TypeF
	TypeG
	TypeH
	TypeI
	TypeJ
	TypeK
)

func main() {}
