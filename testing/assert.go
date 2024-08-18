package testing

import (
	"slices"
	"testing"
)

func AssertNoError(t *testing.T, o error) {
	Assert(t, o == nil, "expect nil but %v", o)
}

func AssertEquals[E comparable](t *testing.T, a, b E) {
	Assert(t, a == b, "expect %v but %v", a, b)
}
func AssertSliceEquals[E comparable, S ~[]E](t *testing.T, a, b S) {
	Assert(t, slices.Equal(a, b), "expect %v but %v", a, b)
}
func Assert(t *testing.T, ok bool, message string, args ...any) {
	if !ok {
		t.Fatalf(message, args...)
	}
}
