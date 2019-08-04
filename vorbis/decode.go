package vorbis

import (
	"fmt"
	"io"
	"math"

	"github.com/toy80/go-al/wav"
)

type sChannelBuf struct {
	floor1Y     [9*32 + 2]int
	sizeFloor1Y int
	floorUnused bool
	floor       [4096]float32
	residue     []float32        // point to sChannelBuf.audio, swap for each packet
	audio       [2][4096]float32 // two buffers is for overlap
	pcm         [4096]float32    // final overlapped audio
}

type sMapping struct {
	submaps       uint8
	couplingSteps uint32
	magnitude     [256]uint32
	angle         [256]uint32
	mux           [256]uint8
	submapFloor   [16]uint8
	submapResidue [16]uint8
}

func (mp *sMapping) readConfig(vb *Vorbis) bool {
	typ := vb.pr.ReadBits(16)
	if debug {
		fmt.Println("  read mapping config, type=" + fmt.Sprint(typ))
	}
	if typ != 0 {
		fmt.Println("unsupported mapping type")
		return false
	}
	// i
	if vb.pr.ReadBits(1) != 0 {
		// A
		mp.submaps = uint8(vb.pr.ReadBits(4) + 1)
	} else {
		// B
		mp.submaps = 1
	}

	// ii
	if vb.pr.ReadBits(1) != 0 {
		// A
		mp.couplingSteps = vb.pr.ReadBits(8) + 1
		for j := uint32(0); j < mp.couplingSteps; j++ {
			n := uint32(ilog(uint32(vb.audioChannels - 1)))
			mp.magnitude[j] = vb.pr.ReadBits(n)
			mp.angle[j] = vb.pr.ReadBits(n)
			if mp.magnitude[j] == mp.angle[j] ||
				mp.magnitude[j] >= uint32(vb.audioChannels) ||
				mp.angle[j] >= uint32(vb.audioChannels) {
				fmt.Println("corrupted mapping info")
				return false
			}
		}
	} else {
		// B
		mp.couplingSteps = 0
	}

	// iii
	if vb.pr.ReadBits(2) != 0 {
		fmt.Println("corrupted mapping info")
		return false
	}

	// iv
	if mp.submaps > 1 {
		for j := uint8(0); j < vb.audioChannels; j++ {
			// A
			mp.mux[j] = uint8(vb.pr.ReadBits(4))
			// B
			if mp.mux[j] > mp.submaps-1 {
				fmt.Println("corrupted mapping info")
				return false
			}
		}
	}

	// v
	for j := uint8(0); j < mp.submaps; j++ {
		vb.pr.ReadBits(8)
		mp.submapFloor[j] = uint8(vb.pr.ReadBits(8))
		mp.submapResidue[j] = uint8(vb.pr.ReadBits(8))
		if mp.submapFloor[j] >= uint8(vb.numFloors) ||
			mp.submapResidue[j] >= uint8(vb.numResidues) {
			fmt.Println("corrupted mapping info")
			return false
		}
	}

	// vi
	return true
}

type sMode struct {
	blockflag     uint8
	windowtype    uint16
	transformtype uint16
	mapping       uint8
}

func (m *sMode) readConfig(vb *Vorbis) bool {
	m.blockflag = uint8(vb.pr.ReadBits(1))
	m.windowtype = uint16(vb.pr.ReadBits(16))
	m.transformtype = uint16(vb.pr.ReadBits(16))
	m.mapping = uint8(vb.pr.ReadBits(8))
	if debug {
		fmt.Println("  read mode config, blockflag=" + fmt.Sprint(m.blockflag) +
			", windowtype=" + fmt.Sprint(m.windowtype) +
			", transformtype=" + fmt.Sprint(m.transformtype) +
			", mapping=" + fmt.Sprint(m.mapping))
	}
	if m.windowtype != 0 ||
		m.transformtype != 0 ||
		uint32(m.mapping) >= vb.numMappings {
		fmt.Println("unsupported modes or corrupted")
		return false
	}
	return true
}

type sOverlap struct {
	//   |<------- w0 ------->|
	//   +========+     +=====|==+
	//   |        |\   /      |  |
	//   0       a0 \ /       |  |
	//               +        |  |
	//       0   a1 / \       |  |
	//       |    |/   \      |  |
	//       +====+     +=====+  |
	//       |<------- w1 ------>|
	s      []float32 // slope
	sw     int       // slope width
	a0     int
	w0     int
	a1     int
	w1     int
	numPcm int
}

