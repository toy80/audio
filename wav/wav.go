package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"golang.org/x/image/riff"
)

var (
	riffMagic    = riff.FourCC{'R', 'I', 'F', 'F'}
	waveMagic    = riff.FourCC{'W', 'A', 'V', 'E'}
	wavChunkFmt  = riff.FourCC{'f', 'm', 't', ' '}
	wavChunkData = riff.FourCC{'d', 'a', 't', 'a'}

	ErrFormat       = errors.New("wav: bad or unsupported format")
	ErrRandomAccess = errors.New("wav: not random accessible")
	ErrCorrupted    = errors.New("wav: corrupted data")
)

type wavReader struct {
	rr *riff.Reader
	u  io.Reader // underlying reader
	b  int64     // begin of file positon

	format         uint16
	channels       uint16
	frameRate      uint32
	bytesPerSecond uint32
	blockAlign     uint16
	bitsPerSample  uint16 // 8, 16, 32

	nd uint32    // data length
	dr io.Reader // data reader
}

func (f *wavReader) String() string {
	if f.dr == nil {
		return "[wave file: corrupted]"
	}
	return fmt.Sprintf("[wave file: %d x %v %dbits %dHz %v]",
		f.NumTracks(), f.SampleType(), f.bitsPerSample, f.Frequency(), f.Duration())
}

