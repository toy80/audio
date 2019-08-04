package aplay

import (
	"bytes"
	"testing"
	"time"

	"github.com/toy80/go-al/vorbis"
	"github.com/toy80/go-al/wav"
)

func TestEmptyWav(t *testing.T) {
	wav, err := wav.NewReader(bytes.NewReader(emptyWav))
	if err != nil {
		t.Fatal(err)
	}
	player := Alloc()
	if err := player.Play(wav, 0.9, 0); err != nil {
		t.Fatal(wav, ":", err)
	}
	time.Sleep(100 * time.Millisecond)

}

func TestEmptyOgg(t *testing.T) {
	v, err := vorbis.New(bytes.NewReader(emptyOgg), wav.I16)
	if err != nil {
		t.Fatal(err)
	}
	player := Alloc()
	if err := player.Play(v, 0.9, 0); err != nil {
		t.Fatal(v, ":", err)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestSilentWav(t *testing.T) {
	wav, err := wav.NewReader(bytes.NewReader(silentWav))
	if err != nil {
		t.Fatal(err)
	}
	player := Alloc()
	if err := player.Play(wav, 0.9, 0); err != nil {
		t.Fatal(wav, ":", err)
	}
	time.Sleep(100 * time.Millisecond)
	player.Terminate()
}

func TestSilentOgg(t *testing.T) {
	v, err := vorbis.New(bytes.NewReader(silentOgg), wav.I16)
	if err != nil {
		t.Fatal(err)
	}

	player := Alloc()
	if err := player.Play(v, 0.9, 0); err != nil {
		t.Fatal(v, ":", err)
	}
	time.Sleep(100 * time.Millisecond)
	player.Terminate()
}
