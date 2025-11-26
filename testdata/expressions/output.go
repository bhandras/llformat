package main

import (
	"context"
	"time"
)

// Example 1: Long if condition with && and ||
func example1(userIsAuthenticated, userHasPermission, accountIsLocked,
	sessionIsValid, isAdminOverride bool) {

	if userIsAuthenticated && userHasPermission && !accountIsLocked &&
		sessionIsValid || isAdminOverride {
		doSomething()
	}
}

// Example 2: Long assignment with binary operators
func example2(user, data, sig interface{}, bypassEnabled bool) bool {
	isValid := checkPermission(user) && validateInput(data) &&

		verifySignature(sig) || bypassEnabled

	return isValid
}

// Example 3: Method chain
func example3() {
	result := client.WithTimeout(30*time.Second).WithRetry(3).WithHeaders(
		headers,
	).Execute(ctx, request)
	_ = result
}

// Example 4: Long case statement
func example4(val int) {
	switch val {
	case TypeA, TypeB, TypeC, TypeD, TypeE, TypeF, TypeG, TypeH, TypeI,
		TypeJ, TypeK:
		doSomething()
	}
}

// Example 5: Long for condition
func example5(items []int, stopRequested bool, ctx context.Context, retryCount,
	maxRetries int) {

	for i := 0; i < len(items) && !stopRequested && ctx.Err() == nil &&
		retryCount < maxRetries; i++ {
		process(items[i])
	}
}

// Example 6: Complex boolean in return
func example6(a, b, c, d, e, f bool) bool {
	return a && b && c || d && e && f || (a && d) || (b && e) || (c && f)
}

// Example 7: Nested binary expressions
func example7(x, y, z, w int) bool {
	return x > 0 && y > 0 && z > 0 && w > 0 && x < 100 && y < 100 &&
		z < 100 && w < 100
}

// Example 8: Arithmetic expression
func example8(a, b, c, d, e, f, g, h int) int {
	result := a + b + c + d + e + f + g + h +
		someVeryLongFunctionName(a, b) + anotherLongFunction(c, d)

	return result
}

// Example 9: String concatenation (already handled by other formatter, should
// be untouched)
func example9() string {
	return "This is a very long string that spans multiple parts and is " +
		"concatenated together"
}

// Example 10: Mixed operators with comparison
func example10(val, min, max int, enabled, active bool) bool {
	return val >= min && val <= max && enabled && active &&
		someCheck(val) && anotherCheck(val)
}

// Stubs to make the file compile
func doSomething()                          {}
func checkPermission(interface{}) bool      { return false }
func validateInput(interface{}) bool        { return false }
func verifySignature(interface{}) bool      { return false }
func process(int)                           {}
func someVeryLongFunctionName(int, int) int { return 0 }
func anotherLongFunction(int, int) int      { return 0 }
func someCheck(int) bool                    { return false }
func anotherCheck(int) bool                 { return false }

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
