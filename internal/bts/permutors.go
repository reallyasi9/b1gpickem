package bts

import (
	"math/big"
	"sync"

	"github.com/segmentio/fasthash/jody"
)

type Permutor interface {
	Len() int
	NumberOfPermutations() *big.Int
	Permutation() []int
	Permute() bool
	Reset()
}

// IndexPermutor permutes an integer range from 0 to N.
type IndexPermutor struct {
	indices []int
	i       int
	counter []int
	first   bool
	lock    sync.Mutex
}

// NewIndexPermutor creates an IndexPermutor for the integer range [0, size)
func NewIndexPermutor(size int) *IndexPermutor {
	out := make([]int, size)
	counter := make([]int, size)
	for i := 0; i < size; i++ {
		out[i] = i
	}
	return &IndexPermutor{indices: out, i: 0, counter: counter, first: true}
}

// Len returns the length of the set being permuted.
func (ip *IndexPermutor) Len() int {
	return len(ip.indices)
}

// NumberOfPermutations returns the number of permutations possible for the set.
func (ip *IndexPermutor) NumberOfPermutations() *big.Int {
	return factorial(ip.Len())
}

// Permutation gets the current permutation.
// This value is a view into the internal slice and will change if Next() is called.
func (ip *IndexPermutor) Permutation() []int {
	return ip.indices
}

// Permute permutes the internal indices and returns false if the iterator has run out of permutations. Get the current permutation with Permutation().
// The implementation uses Heap's algorithm (non-recursive).
func (ip *IndexPermutor) Permute() bool {

	if len(ip.indices) < 2 {
		return false
	}

	ip.lock.Lock()
	defer ip.lock.Unlock()

	if ip.first {
		ip.first = false
		return true
	}

	for ip.i < len(ip.indices) {

		if ip.counter[ip.i] < ip.i {
			if ip.i%2 == 0 {
				ip.indices[0], ip.indices[ip.i] = ip.indices[ip.i], ip.indices[0]
			} else {
				ip.indices[ip.counter[ip.i]], ip.indices[ip.i] = ip.indices[ip.i], ip.indices[ip.counter[ip.i]]
			}

			ip.counter[ip.i]++
			ip.i = 0

			return true

		} else {
			ip.counter[ip.i] = 0
			ip.i++
		}
	}

	return false
}

// Reset resets the permutor to its initial state.
// This invalidates any slices of indices previously read with Permutation().
func (ip *IndexPermutor) Reset() {
	if ip.Len() < 2 {
		return
	}

	ip.lock.Lock()
	defer ip.lock.Unlock()

	ip.indices = make([]int, ip.Len())
	ip.counter = make([]int, ip.Len())
	for i := 0; i < ip.Len(); i++ {
		ip.indices[i] = i
	}
	ip.first = true
	ip.i = 0
}

// IdenticalPermutor permutes a set of potentially repeated integers with n_i copies of integers i=0 to i=N.
type IdenticalPermutor struct {
	indices  []int
	setSizes []int // for faster permutation counting
	visited  map[uint64]struct{}
	i        int
	counter  []int
	first    bool
	lock     sync.Mutex
}

// NewIdenticalPermutor creates an IdenticalPermutor for potentially duplicated integers in the range [0, len(setSizes)).
// Each passed ith value in the variatic argument represents the number of identical copies of integer i that is in the set to be permuted.
// For example:
//   NewIdenticalPermutor(2, 0, 3, 1)
// This will create a permutor over the set [0, 0, 2, 2, 2, 3] (2 copies of 0, 0 copies of 1, 3 copies of 2, and 1 copy of 3).
func NewIdenticalPermutor(setSizes ...int) *IdenticalPermutor {
	out := make([]int, 0)
	for set, setSize := range setSizes {
		for j := 0; j < setSize; j++ {
			out = append(out, set)
		}
	}
	counter := make([]int, len(out))
	visited := make(map[uint64]struct{})
	return &IdenticalPermutor{indices: out, setSizes: setSizes, visited: visited, i: 0, counter: counter, first: true}
}

// Len returns the length of the set being permuted.
func (ip *IdenticalPermutor) Len() int {
	return len(ip.indices)
}

// NumberOfPermutations returns the number of permutations possible for the set.
func (ip *IdenticalPermutor) NumberOfPermutations() *big.Int {
	fact := factorial(ip.Len())
	for _, set := range ip.setSizes {
		fact.Div(fact, factorial(set))
	}
	return fact
}

// Permutation gets the current permutation.
// This value is a view into the internal slice and will change if Next() is called.
func (ip *IdenticalPermutor) Permutation() []int {
	return ip.indices
}

// Permute permutes the internal indices and returns false if the iterator has run out of permutations. Get the current permutation with Permutation().
// The implementation uses Heap's algorithm (non-recursive) and a map of hashes to keep track of which permutations have been seen already.
// This means for very large permutations, there is some small probability that a hash collision will occur and certain permutations could be skipped in the iteration.
func (ip *IdenticalPermutor) Permute() bool {
	if len(ip.indices) < 2 {
		return false
	}

	ip.lock.Lock()
	defer ip.lock.Unlock()

	if ip.first {
		ip.first = false
		ip.visited[hash(ip.indices)] = struct{}{}
		return true
	}

	for ip.i < len(ip.indices) {
		if ip.counter[ip.i] < ip.i {
			if ip.i%2 == 0 {
				ip.indices[0], ip.indices[ip.i] = ip.indices[ip.i], ip.indices[0]
			} else {
				ip.indices[ip.counter[ip.i]], ip.indices[ip.i] = ip.indices[ip.i], ip.indices[ip.counter[ip.i]]
			}

			ip.counter[ip.i]++
			ip.i = 0

			h := hash(ip.indices)
			if _, ok := ip.visited[h]; ok {
				continue
			}

			ip.visited[h] = struct{}{}

			return true

		} else {
			ip.counter[ip.i] = 0
			ip.i++
		}
	}

	return false
}

// Reset resets the permutor to its initial state.
// This invalidates any slices of indices previously read with Permutation().
func (ip *IdenticalPermutor) Reset() {
	if ip.Len() < 2 {
		return
	}

	ip.lock.Lock()
	defer ip.lock.Unlock()

	ip.indices = make([]int, 0)
	for set, setSize := range ip.setSizes {
		for j := 0; j < setSize; j++ {
			ip.indices = append(ip.indices, set)
		}
	}
	ip.counter = make([]int, len(ip.indices))
	ip.visited = make(map[uint64]struct{})
	ip.first = true
	ip.i = 0
}

func factorial(n int) *big.Int {
	z := new(big.Int)
	return z.MulRange(1, int64(n))
}

func hash(v []int) uint64 {
	h := jody.HashUint64(uint64(v[0]))
	for _, x := range v[1:] {
		h = jody.AddUint64(h, uint64(x))
	}
	return h
}
