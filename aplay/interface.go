package aplay

import (
	"errors"

	"github.com/toy80/audio/wav"
)

var (
	// ErrUnsupportedFormat indicates the input format is not supported.
	ErrUnsupportedFormat = errors.New("audio: Unsupported format")
)

// Player is abstract audio player. to use it, place it into a 3D scene, select a song, then press the play button.
type Player interface {
	SetPosition([3]float32)

	SetVelocity([3]float32)

	SetDirection([3]float32)

	// SetOverlayGain set overlay gain of the source, normalized between 0 to 1
	SetOverlayGain(float32)

	// IsRelative reports whether the player postion is relative to the listener.
	// relative postion is typically use for play BGM
	IsRelative() bool

	// SetRelative set/unset retative position
	SetRelative(b bool)

	// Play the sound, if sound implement io.ReadAt interface (i.e. MemSound), it can be played
	// in difference players at same time. otherwise (i.e. SoundFile) the io.Read interface will be used.
	// if the player is playing another sound, the new sound will put in pending state.
	Play(x wav.Reader, gain float32, loop int) error

	// Terminate current playing sound, apply fade out if not zero. the pending sound also be removed,
	// but call Play immediately after Terminate is legal.
	Terminate()

	// SetFadeOut set the fade out duration for terminate sound, it not apply to normal ending sound.
	// there is not "fade in" feature.
	SetFadeOut(msec uint)

	IsPlaying() bool

	// TODO: what?
	Release()
}
