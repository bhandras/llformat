package main

import "fmt"

// Example 1: Switch with cases that need blank lines between them
func exampleSwitch(val int) string {
	switch val {
	case 1:
		fmt.Println("one")

		return "one"

	case 2:
		fmt.Println("two")

		return "two"

	case 3, 4, 5:
		fmt.Println("three, four, or five")

		return "medium"

	default:
		fmt.Println("other")

		return "other"
	}
}

// Example 2: Return statements that need blank lines before them
func exampleReturn(a, b int) int {
	if a > b {
		return a
	}
	sum := a + b
	product := a * b
	if sum > product {
		return sum
	}

	return product
}

// Example 3: Interface with methods that need blank lines between them
type MyInterface interface {
	Method1() error

	Method2(ctx context.Context) (string, error)

	Method3(a, b int) int
}

// Example 4: Interface with embedded interfaces
type ComplexInterface interface {
	io.Reader
	io.Writer

	Process(data []byte) error

	Validate() bool
}

// Example 5: Nested switch
func nestedSwitch(a, b int) string {
	switch a {
	case 1:
		switch b {
		case 1:
			return "1-1"

		case 2:
			return "1-2"

		default:
			return "1-other"
		}

	case 2:
		return "two"

	default:
		return "other"
	}
}

// Example 6: Empty interface (should not add blank lines)
type EmptyInterface interface{}

// Example 7: Single method interface (should not add blank lines)
type SingleMethod interface {
	DoSomething() error
}

// Example 8: Return right after if opening
func earlyReturn(ok bool) error {
	if !ok {
		return fmt.Errorf("not ok")
	}
	doSomething()

	return nil
}

// Example 9: Return right after case
func returnAfterCase(val int) int {
	switch val {
	case 1:
		return 1

	case 2:
		return 2

	default:
		return 0
	}
}

// Example 10: Multiple returns in sequence (only first needs blank)
func multiReturn(a int) int {
	if a < 0 {
		return -1
	}
	if a == 0 {
		return 0
	}

	return a
}

// Stub types and functions
type context struct{}

func (context) Context() {}
func doSomething() {}

type io struct{}

func (io) Reader() {}
func (io) Writer() {}
