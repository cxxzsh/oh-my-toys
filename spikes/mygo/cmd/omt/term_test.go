package main

import (
	"os"
	"testing"
)

func TestNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if isTerminal(f) {
		t.Fatal("devnull looked like a terminal")
	}
}
