package http_test

import (
	"testing"

	"github.com/kinabcd/ko/net/http"
	koT "github.com/kinabcd/ko/testing"
)

func TestBasicAuth(t *testing.T) {
	username := "lllqqqq"
	password := "jjvnnnannar4555"
	auth := http.EncodeBasicAuth(username, password)
	decodedUsername, decodedPasword, _ := http.DecodeBasicAuth(auth)
	koT.AssertEquals(t, username, decodedUsername)
	koT.AssertEquals(t, password, decodedPasword)
}
