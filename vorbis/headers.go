package vorbis

import (
	"fmt"
	"strings"

	"github.com/toy80/debug"
)

func isVorbis(v []byte) bool {
	return string(v) == "vorbis"
}

func (vb *Vorbis) parseIdentHeader() bool {
	debug.Println(" # packet: 0")

	var buf [7]uint8
	vb.pr.ReadBytes(buf[:])

	if buf[0] != 1 || !isVorbis(buf[1:]) {
		fmt.Println(string(buf[:]))
		fmt.Println("corrupted identification header")
		return false
	}
	vb.vorbisVersion = vb.pr.ReadBits(32)
	if vb.vorbisVersion != 0 {
		fmt.Println("unsupported vorbis version")
		return false
	}
	vb.audioChannels = uint8(vb.pr.ReadBits(8))
	if vb.audioChannels > maxChannels {
		fmt.Printf("too many channels: %d\n", vb.audioChannels)
		return false
	}
	vb.outBufRes = make([]byte, int(vb.audioChannels)*4096*vb.outTypeSize)

	//vb.blockAlign = int(vb.audioChannels * 4)
	vb.audioFrameRate = vb.pr.ReadBits(32)
	vb.maxBitrate = vb.pr.ReadBits(32)
	vb.nomBitrate = vb.pr.ReadBits(32)
	vb.minBitrate = vb.pr.ReadBits(32)
	tmp := vb.pr.ReadBits(4)
	vb.blockSize[0] = 0x00000001 << tmp
	//frame_type[0] = tmp - 5;
	tmp = vb.pr.ReadBits(4)
	vb.blockSize[1] = 0x00000001 << tmp
	//frame_type[1] = tmp - 5;
	//debug.Assert(frame_width[frame_type[0]] == blockSize[0]);
	//debug.Assert(frame_width[frame_type[1]] == blockSize[1]);
	if vb.blockSize[0] > vb.blockSize[1] || vb.blockSize[0] < 64 || vb.blockSize[1] > 8192 {
		fmt.Println(fmt.Sprintf("unsupported blocksize pair %d,%d", vb.blockSize[0], vb.blockSize[1]))
		return false
	}

	vb.pr.ReadBits(1) //framing_flag

	vb.mdct[0].init(int(vb.blockSize[0]))
	vb.mdct[1].init(int(vb.blockSize[1]))
	return true
}

func (vb *Vorbis) parseCommentsHeader() bool {
	var err error
	if err = vb.pr.NextPacket(); err != nil {
		fmt.Println("missing comments header:", err)
		return false
	}

	var buf [32]uint8
	vb.pr.ReadBytes(buf[:7])
	if buf[0] != 3 || !isVorbis(buf[1:7]) {
		fmt.Println(string(buf[1:7]))
		fmt.Println("corrupted comments header")
		return false
	}

	vb.vendor = vb.pr.ReadString()
	listCount := vb.pr.ReadBits(32)
	vb.comments = make(map[string]string)
	for j := uint32(0); j < listCount; j++ {
		s := vb.pr.ReadString()
		if pos := strings.IndexByte(s, '='); pos != -1 {
			k, v := s[:pos], s[pos+1:]
			if v0, ok := vb.comments[k]; ok {
				vb.comments[k] = v0 + "|" + v
			} else {
				vb.comments[k] = v
			}
		}

	}
	if debug.ON {
		debug.Println("vendor:", vb.vendor)
		for k, v := range vb.comments {
			debug.Println(k, "=", v)
		}
	}
	return true
}

func (vb *Vorbis) parseSetupHeader() bool {
	if vb.pr.NextPacket() != nil {
		fmt.Println("missing setup header")
		return false
	}

	var buf [32]uint8
	// setup header packet
	vb.pr.ReadBytes(buf[:7])

	if buf[0] != 5 || !isVorbis(buf[1:7]) {
		fmt.Println(string(buf[1:7]))
		fmt.Println("corrupted setup header")
		return false
	}

	// codebooks
	vb.numCodebooks = vb.pr.ReadBits(8) + 1
	vb.codebooks = make([]sCodeBook, vb.numCodebooks)
	for i := 0; i < int(vb.numCodebooks); i++ {
		vb.codebooks[i].id = i
		vb.codebooks[i].readConfig(vb)
	}

	//  time domain transforms (unused)
	tdtCount := vb.pr.ReadBits(6) + 1
	for tdtCount > 0 {
		tdtCount--
		if vb.pr.ReadBits(16) != 0 {
			fmt.Println("unsupported time domain transforms")
			return false
		}
	}

	// floors
	debug.Assert(vb.floors == nil)
	vb.numFloors = vb.pr.ReadBits(6) + 1
	if vb.numFloors != 0 {
		vb.floors = make([]sFloor, vb.numFloors)
		for i := uint32(0); i < vb.numFloors; i++ {
			vb.floors[i].readConfig(vb)
		}
	}

	// residues
	vb.numResidues = vb.pr.ReadBits(6) + 1
	debug.Assert(vb.residues == nil)
	vb.residues = make([]sResidue, vb.numResidues)
	for i := uint32(0); i < vb.numResidues; i++ {
		vb.residues[i].readConfig(vb)
	}

	// mapping
	vb.numMappings = vb.pr.ReadBits(6) + 1
	debug.Assert(vb.mappings == nil)
	vb.mappings = make([]sMapping, vb.numMappings)
	for i := uint32(0); i < vb.numMappings; i++ {
		vb.mappings[i].readConfig(vb)
	}

	// modes
	vb.numModes = vb.pr.ReadBits(6) + 1
	debug.Assert(vb.modes == nil)
	vb.modes = make([]sMode, vb.numModes)
	for i := uint32(0); i < vb.numModes; i++ {
		vb.modes[i].readConfig(vb)
	}

	framingFlag := vb.pr.ReadBits(1)
	if framingFlag == 0 {
		fmt.Println("corrupted setup header")
		return false
	}

	return true
}

func (vb *Vorbis) parseVorbisHeaders() bool {
	vb.headerReady = false
	if !vb.parseIdentHeader() {
		return false
	}

	if !vb.parseCommentsHeader() {
		return false
	}

	if !vb.parseSetupHeader() {
		return false
	}
	vb.headerReady = true
	debug.Println("vorbis: header decode complete.")
	return true
}
