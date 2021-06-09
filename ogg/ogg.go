package ogg

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/toy80/debug"
)

var (
	// ErrEndOfPacket indicates reach packet edge
	ErrEndOfPacket = errors.New("ogg: end of packet")

	// ErrCorrupted indicates bad ogg format or data corrupted
	ErrCorrupted = errors.New("ogg: corrupted")
)

// Reader for ogg stream
// see: https://xiph.org/vorbis/doc/framing.html
//
// basic structure of ogg is like this:
// --------------page-------------|-----page------|----page-------
// segment|segment.segment.segment.segment|segment.segment.segment
// =======|============ packet ===========|======== packet =======
//
// for vorbis decode, we read the ogg file, decode into packets, pass the packet
// to vorbis decoder bit by bit.
type Reader struct {
	r      io.Reader // upstream reader
	closer io.Closer // for Close

	// page info, order is not critical
	flags    uint8      // 5
	granule  uint64     // 6 ~13
	stream   uint32     // 14~17   stream S/N, not important to us, only 1 stream per file.
	pagesn   uint32     // 18 ~ 21 page S/N
	checksum uint32     // 22 ~ 25 page checksum, currentlly just skip verify
	numSegs  uint8      // 26      segments count
	tabSegs  [255]uint8 // 27 ~    segments table
	// end of page info

	idxSeg    int
	lenSeg    int // bytes of current segment
	idxPage   int
	idxPacket int

	// no more data in current packet, readPacketBits will return zero,
	// until switch to next packet.
	endOfPacket bool

	// no more data in entire stream.  it is not same as page's EOS flag,
	// technically EOS page can still have valid packets.
	// endOfStream is set when failed to switch to next packet.
	// when end-of-packet ist set but not switch packet yet, the end-of-stream is not set.
	endOfStream bool

	// buffer for bits reading
	bitsbuf uint64
	numbits uint32
}

func (o *Reader) Init(r io.Reader) (err error) {
	return o.initOgg(r)
}

// Close the underlying file
func (o *Reader) Close() error {
	if o.closer != nil {
		return o.closer.Close()
	}
	o.r = nil
	return nil
}

func (o *Reader) NextPacket() (err error) {
	return o.switchNextPacket()
}

func (o *Reader) EndOfPacket() bool {
	return o.endOfPacket
}

func (o *Reader) ReadBits(bits uint32) uint32 {
	// TODO: error handling?
	return o.readPacketBits(bits)
}

func (o *Reader) ReadBytes(p []byte) {
	// TODO: error handling?
	o.readPacketBytes(p)
}

func (o *Reader) ReadString() string {
	// TODO: error handling?
	return o.readPacketString()
}

func u64(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func u32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// func u16(b []byte) uint16 {
// 	return uint16(b[0]) | uint16(b[1])<<8
// }

func (o *Reader) pageFlagCon() bool { return o.flags&0x01 == 0x01 }

func (o *Reader) pageFlagBos() bool { return o.flags&0x02 == 0x02 } // first page

func (o *Reader) pageFlagEos() bool { return o.flags&0x04 == 0x04 } // last page

func (o *Reader) initOgg(r io.Reader) (err error) {
	if c, ok := r.(io.Closer); ok {
		o.closer = c
	}

	if f, ok := r.(*os.File); ok {
		// we need bufio for a file, or the system call becomes bottle neck
		r = bufio.NewReaderSize(f, 4096)
	}

	o.r = r
	if err = o.initNextPage(); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// first page error indicates not a ogg stream
			err = ErrCorrupted
		}
		return err
	}
	return nil
}