func (ov *sOverlap) add(_o []float32, _l []float32, _r []float32) {
	copy(_o[:ov.a0], _l)
	_o = _o[ov.a0:]
	_l = _l[ov.a0:]
	_r = _r[ov.a1:]
	x0, x1 := ov.sw-1, 0
	for i := 0; i < ov.sw; i++ {
		_o[i] = _l[i]*ov.s[x0] + _r[i]*ov.s[x1]
		x0--
		x1++
	}
	_o = _o[ov.sw:]
	_r = _r[ov.sw:]
	k := ov.w1 - (ov.a1 + ov.sw)
	copy(_o[:k], _r)
}

func (vb *Vorbis) setOutputFormat(t wav.Type) error {
	if t != wav.I16 && t != wav.U8 && t != wav.F32 {
		return fmt.Errorf("vorbis: unsupported target PCM format %s", t)
	}
	vb.outTypeSize = t.Bits() / 8
	vb.outType = t
	return nil
}

// standard PCM uint8 zero at 128
func (vb *Vorbis) outputPCMUint8(pcmCount int) {
	vb.outBuf = vb.outBufRes[:pcmCount*int(vb.audioChannels)]
	k := 0
	for i := 0; i < pcmCount; i++ {
		for ch := 0; ch < int(vb.audioChannels); ch++ {
			vb.outBuf[k] = byte(vb.chnBufs[ch].pcm[i]*127 + 128)
			k++
		}
	}
}

// standard PCM signed int16
func (vb *Vorbis) outputPCMInt16(pcmCount int) {
	vb.outBuf = vb.outBufRes[:2*pcmCount*int(vb.audioChannels)]
	k := 0
	for i := 0; i < pcmCount; i++ {
		for ch := 0; ch < int(vb.audioChannels); ch++ {
			x := uint16(int16(vb.chnBufs[ch].pcm[i] * 32767))
			vb.outBuf[k] = byte(x)
			vb.outBuf[k+1] = byte(x >> 8)
			k += 2
		}
	}
}

// standard PCM float32
func (vb *Vorbis) outputPCMFloat(pcmCount int) {
	vb.outBuf = vb.outBufRes[:4*pcmCount*int(vb.audioChannels)]
	k := 0
	for i := 0; i < pcmCount; i++ {
		for ch := 0; ch < int(vb.audioChannels); ch++ {
			x := math.Float32bits(vb.chnBufs[ch].pcm[i])
			vb.outBuf[k] = byte(x)
			vb.outBuf[k+1] = byte(x >> 8)
			vb.outBuf[k+2] = byte(x >> 16)
			vb.outBuf[k+3] = byte(x >> 24)
			k += 4
		}
	}
}

