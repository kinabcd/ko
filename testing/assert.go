package testing

import "testing"

func AssertNoError(t *testing.T, o error) {
	Assert(t, o == nil, "expect nil but %v", o)
}

func AssertEquals(t *testing.T, a, b any) {
	Assert(t, a == b, "expect %v but %v", a, b)
}
func Assert(t *testing.T, ok bool, message string, args ...any) {
	if !ok {
		t.Fatalf(message, args...)
	}
}