// switchNextPacket skip to next packet
func (o *Reader) switchNextPacket() (err error) {
	o.numbits = 0
	o.bitsbuf = 0 // important
	if o.endOfStream {
		return io.EOF
	}
	defer func() {
		o.endOfPacket = err != nil
	}()

	var isPacketEdge bool
	for !isPacketEdge {
		if o.lenSeg > 0 {
			if err := o.discardInput(o.lenSeg); err != nil {
				return err
			}
			o.lenSeg = 0
		}

		isPacketEdge = o.tabSegs[o.idxSeg] < 255
		o.idxSeg++
		if o.idxSeg >= int(o.numSegs) {
			// packet span page, turn next page
			if o.pageFlagEos() {
				o.endOfStream = true
				return io.EOF
			}
			if err := o.initNextPage(); err != nil {
				o.endOfStream = true
				return err
			}
			if o.pageFlagCon() == isPacketEdge {
				o.endOfStream = true
				return io.EOF
			}
		}
		o.lenSeg = int(o.tabSegs[o.idxSeg])
	}

	//o.eop = false
	o.idxPacket++
	if debug.ON {
		exact := ">="
		n := 0
		for i := o.idxSeg; i < int(o.numSegs); i++ {
			n += int(o.tabSegs[i])
			if o.tabSegs[i] < 255 {
				exact = "=="
				break
			}
		}
		debug.Printf("  ogg: packet %d: %s %d bytes\n", o.idxPacket, exact, n)
	}
	return nil
}

// readInput, fill exact the buf length
func (o *Reader) readInput(b []byte) (n int, err error) {
	if len(b) == 0 {
		return
	}
	for len(b) > 0 && err == nil {
		var x int
		x, err = o.r.Read(b)
		if x != 0 {
			n += x
			b = b[x:]
		}
	}
	return n, err
}

func (o *Reader) discardInput(n int) (err error) {
	if n == 0 {
		return
	}

	// bufio.Reader has Discard method, so use it
	if d, ok := o.r.(interface {
		Discard(n int) (discarded int, err error)
	}); ok {
		var x int
		x, err = d.Discard(n)
		if err != nil {
			return
		}
		n -= x
	}

	// fallback to read and drop method
	var tmp [256]byte
	for n > 0 && err == nil {
		x := n
		if x > 255 {
			x = 255
		}
		x, err = o.r.Read(tmp[:x])
		n -= x
	}
	return
}

// discard unread data within current page, turn to next page
// func (o *Reader) turnNextPage() (err error) {
// 	defer func() {
// 		if err != nil {
// 			o.endOfPacket = true
// 			o.endOfStream = true
// 		}
// 	}()
//
// 	if o.endOfStream || o.pageFlagEos() {
// 		return io.EOF
// 	}
// 	skip := o.lenSeg
// 	for i := o.idxSeg + 1; i < int(o.numSegs); i++ {
// 		skip += int(o.tabSegs[i])
// 	}
// 	err = o.discardInput(skip)
// 	if err != nil {
// 		return err
// 	}
// 	err = o.initNextPage()
// 	return
// }

func (o *Reader) initNextPage() error {
	if o.endOfStream || o.pageFlagEos() {
		return io.EOF
	}
	var buf [27]byte
	n, err := o.readInput(buf[:])
	if n != len(buf) {
		return err
	}
	if buf[0] != 'O' || buf[1] != 'g' || buf[2] != 'g' || buf[3] != 'S' {
		return ErrCorrupted
	}
	if buf[4] != 0 {
		return fmt.Errorf("ogg: stream version %d is not supported yet", buf[4])
	}
	o.flags = buf[5]
	o.granule = u64(buf[6:])
	o.stream = u32(buf[14:])
	o.pagesn = u32(buf[18:])
	o.checksum = u32(buf[22:])
	o.numSegs = buf[26]

	if n, err := o.readInput(o.tabSegs[:o.numSegs]); n != int(o.numSegs) {
		return err
	}

	o.idxPage++
	o.idxSeg = 0
	o.lenSeg = int(o.tabSegs[0])

	if debug.ON {
		sflags := ""
		if o.pageFlagBos() {
			sflags += " (first)"
		}
		if o.pageFlagCon() {
			sflags += " (continued)"
		}
		if o.pageFlagEos() {
			sflags += " (last)"
		}

		debug.Printf("ogg: page %d: stream=%d sn=%d, granule=%d, segs=%d %s\n",
			o.idxPage, o.stream, o.pagesn, o.granule, o.numSegs, sflags)
	}

	return nil
}

