package vorbis

import (
	"bytes"
	"io"
	"log"
	"testing"

	"github.com/toy80/go-al/ogg"
	"github.com/toy80/go-al/wav"
)

func TestOgg(t *testing.T) {

	f := bytes.NewReader(oggfile1)
	ogg := new(ogg.Reader)
	var err error
	if err = ogg.Init(f); err != nil {
		log.Fatalln(err)
	}
	var buf [123]byte
	for {
		ogg.ReadBytes(buf[:])
		if ogg.EndOfPacket() {
			err = ogg.NextPacket()
			if err != nil && err != io.EOF {
				log.Fatal(err)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
	}
}

func BenchmarkVorbis(b *testing.B) {
	var buf [1024]byte
	var sz int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(oggfile1)
		vb, err := New(r, wav.I16)
		if err != nil {
			b.Fatal(err)
		}
		for {
			n, err := vb.Read(buf[:])
			sz += int64(n)
			if err != nil {
				if err != io.EOF {
					b.Fatal(err)
				}
				break
			}
		}
	}

	b.SetBytes(sz)
	b.ReportAllocs()
}
