package vorbis

import (
	"testing"
)

type fakeBits []byte

func (p *fakeBits) ReadBits(n uint32) (x uint32) {
	if n > 32 {
		panic("n > 32")
	}
	for i := uint32(0); i < n; i++ {
		x |= uint32((*p)[i]&1) << i
	}
	*p = (*p)[n:]
	return
}

func TestHuffman(t *testing.T) {
	lengths := []uint8{2, 4, 4, 4, 4, 2, 0, 3, 3, 0, 0, 0, 0}
	d := new(huffmanDecoder)
	err := d.constructHufman(lengths)
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeBits{
		0, 0,
		0, 1, 0, 0,
		0, 1, 0, 1,
		0, 1, 1, 0,
		0, 1, 1, 1,
		1, 0,
		1, 1, 0,
		1, 1, 1,
		0, 1, 0, 0,
		0, 1, 0, 0,
		0, 1, 0, 1,
		0, 1, 0, 0,
		0, 1, 0, 0,
		0, 1, 1, 1,
	}
	syms := []uint32{0, 1, 2, 3, 4, 5, 7, 8, 1, 1, 2, 1, 1, 4}
	for i, x := range syms {
		s := d.decodeHuffman(r)
		if s != x {
			t.Fatalf("decode %d failed, got %d, want %d", i, s, x)
		}
	}
}