// read at least 1 byte, never cross packet edge
func (o *Reader) _readPacket(_buf []byte) (n int, err error) {
	if o.endOfStream {
		return 0, io.EOF
	}
	m := len(_buf)
	for m != 0 {
		if o.lenSeg > 0 {
			// read within segment
			var bytesToRead int
			if m < o.lenSeg {
				bytesToRead = m
			} else {
				bytesToRead = o.lenSeg
			}
			var bytesRead int
			bytesRead, err = o.readInput(_buf[:bytesToRead])
			n += bytesRead
			if bytesRead != 0 {
				m -= bytesRead
				o.lenSeg -= bytesRead
				_buf = _buf[bytesRead:]
			} else {
				break
			}
		} else {
			// try next segment
			if o.tabSegs[o.idxSeg] < 255 {
				err = ErrEndOfPacket
				break // end of packet
			}

			o.idxSeg++
			if o.idxSeg >= int(o.numSegs) {
				// across page edge
				if o.pageFlagEos() {
					fmt.Println("packet cross end-of-stream.")
					err = errors.New("packet cross end-of-stream")
					break
				}
				if err = o.initNextPage(); err != nil {
					fmt.Println("packet cross page, but failed to read next page.")
					break
				}
				if !o.pageFlagCon() {
					fmt.Println("packet cross page, but next page is not mark as continuation.")
					err = errors.New("packet cross page, but next page is not mark as continuation")
					break
				}
			}
			o.lenSeg = int(o.tabSegs[o.idxSeg])
		}
	}
	return
}

// read but not drop
func (o *Reader) peekPacketBits(_n uint32) uint32 {
	// debug.Assert(_n > 0 && _n <= 32)

	if o.endOfPacket {
		// Attempting to read past the end of an encoded packet results in an ’end-of-packet’ condition.
		// End-of-packet is not to be considered an error; it is merely a state indicating that there is
		// insufficient remaining data to fulfill the desired read size.
		return 0
	}

	if o.numbits < _n {
		// read bytes into bits buffer
		//debug.Assert(!o.endOfPacket)
		room := 8 - ((o.numbits + 0x07) >> 3)
		//debug.Assert(room > 0)
		var buf [8]byte
		n, _ := o._readPacket(buf[:room])
		if n != 0 {
			tmp := u64(buf[:]) // TODO: optimize
			o.bitsbuf |= tmp << o.numbits
			o.numbits += uint32(n) << 3
		}
	}
	mask := ^(^uint64(0) << _n)
	return (uint32)(mask & o.bitsbuf)
}

func (o *Reader) dropPacketBits(bits uint32) {
	if bits > o.numbits {
		if !o.endOfPacket {
			debug.Println("end of packet")
			o.endOfPacket = true
		}
		o.bitsbuf = 0
		o.numbits = 0
	} else {
		o.bitsbuf >>= bits
		o.numbits -= bits
	}
}

func (o *Reader) readPacketBits(bits uint32) uint32 {
	if bits > 32 {
		panic("read bits > 32 is not supported")
	}
	// debug.Assert(bits <= 32)
	ret := o.peekPacketBits(bits)
	o.dropPacketBits(bits)
	return ret
}

// read bytes from bits buffer, may not byte aligned.
// this function is use for parse string, so no need to optimize.
func (o *Reader) readPacketBytes(_buf []byte) {
	// debug.Assert(len(_buf) != 0)
	num := uint32(len(_buf))
	// debug.Assert(num > 0)
	numDwords := num / 4
	remains := num & 0x00000003
	for numDwords > 0 {
		numDwords--
		n := o.readPacketBits(32)
		_buf[0] = uint8(n & 0xFF)
		_buf[1] = uint8((n >> 8) & 0xFF)
		_buf[2] = uint8((n >> 16) & 0xFF)
		_buf[3] = uint8((n >> 24) & 0xFF)
		_buf = _buf[4:]
	}
	if remains != 0 {
		n := o.readPacketBits(remains * 8)
		for i := uint32(0); i < remains; i++ {
			_buf[i] = uint8((n >> (8 * i)) & 0xFF)
		}
	}
}

func (o *Reader) readPacketString() string {
	length := o.readPacketBits(32)
	if length == 0 {
		return ""
	}
	buf := make([]byte, length)
	o.readPacketBytes(buf)
	return string(buf)
}
