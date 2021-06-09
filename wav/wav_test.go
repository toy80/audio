package wav

import (
	"bytes"
	"io"
	"math/rand"
	"testing"
)

func TestReadWrite(t *testing.T) {
	// generate white noise
	var data [12345]byte
	for i := range data {
		data[i] = byte(rand.Intn(8))
	}
	block := NewBlock(data[:], 2, I16, 44100)
	file := bytes.NewBuffer(nil)

	// test write
	if err := Write(file, block); err != nil {
		t.Fatal(err)
	}

	// test read
	wavData := file.Bytes()
	x, err := NewReader(bytes.NewReader(wavData))
	if err != nil {
		t.Fatal(err)
	}

	data2, err := io.ReadAll(x)
	if err != nil {
		t.Fatal(err)
	}

	if len(data2) != len(data) {
		t.Fatal("data length miss-match")
	}
	for i, v := range data2 {
		if data[i] != v {
			t.Fatalf("data miss-match at byte %d: %d != %d", i, v, data[i])
		}
	}
}
