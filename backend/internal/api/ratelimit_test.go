package api

import "testing"

func TestAPIWindowBlocksTheNextCall(t *testing.T) {
	window := &apiWindow{byKey: map[string]attemptWindow{}, max: 2}
	if !window.allow("user-1") || !window.allow("user-1") || window.allow("user-1") {
		t.Fatal("authenticated window must block the call past the limit")
	}
	if !window.allow("user-2") {
		t.Fatal("a different key must keep its own window")
	}
}