func (f *wavReader) Close() error {
	if c, ok := f.u.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (f *wavReader) SampleType() Type {
	if f.format == 3 {
		return F32
	}
	if f.format == 1 {
		if f.bitsPerSample == 8 {
			return U8
		}
		if f.bitsPerSample == 16 {
			return I16
		}
	}
	return 0
}

func (f *wavReader) Frequency() int {
	return int(f.frameRate)
}
func (f *wavReader) NumTracks() int {
	return int(f.channels)
}
func (f *wavReader) Read(p []byte) (int, error) {
	return f.dr.Read(p)
}

func (f *wavReader) CanRewind() bool {
	if _, ok := f.u.(io.Seeker); ok {
		return true
	}
	return false
}

func (f *wavReader) Rewind() error {
	return f.reset(true)
}

func (f *wavReader) Duration() time.Duration {
	if f.bytesPerSecond != 0 {
		return time.Second * time.Duration(f.nd) / time.Duration(f.bytesPerSecond)
	}
	return time.Second * time.Duration(f.nd) /
		(time.Duration(f.bitsPerSample/8) * time.Duration(f.channels) * time.Duration(f.frameRate))
}

func u32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func u16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

func (f *wavReader) reset(rewind bool) error {
	// reset file positon
	if rs, ok := f.u.(io.ReadSeeker); ok {
		if _, err := rs.Seek(f.b, io.SeekStart); err != nil {
			return err
		}
	} else if rewind {
		return ErrRandomAccess
	}
	// parse the file, must be RIFF + WAVE
	t, rr, err := riff.NewReader(f.u)
	if err != nil {
		return err
	}
	if t[0] != 'W' || t[1] != 'A' || t[2] != 'V' || t[3] != 'E' {
		return ErrFormat
	}
	f.dr = nil
	var fmtLoaded bool
	for {
		chunkID, chunkLen, chunkData, err := rr.Next()
		if err == io.EOF {
			return ErrCorrupted
		}
		if err != nil {
			return err
		}
		if chunkID == wavChunkFmt {
			var b []byte
			if b, err = ioutil.ReadAll(chunkData); err != nil {
				return err
			}

			f.format = u16(b)
			f.channels = u16(b[2:])
			f.frameRate = u32(b[4:])
			f.bytesPerSecond = u32(b[8:])
			f.blockAlign = u16(b[12:])
			f.bitsPerSample = u16(b[14:])

			if f.format != 1 && f.format != 3 {
				return ErrFormat // only PCM and float is supported
			}
			if f.format == 3 && f.bitsPerSample != 32 {
				return fmt.Errorf("wav: unsupported float bits width %d", f.bitsPerSample)
			} else if f.bitsPerSample != 8 && f.bitsPerSample != 16 {
				return fmt.Errorf("wav: unsupported integer bits width %d", f.bitsPerSample)
			}
			fmtLoaded = true
		} else if chunkID == wavChunkData {
			if !fmtLoaded {
				return ErrCorrupted // data before fmt, ill form
			}
			f.nd = chunkLen
			f.dr = chunkData
			f.rr = rr
			return nil
		}
	}
}

func NewReader(r io.Reader) (Reader, error) {
	w := &wavReader{u: r}
	if rs, ok := r.(io.ReadSeeker); ok {
		w.b, _ = rs.Seek(0, io.SeekCurrent)
	}
	if err := w.reset(false); err != nil {
		return nil, err
	}
	return w, nil
}

// Open PCM wave file
func Open(filename string) (ReadCloser, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	x, err := NewReader(f)
	if err != nil {
		return nil, err
	}
	return x.(ReadCloser), err
}

// Write PCM wave to writer w
func Write(w io.Writer, wave Reader) (err error) {
	var format uint16
	var channels uint16
	var frameRate uint32
	var bytesPerSecond uint32
	var blockAlign uint16
	var bitsPerSample uint16 // 8, 16, 32
	//var cbSize uint32
	var subChunkSize1 uint32
	var subChunkSize2 uint32
	var chunkSize uint32

	isFloat := wave.SampleType() == F32
	if isFloat {
		format = 3
	} else {
		format = 1
	}
	channels = uint16(wave.NumTracks())
	frameRate = uint32(wave.Frequency())
	bitsPerSample = uint16(wave.SampleType().Bits())
	if bitsPerSample%8 != 0 {
		return fmt.Errorf("wav: unsupported bits width %d", bitsPerSample)
	}
	blockAlign = channels * bitsPerSample / 8
	bytesPerSecond = uint32(blockAlign) * frameRate

	buf, err := ioutil.ReadAll(wave)
	if err != nil {
		return err
	}

	subChunkSize1 = 16
	//cbSize = 0

	subChunkSize2 = uint32(len(buf))
	//subChunkSize2 = subChunkSize2 / uint32(blockAlign) * uint32(blockAlign)
	chunkSize = 4 + 4 + 8 + subChunkSize1 + 8 + subChunkSize2
	padded := chunkSize&0x1 != 0
	if padded {
		chunkSize++
	}
	if _, err = w.Write(riffMagic[:]); err != nil {
		return err
	}
	if err = binary.Write(w, binary.LittleEndian, chunkSize); err != nil {
		return err
	}
	if _, err = w.Write(waveMagic[:]); err != nil {
		return err
	}

	if _, err = w.Write(wavChunkFmt[:]); err != nil {
		return err
	}

	if err = binary.Write(w, binary.LittleEndian, subChunkSize1); err != nil {
		return err
	}
	if err = binary.Write(w, binary.LittleEndian, format); err != nil {
		return err
	}
	if err = binary.Write(w, binary.LittleEndian, channels); err != nil {
		return err
	}
	if err = binary.Write(w, binary.LittleEndian, frameRate); err != nil {
		return err
	}
	if err = binary.Write(w, binary.LittleEndian, bytesPerSecond); err != nil {
		return err
	}
	if err = binary.Write(w, binary.LittleEndian, blockAlign); err != nil {
		return err
	}
	if err = binary.Write(w, binary.LittleEndian, bitsPerSample); err != nil {
		return err
	}
	// 	if err = binary.Write(w, binary.LittleEndian, cbSize); err != nil {
	// 		return err
	// 	}

	if _, err = w.Write(wavChunkData[:]); err != nil {
		return err
	}
	if err = binary.Write(w, binary.LittleEndian, subChunkSize2); err != nil {
		return err
	}
	if _, err = w.Write(buf); err != nil {
		return err
	}
	if padded {
		var zero [1]byte
		if _, err = w.Write(zero[:]); err != nil {
			return err
		}
	}

	return nil
}

// WriteFile write PCM wave to file
func WriteFile(filename string, wave Reader) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return Write(f, wave)
}
