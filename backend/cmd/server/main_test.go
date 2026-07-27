package main

import "testing"

func TestProductName(t *testing.T) {
	t.Parallel()

	if productName != "ModelPort" {
		t.Fatalf("productName = %q, want ModelPort", productName)
	}
}
