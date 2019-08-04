package vorbis

import (
	"fmt"
	"io"
)

// simple binary tree implementation
type huffmanNode struct {
	sub [2]*huffmanNode
	sym uint32
}

func newHuffmanNode() *huffmanNode { return &huffmanNode{sym: 0xFFFFFFFF} }

type huffmanDecoder struct {
	huffmanNode
}

// TODO: huffman decode is bottle neck, can be optimize with lookup table
func (d *huffmanDecoder) decodeHuffman(in interface {
	ReadBits(bits uint32) uint32
}) uint32 {
	p := &d.huffmanNode
	for {
		p = p.sub[in.ReadBits(1)]
		if p.sym != 0xFFFFFFFF {
			return p.sym
		}
	}
}

func (p *huffmanNode) fillHuffmanTable(sym uint32, len uint8) bool {
	if len == 0 {
		return false
	}
	if p.sym != 0xFFFFFFFF {
		return false
	}

	if p.sub[0] == nil {
		// ...0000...
		for ; len != 0; len-- {
			p.sub[0] = newHuffmanNode()
			p = p.sub[0]
		}
		p.sym = sym
		return true
	}
	if p.sub[0].fillHuffmanTable(sym, len-1) {
		return true
	}

	if p.sub[1] == nil {
		// ...1000...
		len--
		p.sub[1] = newHuffmanNode()
		p = p.sub[1]
		for ; len != 0; len-- {
			p.sub[0] = newHuffmanNode()
			p = p.sub[0]
		}
		p.sym = sym
		return true
	}

	return p.sub[1].fillHuffmanTable(sym, len-1)
}

// for debug.  i.e. dumpHuffman(os.Stderr, "+")
func (p *huffmanNode) dumpHuffman(w io.Writer, code string) {
	if p.sub[0] == nil && p.sub[1] == nil {
		fmt.Printf("%10d %s\n", p.sym, code)
		return
	}
	if p.sub[0] != nil {
		p.sub[0].dumpHuffman(w, code+"0")
	}
	if p.sub[1] != nil {
		p.sub[1].dumpHuffman(w, code+"1")
	}
}

func (d *huffmanDecoder) constructHufman(codeLengths []uint8) error {
	d.huffmanNode.sub[0] = nil
	d.huffmanNode.sub[1] = nil
	d.huffmanNode.sym = 0xFFFFFFFF
	// construct the tree
	for s, l := range codeLengths {
		if l == 0 {
			continue // vorbis has "sparse lengths"
		}
		if l > 32 {
			return fmt.Errorf("vorbis: unsupported huffman code length: %d", l)
		}
		if !d.huffmanNode.fillHuffmanTable(uint32(s), l) {
			return fmt.Errorf("vorbis: conflict huffman code 0x%X", s)
		}
	}
	// for vorbis, unused codeword entries is valid
	return nil
}
