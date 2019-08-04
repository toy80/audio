package wav

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"
)

// Block is PCM wave in memory
type Block struct {
	buf    []byte
	i      int64
	t      Type
	f      int
	tracks uint8
}

func (s *Block) String() string {
	if s.buf == nil {
		return "[wave block: corrupted]"
	}
	return fmt.Sprintf("[wave block: %d x %s %dHz %s]",
		s.NumTracks(), s.SampleType(), s.Frequency(), s.Duration())
}

// SampleType reporst sample's data type
func (s *Block) SampleType() Type {
	return s.t
}

// Frequency reports the sample frequency. i.e. 441000
func (s *Block) Frequency() int {
	return s.f
}

// NumTracks reports track count
func (s *Block) NumTracks() int {
	return int(s.tracks)
}

func (s *Block) Read(b []byte) (n int, err error) {
	if s.i >= int64(len(s.buf)) {
		return 0, io.EOF
	}
	n = copy(b, s.buf[s.i:])
	s.i += int64(n)
	return
}

// ReadAt implement io.ReadAt
func (s *Block) ReadAt(b []byte, off int64) (n int, err error) {
	if off >= int64(len(s.buf)) {
		return 0, io.EOF
	}
	n = copy(b, s.buf[off:])
	return
}

// Rewind to begin of data
func (s *Block) Rewind() error {
	s.i = 0
	return nil
}

// Duration of the data
func (s *Block) Duration() time.Duration {
	return time.Second * time.Duration(len(s.buf)) /
		(time.Duration(s.f) * time.Duration(s.t.Bits()/8) * time.Duration(s.tracks))
}

// ReadAll read all data into memory Block from reader x
func ReadAll(x Reader) (*Block, error) {
	b, err := ioutil.ReadAll(x)
	if err != nil {
		return nil, err
	}
	return &Block{buf: b,
		i:      0,
		t:      x.SampleType(),
		f:      x.Frequency(),
		tracks: uint8(x.NumTracks())}, nil
}

// NewBlock create PCM memory block from bytes
func NewBlock(b []byte, tracks uint8, t Type, freq int) *Block {
	p := &Block{buf: b,
		i:      0,
		t:      t,
		f:      freq,
		tracks: uint8(tracks)}
	return p
}

//
// func Convert(x Reader, t Type, freq, bits, tracks int) (*Block, error) {
//
// }