func (vb *Vorbis) output(buf []byte) (n int, err error) {
	for len(buf) != 0 {

		if len(vb.outBuf) != 0 {
			n1 := len(buf)
			if n1 > len(vb.outBuf) {
				n1 = len(vb.outBuf)
			}
			copy(buf[:n1], vb.outBuf)
			buf = buf[n1:]
			vb.outBuf = vb.outBuf[n1:]
			n += n1
			if len(buf) == 0 {
				return
			}
		}

		if vb.pr.NextPacket() != nil {
			err = io.EOF
			break
		}

		// 4.3.1 packet type, mode and window decode
		// 1
		packetType := vb.pr.ReadBits(1)
		if packetType != 0 {
			fmt.Println("    skip non-audio packet. type = " + fmt.Sprint(packetType))
			continue
		}

		// 2
		bits := uint32(ilog(vb.numModes - 1))
		modeNumber := vb.pr.ReadBits(bits)
		// 3
		mode := &vb.modes[modeNumber]
		curWindowFlag := mode.blockflag

		blockSize := vb.blockSize[mode.blockflag]
		halfBlockSize := blockSize >> 1
		if mode.blockflag != 0 {
			vb.pr.ReadBits(1)
			vb.pr.ReadBits(1)
		}

		if debug {
			fmt.Printf("    decode audio block=%d,  size=%d\n", vb.idxAutoPacket, blockSize)
		}

		// 4.3.2 floor curve decode
		mapping := &vb.mappings[mode.mapping]
		for ch := uint8(0); ch < vb.audioChannels; ch++ {
			chnbuf := &vb.chnBufs[ch]
			submapNum := mapping.mux[ch]
			floorNum := mapping.submapFloor[submapNum]
			vb.floors[floorNum].decode(vb, chnbuf, int(halfBlockSize))
		}

		for i := uint32(0); i < mapping.couplingSteps; i++ {
			if vb.chnBufs[mapping.magnitude[i]].floorUnused ||
				vb.chnBufs[mapping.angle[i]].floorUnused {
				vb.chnBufs[mapping.magnitude[i]].floorUnused = true
				vb.chnBufs[mapping.angle[i]].floorUnused = true
			}
		}

		// 4.3.4 residue decode
		var bufChOrder [maxChannels]*sChannelBuf
		for i := uint8(0); i < mapping.submaps; i++ {
			// 1
			ch := uint32(0)
			// 2
			for j := uint8(0); j < vb.audioChannels; j++ {
				// a)
				submapNum := mapping.mux[i]
				if submapNum == i {
					// i
					chnbuf := &vb.chnBufs[j]
					bufChOrder[ch] = chnbuf
					chnbuf.residue = chnbuf.audio[vb.idxAutoPacket&1][:]
					// ii
					ch++
				}
			}
			// 3
			residueNum := mapping.submapResidue[i]
			// 4
			residue := &vb.residues[residueNum]
			// 5, 6, 7
			if residue.typ == 2 {
				residue.decodeFormat2(vb, bufChOrder[:], ch, halfBlockSize)
			} else {
				residue.decodeFormat01(vb, bufChOrder[:], ch, halfBlockSize)
			}
		}

		// 4.3.5 inverse coupling
		for i := int(mapping.couplingSteps) - 1; i >= 0; i-- {
			magVecIdx := mapping.magnitude[i]
			angVecIdx := mapping.angle[i]
			mag := vb.chnBufs[magVecIdx].residue[:]
			ang := vb.chnBufs[angVecIdx].residue[:]
			inverseCoupling(mag, ang, halfBlockSize)
		}

		var ov *sOverlap
		var pcmCount int
		isFirstFrame := vb.idxAutoPacket == 0

		if !isFirstFrame {
			ov = &vb.overlap[vb.prevWindowFlag][curWindowFlag]
			pcmCount = ov.numPcm
		}

		for ch := uint8(0); ch < vb.audioChannels; ch++ {
			// 4.3.6 dot product
			chnbuf := &vb.chnBufs[ch]
			dotProduct(chnbuf.residue[:], chnbuf.floor[:], halfBlockSize)
			vb.mdct[curWindowFlag].inverse(chnbuf.residue[:])
			if !isFirstFrame {
				prevHalfAudio := chnbuf.audio[1&^vb.idxAutoPacket][vb.prevBlockSize/2:]
				ov.add(chnbuf.pcm[:], prevHalfAudio, chnbuf.residue[:])
			}
		}
		if !isFirstFrame {
			switch vb.outType {
			case wav.F32:
				vb.outputPCMFloat(pcmCount)
			case wav.I16:
				vb.outputPCMInt16(pcmCount)
			case wav.U8:
				vb.outputPCMUint8(pcmCount)
			default:
				panic(nil)
			}
		}
		vb.prevWindowFlag = int(curWindowFlag)
		vb.idxAutoPacket++
		vb.prevBlockSize = blockSize
	}
	return
}

func dotProduct(_a []float32, _b []float32, _len uint32) {
	for i := uint32(0); i < _len; i++ {
		_a[i] = _a[i] * _b[i]
	}
}

func inverseCoupling(mag []float32, ang []float32, halfBlockSize uint32) {
	for i := uint32(0); i < halfBlockSize; i++ {
		m := mag[i]
		a := ang[i]
		if m > 0 {
			if a > 0 {
				ang[i] = m - a
			} else {
				ang[i] = m
				mag[i] = m + a
			}
		} else {
			if a > 0 {
				ang[i] = m + a
			} else {
				ang[i] = m
				mag[i] = m - a
			}
		}
	}
}

func (vb *Vorbis) requireTempBufSize(_size uint32, zero bool) {
	if _size > uint32(len(vb.tempBuf)) {
		old := vb.tempBuf
		vb.tempBuf = make([]float32, _size)
		if !zero {
			copy(vb.tempBuf, old)
		}
	}
}
