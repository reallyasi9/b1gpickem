package bts

import (
	"math/big"
	"testing"
)

func TestIndexPermutor(t *testing.T) {
	s := []int{0, 1, 2, 3, 4, 5}
	p := NewIndexPermutor(6)

	// Just in case
	if p.Len() != 6 {
		t.Errorf("expected %v, got %v", 6, p.Len())
	}

	// First should be identical
	p.Permute()
	test := p.Permutation()
	if !check(s, test) {
		t.Errorf("expected %v, got %v", s, test)
	}

	// Second should not be identical
	p.Permute()
	test = p.Permutation()
	if check(s, test) {
		t.Errorf("expected different from %v, got %v", s, test)
	}

	s2 := []int{0, 1, 2}
	p2 := NewIndexPermutor(3)

	// Should be only 6 of these
	n := 0
	for p2.Permute() {
		test = p2.Permutation()
		t.Log(test)
		n++
	}

	if n != 6 {
		t.Errorf("expected 6, got %v", n)
	}

	// Last should be different than first
	if check(s2, test) {
		t.Errorf("expected different from %v, got %v", s2, test)
	}

	// Should be a! of these
	p3 := NewIndexPermutor(6)
	n = 0
	for p3.Permute() {
		// test = p3.Permutation()
		// t.Log(test)
		n++
	}

	factest := factorial(6)
	if factest.Cmp(big.NewInt(int64(n))) != 0 {
		t.Errorf("expected %v, got %v", factest, n)
	}
	nop := p3.NumberOfPermutations()
	if factest.Cmp(nop) != 0 {
		t.Errorf("expected %v, got %v", factest, nop)
	}
}

func BenchmarkPermuteAll10(b *testing.B) {
	p := NewIndexPermutor(10)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for p.Permute() {
		}
		p.Reset()
	}
}
