// +build !noaudio

package aplay

/*
#cgo windows LDFLAGS: -lOpenAL32
#cgo darwin LDFLAGS: -framework OpenAL
#cgo linux freebsd LDFLAGS: -lopenal
#ifdef __APPLE__
#	include <OpenAL/al.h>
#	include <OpenAL/alc.h>
#else
#	include <AL/al.h>
#	include <AL/alc.h>
#endif
*/
import "C"
import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/toy80/audio/wav"
	"github.com/toy80/utils/debug"
)

const (
	msecsPerBuffer    = 100
	buffersPerSlot    = 4
	msecsPerSlot      = msecsPerBuffer * buffersPerSlot
	msecsPollInterval = 20
)

type cmdType int

const (
	cmdAlloc cmdType = iota
	cmdQueue
	cmdTerm
	cmdRelease
	cmdOvGain
	cmdFade
)

func (t cmdType) String() string {
	switch t {
	case cmdAlloc:
		return "Alloc"
	case cmdQueue:
		return "Queue"
	case cmdTerm:
		return "Term"
	case cmdRelease:
		return "Release"
	case cmdOvGain:
		return "OvGain"
	case cmdFade:
		return "Fade"
	default:
		return fmt.Sprint(int(t))
	}
}

type cmd struct {
	typ   cmdType // command type
	src   *source
	param interface{}
}

func (c *cmd) String() string {
	return fmt.Sprintf("cmd %s %s %v", c.typ, c.src, c.param)
}

var (
	holder  *int
	device  *C.ALCdevice
	context *C.ALCcontext

	idle   = make(chan int, 1) // idle signal
	cmds   = make(chan *cmd, 1024)
	active = make(map[*source]bool)

	sourcePool = sync.Pool{New: func() interface{} {
		p := new(source)
		err := p.init()
		if err != nil {
			panic(err)
		}
		return p
	}}

	pendingPool = sync.Pool{New: func() interface{} {
		return new(pending)
	}}
	cmdPool = sync.Pool{New: func() interface{} {
		return new(cmd)
	}}
)

func init() {
	if unsafe.Sizeof(*(*C.ALfloat)(nil)) != unsafe.Sizeof(*(*float32)(nil)) {
		panic("OpenAL is not 32 bits.")
	}
	holder = new(int)
	runtime.SetFinalizer(holder, deinit)

	var err error
	defer func() {
		if err != nil {
			holder = nil
		}
	}()

	deviceName := C.alcGetString(nil, C.ALC_DEFAULT_DEVICE_SPECIFIER)
	if deviceName == nil {
		var x C.ALCchar
		device = C.alcOpenDevice(&x) // alcOpenDevice("")
	} else {
		device = C.alcOpenDevice(deviceName)
	}
	if device == nil {
		code := C.alcGetError(device)
		err = fmt.Errorf("alcOpenDevice() %s", getALErrorString(code))
		return
	}

	context = C.alcCreateContext(device, nil)
	if context == nil {
		code := C.alcGetError(device)
		err = fmt.Errorf("alcCreateContext() %s", getALErrorString(code))
		return
	}

	C.alcMakeContextCurrent(context)
	code := C.alcGetError(device)
	if code != C.ALC_NO_ERROR {
		err = fmt.Errorf("alcMakeContextCurrent() %s", getALErrorString(code))
		return
	}
	go poll()

	return
}

func getALErrorString(code C.ALCenum) string {
	switch code {
	case C.AL_NO_ERROR:
		return fmt.Sprintf("%d AL_NO_ERROR", code)
	case C.AL_INVALID_NAME:
		return fmt.Sprintf("%d AL_INVALID_NAME", code)
	case C.AL_INVALID_ENUM:
		return fmt.Sprintf("%d AL_INVALID_ENUM", code)
	case C.AL_INVALID_VALUE:
		return fmt.Sprintf("%d AL_INVALID_VALUE", code)
	case C.AL_INVALID_OPERATION:
		return fmt.Sprintf("%d AL_INVALID_OPERATION", code)
	case C.AL_OUT_OF_MEMORY:
		return fmt.Sprintf("%d AL_OUT_OF_MEMORY", code)
	default:
		return fmt.Sprintf("%d unkown OpenAL error", code)
	}
}

func deinit(dummy *int) {
	if context != nil {
		C.alcDestroyContext(context)
		context = nil
	}

	if device != nil {
		C.alcCloseDevice(device)
		device = nil
	}
}

// IsAvailable reports whether audio available
func IsAvailable() bool {
	return context != nil && device != nil
}

type pending struct {
	sound wav.Reader
	gain  float32
	loop  int
}

