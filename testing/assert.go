package testing

import (
	"fmt"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
)

func AssertNoError(t *testing.T, o error) {
	assert(t, o == nil, "expect nil but %v", o)
}

func AssertEquals[E comparable](t *testing.T, a, b E) {
	assert(t, a == b, "expect %v but %v", a, b)
}
func AssertSliceEquals[E comparable, S ~[]E](t *testing.T, a, b S) {
	assert(t, slices.Equal(a, b), "expect %v but %v", a, b)
}
func Assert(t *testing.T, ok bool, message string, args ...any) {
	assert(t, ok, message, args...)
}

func assert(t *testing.T, ok bool, message string, args ...any) {
	if !ok {
		stack := string(debug.Stack())
		sp := strings.Split(stack, "\n")
		newStack := slices.Concat(sp[:1], sp[7:])
		msg := fmt.Sprintf(message, args...)
		t.Fatal(msg, "\n", strings.Join(newStack, "\n"))
	}
}
