//+build noaudio

package aplay

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

type mutedPlayer int

func Alloc() Player {
	return &theMutedPlayer
}