type source struct {
	id C.ALuint // source id

	head   int8 // buffer that wait for processed
	tail   int8 // buffer that will be submit
	queued int8 // how many buffers queued

	buffers [buffersPerSlot]C.ALuint // loop buffers

	sound    wav.Reader // current sound input, copy form pending
	bpb      int        // bytes per buffer
	format   C.ALenum
	pos      int64   // for ReadAt
	loop     int     // loop count, copy from pending
	pending  pending // pending sound input
	retained bool    // not released
	active   bool    // have data to process, not idle
	term     uint32  // terminating
	ovgain   float32 // overlay gain
	gain     float32 // same as alGetSourcei(AL_GAIN)
	fade     uint32  // fade steps, msecs / msecsPollInterval
	gainFade float32 // gain delta per fade step

	playing uint32 // atomic for IsPlaying
}

func (s *source) String() string {
	return fmt.Sprint(s.sound)
}

func sourceFinalize(s *source) {
	if s.buffers[0] != 0 {
		C.alDeleteBuffers(buffersPerSlot, &s.buffers[0])
	}
	if s.id != 0 {
		C.alDeleteSources(1, &s.id)
	}
}

func (s *source) reset() {
	s.sound = nil
	s.loop = 0
	s.pending.sound = nil
	s.retained = true
	s.active = false
	s.term = 0
	s.fade = 1000 / msecsPollInterval
	s.ovgain = 1
	s.gain = 1
	s.playing = 0
}

func (s *source) init() (err error) {
	runtime.SetFinalizer(s, sourceFinalize)
	C.alGenSources(1, &s.id)
	if s.id == 0 {
		code := C.alcGetError(device)
		err = fmt.Errorf("alGenSources() %s", getALErrorString(code))
		return
	}

	C.alGenBuffers(buffersPerSlot, &s.buffers[0])
	code := C.alcGetError(device)
	if code != C.ALC_NO_ERROR {
		err = fmt.Errorf("alGenBuffers() %s", getALErrorString(code))
		return
	}
	s.reset()
	return
}

func (s *source) full() bool {
	return int(s.queued) == buffersPerSlot
}

func msecsToFrames(ms int, freq int) int {
	return ms * int(freq) / 1000
}

func framesToMsecs(f int, freq int) int {
	return f * 1000 / int(freq)
}

func alFormat(x wav.Reader) C.ALenum {
	if x == nil {
		return 0
	}
	tracks := x.NumTracks()
	if tracks <= 0 || tracks > 2 {
		return 0
	}
	t := x.SampleType()
	if t != wav.U8 && t != wav.I16 {
		return 0
	}
	if t == wav.U8 {
		if tracks == 1 {
			return C.AL_FORMAT_MONO8
		}
		return C.AL_FORMAT_STEREO8
	}
	if t == wav.I16 {
		if tracks == 1 {
			return C.AL_FORMAT_MONO16
		}
		return C.AL_FORMAT_STEREO16
	}
	return 0
}

func bytesPerBuffer(x wav.Reader) int {
	freq := x.Frequency()
	framesPerChn := msecsToFrames(msecsPerBuffer, freq)
	return framesPerChn * x.SampleType().Bits() * x.NumTracks() / 8
}

func (s *source) SetPosition([3]float32) {

}
func (s *source) SetVelocity([3]float32) {

}
func (s *source) SetDirection([3]float32) {

}

// SetOverlayGain set overlay gain of the source, normalized between 0 to 1
func (s *source) SetOverlayGain(x float32) {
	c := cmdPool.Get().(*cmd)
	c.typ = cmdOvGain
	c.src = s
	c.param = x
	cmds <- c
}

func (s *source) setGain(x float32) {
	s.gain = x
	y := C.ALfloat(s.gain * s.ovgain)
	debug.Println("alSourcefv AL_GAIN:", s.gain)
	C.alSourcefv(s.id, C.AL_GAIN, &y)
}

// IsRelative reports whether the player postion is relative to the listener
func (s *source) IsRelative() bool {
	var x C.ALint
	C.alGetSourceiv(s.id, C.AL_SOURCE_RELATIVE, &x)
	return x != 0
}

func (s *source) SetRelative(b bool) {
	var x C.ALint
	if b {
		x = 1
	}
	C.alSourceiv(s.id, C.AL_SOURCE_RELATIVE, &x)
}

func (s *source) Play(x wav.Reader, gain float32, loop int) error {
	t := x.SampleType()
	if t != wav.I16 && t != wav.U8 {
		return fmt.Errorf("unsuppored pcm wave type %s", t)
	}
	p := pendingPool.Get().(*pending)
	p.sound = x
	p.gain = gain
	p.loop = loop
	c := cmdPool.Get().(*cmd)
	c.typ = cmdQueue
	c.src = s
	c.param = p
	cmds <- c
	return nil
}

