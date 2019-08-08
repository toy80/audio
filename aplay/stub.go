// +build noaudio

package aplay

import (
	"github.com/toy80/audio/wav"
	"github.com/toy80/utils/debug"
)

var theMutedPlayer mutedPlayer

// IsAvailable reports whether audio available
func IsAvailable() bool {
	return false
}

// SetListenerPosition set the position of listener
func SetListenerPosition(p [3]float32) {
}

// ListenerPosition reports the position of listener
func ListenerPosition() (p [3]float32) {
	return
}

// SetListenerVelocity set the velocity of listener
func SetListenerVelocity(v [3]float32) {
}

// ListenerVelocity reports the velocity of listener
func ListenerVelocity() (v [3]float32) {
	return
}

// SetListenerOrient set the orientation of listener, the toward vector
func SetListenerOrient(toward [3]float32) {
}

// ListenerOrient reports the orientation of listener, the toward vector
func ListenerOrient() (toward [3]float32) {
	return
}

// SetMasterGain set the master gain
func SetMasterGain(gain float32) {
}

// MasterGain reports the master gain
func MasterGain() float32 {
	return 0
}

func WaitIdle() {}

type mutedPlayer int

func (mutedPlayer) SetPosition([3]float32) {}

func (mutedPlayer) SetVelocity([3]float32) {}

func (mutedPlayer) SetDirection([3]float32) {}

func (mutedPlayer) SetOverlayGain(float32) {}

func (mutedPlayer) IsRelative() bool { return false }

func (mutedPlayer) SetRelative(b bool) {}

func (mutedPlayer) Play(x wav.Reader, gain float32, loop int) error {
	debug.Println("play audio on a muted player will not output any sounds")
	return nil
}

func (mutedPlayer) Terminate() {}

func (mutedPlayer) SetFadeOut(msec uint) {}

func (mutedPlayer) IsPlaying() bool { return false }

func (mutedPlayer) Release() {}

func Alloc() Player {
	return theMutedPlayer
}
