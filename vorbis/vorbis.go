// Package vorbis is a vorbis decoder implementation in pure golang.
// it was wrote from scratch follow the [Vorbis I specification](https://xiph.org/vorbis/doc/Vorbis_I_spec.html).
package vorbis

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/toy80/audio/ogg"
	"github.com/toy80/audio/wav"
)

const (
	maxChannels = 20
)

// as this vorbis decoder is converted from the same one written in C++
// the assert macro is keep for debugging purpose
func assert(b bool) {
	if debug && !b {
		panic("assert")
	}
}

type PacketReader interface {
	NextPacket() (err error)
	ReadBits(bits uint32) uint32
	ReadBytes(p []byte)
	ReadString() string
}

// Vorbis decoder
type Vorbis struct {
	pr PacketReader

	headerReady bool

	vorbisVersion  uint32
	audioChannels  uint8
	audioFrameRate uint32 // frequency
	maxBitrate     uint32
	nomBitrate     uint32
	minBitrate     uint32
	blockSize      [2]uint32
	slope          [2][]float32

	overlap [2][2]sOverlap

	vendor string

	// comments is like "TITLE=xxxx", according to the spec, the key are not required to be unique,
	// we don't care about this, just join them up into unique key
	comments map[string]string

	mdct [2]MDCT

	chnBufs        []sChannelBuf
	prevWindowFlag int
	prevBlockSize  uint32
	idxAutoPacket  uint32    // non-audio packet is excluded
	tempBuf        []float32 // use in decode format 2 residue

	// output format and position
	outTypeSize int      // bytes per sample
	outType     wav.Type // data type
	outBuf      []byte   // current output buffer
	outBufRes   []byte   // memory reserve for outBuf

	numCodebooks uint32
	codebooks    []sCodeBook
	floors       []sFloor
	numFloors    uint32
	residues     []sResidue
	numResidues  uint32
	mappings     []sMapping
	numMappings  uint32
	modes        []sMode
	numModes     uint32
}

func (vb *Vorbis) initOverlap() {
	for i := 0; i < 2; i++ {
		w := vb.blockSize[i]
		hw := w >> 1
		assert(len(vb.slope[i]) == 0)
		s := make([]float32, hw)
		vb.slope[i] = s
		for n := uint32(0); n < hw; n++ {
			a := math.Sin((float64(n) + 0.5) / float64(w) * math.Pi)
			s[n] = float32(math.Sin(0.5 * math.Pi * a * a))
		}
	}

	ov := &vb.overlap[0][0]
	ov.sw = int(vb.blockSize[0] >> 1)
	ov.s = vb.slope[0]
	ov.w0 = int(vb.blockSize[0] >> 1)
	ov.w1 = int(vb.blockSize[0] >> 1)
	ov.a0 = 0
	ov.a1 = 0
	ov.numPcm = (ov.w0 >> 1) + (ov.w1 >> 1)

	ov = &vb.overlap[0][1]
	ov.sw = int(vb.blockSize[0] >> 1)
	ov.s = vb.slope[0]
	ov.w0 = int(vb.blockSize[0] >> 1)
	ov.w1 = int(vb.blockSize[1] >> 1)
	ov.a0 = 0
	ov.a1 = (ov.w1 >> 1) - (ov.w0 >> 1)
	ov.numPcm = (ov.w0 >> 1) + (ov.w1 >> 1)

	ov = &vb.overlap[1][0]
	ov.sw = int(vb.blockSize[0] >> 1)
	ov.s = vb.slope[0]
	ov.w0 = int(vb.blockSize[1] >> 1)
	ov.w1 = int(vb.blockSize[0] >> 1)
	ov.a0 = (ov.w0 >> 1) - (ov.w1 >> 1)
	ov.a1 = 0
	ov.numPcm = (ov.w0 >> 1) + (ov.w1 >> 1)

	ov = &vb.overlap[1][1]
	ov.sw = int(vb.blockSize[1] >> 1)
	ov.s = vb.slope[1]
	ov.w0 = int(vb.blockSize[1] >> 1)
	ov.w1 = int(vb.blockSize[1] >> 1)
	ov.a0 = 0
	ov.a1 = 0
	ov.numPcm = (ov.w0 >> 1) + (ov.w1 >> 1)
}

func (vb *Vorbis) String() string {
	return fmt.Sprintf("[vorbis file: %d x %v %dbits %dHz %v TITLE=%s]",
		vb.NumTracks(), vb.SampleType(), vb.BitsPerSample(), vb.Frequency(), vb.Duration(), vb.Comment("TITLE"))
}

func (vb *Vorbis) Read(buf []byte) (n int, err error) {
	return vb.output(buf)
}

// // NumFrames reports total frames count
// func (vb *Vorbis) NumFrames() uint64 {
// 	return vb.totalFrames
// }

// SampleType reporst sample's data type
func (vb *Vorbis) SampleType() wav.Type {
	return vb.outType
}

// BitsPerSample reports bits per sample (single track)
func (vb *Vorbis) BitsPerSample() int {
	return vb.outTypeSize * 8
}

// Frequency reports the sample frequency. i.e. 441000
func (vb *Vorbis) Frequency() int {
	return int(vb.audioFrameRate)
}

// NumTracks reports track count
func (vb *Vorbis) NumTracks() int {
	return int(vb.audioChannels)
}

// Duration of the audio
func (vb *Vorbis) Duration() time.Duration {
	// TODO: implement
	// frames := vb.NumFrames()
	// if vb.audioFrameRate == 0 || frames == 0 {
	// 	return 0
	// }
	// return time.Second * time.Duration(frames) / time.Duration(vb.audioFrameRate)
	return 0
}

// Vendor info
func (vb *Vorbis) Vendor() string {
	return vb.vendor
}

// Comments reports the NAME=value comment pairs
func (vb *Vorbis) Comments() map[string]string {
	return vb.comments
}

// Comment reports of the name, i.e.  vb.Comment("TITLE")
func (vb *Vorbis) Comment(name string) string {
	if vb.comments == nil {
		return ""
	}
	return vb.comments[name]
}

func (vb *Vorbis) Init(r io.Reader) (err error) {
	pr := new(ogg.Reader)
	if err = pr.Init(r); err != nil {
		return
	}
	vb.pr = pr
	return
}

// New vorbis decoder
func New(r io.Reader, t wav.Type) (vb *Vorbis, err error) {
	defer func() {
		if err != nil {
			vb = nil
		}
	}()

	vb = new(Vorbis)
	if err = vb.setOutputFormat(t); err != nil {
		return
	}

	if err = vb.Init(r); err != nil {
		return
	}

	if !vb.parseVorbisHeaders() {
		err = errors.New("failed to read vorbis headers")
		return
	}

	vb.chnBufs = make([]sChannelBuf, vb.audioChannels)
	vb.initOverlap()
	vb.requireTempBufSize(vb.blockSize[1], true)
	return
}

// Open vorbis file
func Open(filename string) (*Vorbis, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	vb, err := New(f, wav.I16)
	if err != nil {
		f.Close()
		return nil, err
	}
	return vb, nil
}

func (vb *Vorbis) Close() error {
	if c, ok := vb.pr.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}