func (s *source) Terminate() {
	c := cmdPool.Get().(*cmd)
	c.typ = cmdTerm
	c.src = s
	c.param = nil
	cmds <- c
}

func (s *source) SetFadeOut(msec uint) {
	c := cmdPool.Get().(*cmd)
	c.typ = cmdFade
	c.src = s
	c.param = msec
	cmds <- c
}

func (s *source) IsPlaying() bool {
	return atomic.LoadUint32(&s.playing) != 0
}

func (s *source) Release() {
	c := cmdPool.Get().(*cmd)
	c.typ = cmdRelease
	c.src = s
	c.param = nil
	cmds <- c
}

func (s *source) read(p []byte) (n int, err error) {
	if ra, ok := s.sound.(io.ReaderAt); ok {
		n, err = ra.ReadAt(p, s.pos)
		s.pos += int64(n)
		return
	}
	return s.sound.Read(p)
}

func (s *source) ensurePlaying() error {
	var x C.ALint
	C.alGetSourceiv(s.id, C.AL_SOURCE_STATE, &x)
	if x != C.AL_PLAYING {
		C.alSourcePlay(s.id)
		code := C.alcGetError(device)
		if code != C.ALC_NO_ERROR {
			return fmt.Errorf("alSourcePlay() %s", getALErrorString(code))
		}
	}
	return nil
}

func (s *source) unqueue() error {
	if s.queued == 0 {
		return nil
	}
	var processed C.ALint
	C.alGetSourcei(s.id, C.AL_BUFFERS_PROCESSED, &processed)
	for processed > 0 {
		//	fmt.Println("unqueue:", s.head)
		C.alSourceUnqueueBuffers(s.id, 1, &s.buffers[s.head])
		code := C.alcGetError(device)
		if code != C.ALC_NO_ERROR {
			return fmt.Errorf("alSourceUnqueueBuffers() %s", getALErrorString(code))
		}
		s.queued--
		s.head = s.head + 1
		if s.head == buffersPerSlot {
			s.head = 0
		}
		processed--
	}
	return nil
}

// try submit current sound
func (s *source) trySubmit1() error {
	s.unqueue()
	if s.full() {
		//fmt.Println("s.full")
		//s.ensurePlaying()
		return nil
	}
	if s.sound == nil {
		//fmt.Println("s.sound == nil")
		return io.EOF
	}
	//freq := s.sound.Frequency()
	//framesPerChn := msecsToFrames(msecsPerBuffer, freq)
	//bytesPerBuffer := framesPerChn * s.sound.BytesPerFrame()
	data := make([]byte, s.bpb) // TODO: use pool?
	//fmt.Println("bytesPerBuffer:", bytesPerBuffer)
	for s.queued < buffersPerSlot {
		n, err := s.read(data)
		if n == 0 {
			// error or nothing happen
			return err
		}
		//fmt.Println("queue:", s.tail)
		C.alBufferData(s.buffers[s.tail], s.format, unsafe.Pointer(&data[0]), C.ALsizei(n), C.ALsizei(s.sound.Frequency()))
		code := C.alcGetError(device)
		if code != C.ALC_NO_ERROR {
			return fmt.Errorf("alBufferData() %s", getALErrorString(code))
		}
		C.alSourceQueueBuffers(s.id, 1, &s.buffers[s.tail])
		code = C.alcGetError(device)
		if code != C.ALC_NO_ERROR {
			return fmt.Errorf("alSourceQueueBuffers() %s", getALErrorString(code))
		}
		s.tail = s.tail + 1
		if s.tail == buffersPerSlot {
			s.tail = 0
		}
		s.ensurePlaying()
		s.queued++
	}
	return nil
}

// try submit current and pending sound
func (s *source) trySubmit2() error {
	if err := s.trySubmit1(); err != nil {
		if s.pending.sound == nil {
			return err
		}
		debug.Println("current:", s.pending.sound)
		s.setGain(s.pending.gain)
		s.sound = s.pending.sound
		s.loop = s.pending.loop
		s.pending.sound = nil
		s.bpb = bytesPerBuffer(s.sound)
		s.format = alFormat(s.sound)
		if err = s.trySubmit1(); err != nil {
			return err
		}
	}
	return nil
}

