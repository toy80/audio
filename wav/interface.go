// Package wav privides interface to PCM wave data.
// 8bits unsigned, 16bits signed and 32bits float are supported.
// samples must tight interlacing.
package wav

import (
	"fmt"
	"time"
)

// Type is sample type
type Type int8

// Sample types
const (
	U8 Type = iota
	I16
	F32
)

func (x Type) String() string {
	switch x {
	case U8:
		return "8bits unsigned"
	case I16:
		return "16bits signed"
	case F32:
		return "32bits float"
	default:
		return fmt.Sprintf("unkown pcm wave type %d", int8(x))
	}
}

// Bits per sample
func (x Type) Bits() int {
	switch x {
	case U8:
		return 8
	case I16:
		return 16
	case F32:
		return 32
	default:
		return 0
	}
}

// Reader for PCM data
type Reader interface {
	Read(p []byte) (n int, err error)

	// SampleType reporst sample's data type
	SampleType() Type
	// Frequency reports the sample frequency. i.e. 441000
	Frequency() int
	// NumTracks reports track count
	NumTracks() int
	// Duration of the audio
	Duration() time.Duration
}

// ReadCloser for PCM data
type ReadCloser interface {
	Reader
	Close() error
}