func (s *source) step() {
	if s.term > 0 {
		if s.term > 1 {
			s.term--
			//if (s.term & 0x03) == 0 {
			s.gain -= s.gainFade
			if s.gain < 0 {
				s.gain = 0
			}
			s.setGain(s.gain)
			// don't over submit data
			if (s.term-1)*msecsPollInterval > msecsPerSlot {
				s.trySubmit1()
			}
			return
		}
		if s.sound != nil && s.pending.sound == nil {
			s.deactivate() // TODO: wait complete
		}
		s.sound = nil

		s.setGain(0)
		if !s.retained {
			s.pending.sound = nil
			delete(active, s)
			sourcePool.Put(s)
		}
		s.term = 0
		return
	}
	if err := s.trySubmit2(); err != nil {
		if s.sound != nil {
			s.deactivate() // TODO: wait complete
			s.sound = nil
		}
	}
}

// start to play
func (s *source) activate() {
	debug.Println("activate")
	s.active = true
}

// compleleted or terminated current+pending
func (s *source) deactivate() {
	debug.Println("deactivate")
	s.active = false
}

// SetListenerPosition set the position of listener
func SetListenerPosition(p [3]float32) {
	C.alListenerfv(C.AL_POSITION, (*C.ALfloat)(&p[0]))
}

// ListenerPosition reports the position of listener
func ListenerPosition() (p [3]float32) {
	C.alGetListenerfv(C.AL_POSITION, (*C.ALfloat)(&p[0]))
	return
}

// SetListenerVelocity set the velocity of listener
func SetListenerVelocity(v [3]float32) {
	C.alListenerfv(C.AL_VELOCITY, (*C.ALfloat)(&v[0]))
}

// ListenerVelocity reports the velocity of listener
func ListenerVelocity() (v [3]float32) {
	C.alGetListenerfv(C.AL_VELOCITY, (*C.ALfloat)(&v[0]))
	return
}

// SetListenerOrient set the orientation of listener, the toward vector
func SetListenerOrient(toward [3]float32) {
	C.alListenerfv(C.AL_ORIENTATION, (*C.ALfloat)(&toward[0]))
}

// ListenerOrient reports the orientation of listener, the toward vector
func ListenerOrient() (toward [3]float32) {
	C.alGetListenerfv(C.AL_ORIENTATION, (*C.ALfloat)(&toward[0]))
	return
}

// SetMasterGain set the master gain
func SetMasterGain(gain float32) {
	C.alListenerf(C.AL_GAIN, C.ALfloat(gain))
}

// MasterGain reports the master gain
func MasterGain() float32 {
	var x C.ALfloat
	C.alGetListenerf(C.AL_GAIN, &x)
	return float32(x)
}

// Alloc a player, this function never failed.
func Alloc() (p Player) {
	s := sourcePool.Get().(*source)
	s.reset()
	c := cmdPool.Get().(*cmd)
	c.typ = cmdAlloc
	c.src = s
	c.param = nil
	cmds <- c
	return s
}

func handleCmd(cmd *cmd) {
	debug.Printf("%s\n", cmd)
	switch cmd.typ {
	case cmdAlloc:
		active[cmd.src] = true
	case cmdQueue:
		p := cmd.param.(*pending)
		cmd.src.pending = *p
		if !cmd.src.active {
			cmd.src.activate()
		}
		//p.sound = nil
		pendingPool.Put(p)
	case cmdTerm:
		cmd.src.term = cmd.src.fade + 1
		cmd.src.pending.sound = nil
	case cmdOvGain:
		cmd.src.ovgain = cmd.param.(float32)
		cmd.src.setGain(cmd.src.gain)
	case cmdFade:
		cmd.src.fade = uint32((cmd.param.(uint) + msecsPollInterval - 1) / msecsPollInterval)
		cmd.src.gainFade = cmd.src.gain / float32(cmd.src.fade+1)
	case cmdRelease:
		// TODO: what to do?
	default:
		debug.Printf("unkown command: %s\n", cmd)
	}
}

func poll() {
	C.alcMakeContextCurrent(context)
	code := C.alcGetError(device)
	if code != C.ALC_NO_ERROR {
		panic(fmt.Errorf("alcMakeContextCurrent() %s", getALErrorString(code)))
	}
	// openal 1.1 is thread safe
	for {
		for i := 0; i < 30; i++ {
			var br bool
			select {
			case cmd := <-cmds:
				handleCmd(cmd)
			default:
				br = true
			}
			if br {
				break
			}
		}
		for s := range active {
			if s.active {
				s.step()
			}
		}
		time.Sleep(msecsPollInterval * time.Millisecond) // TODO
	}
}

// WaitIdle wait for all players stopped, so we can exit program without make broken noise
func WaitIdle() {
	// TODO: implementation
	idle <- 1
	<-idle
}
