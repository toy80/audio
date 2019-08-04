package vorbis

import (
	"fmt"
	"math"
)

type sCodeBook struct {
	huffmanDecoder
	id int

	codeDims    uint32 // code dimensions
	isOrdered   bool   // code length is ordered
	codeLens    []uint8
	lookupType  uint8
	valMin      float32
	valDelta    float32
	valBits     uint32
	seqp        bool // seqp
	numLookVals uint32
	muls        []uint8 // muls
}

func float32Unpack(_x uint32) float32 {
	mantissa := int(_x & 0x1fffff)
	sign := _x & 0x80000000
	exponent := (_x & 0x7fe00000) >> 21
	if sign != 0 {
		mantissa = -mantissa
	}
	return float32(mantissa) * float32(math.Pow(2.0, float64(exponent)-788))
}

func (cb *sCodeBook) constructHuffman() bool {
	if debug {
		fmt.Println(" book[" + fmt.Sprint(cb.id) + "]:" +
			" dim=" + fmt.Sprint(cb.codeDims) +
			",\tcount=" + fmt.Sprint(len(cb.codeLens)) +
			",\tVQ=" + fmt.Sprint(cb.lookupType))
	}
	if cb.codeLens == nil {
		return false
	}
	var err error
	err = cb.huffmanDecoder.constructHufman(cb.codeLens)
	if err != nil {
		return false
	}
	cb.codeLens = nil
	return true
}

func ipower(_b uint32, _e uint32) (ret uint32) {
	ret = 1
	for i := uint32(0); i < _e; i++ {
		ret *= _b
	}
	return ret
}

func lookup1Values(entries uint32, dims uint32) (ret uint32) {
	if dims == 0 {
		return 0
	}
	// max ( ret^dims <= entries)
	ret = uint32(math.Pow(float64(entries), 1.0/float64(dims)))
	if ret == 0 {
		ret = 1
	}
	for {
		if ipower(ret, dims) > entries {
			ret--
			if ret == 0 {
				break
			}
		} else if ipower(ret+1, dims) <= entries {
			ret++
		} else {
			break
		}
	}
	return ret
}

func (cb *sCodeBook) readConfig(vb *Vorbis) bool {
	//				fmt.Println("sCodeBook::readConfig(): id=" + fmt.Sprint(id));
	var buf [4]uint8
	vb.pr.ReadBytes(buf[:3])
	if buf[0] != 0x42 || buf[1] != 0x43 || buf[2] != 0x56 {
		fmt.Println(fmt.Sprintf("corrupted codebook %d", cb.id))
		return false
	}
	cb.codeDims = vb.pr.ReadBits(2 * 8)
	numCodes := vb.pr.ReadBits(3 * 8)
	cb.isOrdered = vb.pr.ReadBits(1) != 0
	cb.codeLens = make([]uint8, numCodes)
	if cb.isOrdered {
		curEntry := uint32(0)
		curLen := vb.pr.ReadBits(5) + 1
		for curEntry < numCodes {
			n := ilog(numCodes - curEntry)
			number := vb.pr.ReadBits(uint32(n))
			for i := curEntry; i < curEntry+number; i++ {
				cb.codeLens[i] = uint8(curLen)
			}
			curEntry += number
			curLen++
			if curEntry > numCodes {
				fmt.Println("extra codebook entry")
				return false
			}
		}
	} else {
		sparse := vb.pr.ReadBits(1) != 0
		if sparse {
			for curEntry := uint32(0); curEntry < numCodes; curEntry++ {
				if vb.pr.ReadBits(1) != 0 {
					cb.codeLens[curEntry] = uint8(vb.pr.ReadBits(5) + 1)
				}
			}
		} else {
			for curEntry := uint32(0); curEntry < numCodes; curEntry++ {
				cb.codeLens[curEntry] = uint8(vb.pr.ReadBits(5) + 1)
			}
		}
	}

	cb.lookupType = uint8(vb.pr.ReadBits(4))
	if cb.lookupType > 0 {
		if cb.lookupType > 2 {
			fmt.Println("unsupported lookup type")
			return false
		}
		cb.valMin = float32Unpack(vb.pr.ReadBits(32))
		cb.valDelta = float32Unpack(vb.pr.ReadBits(32))
		cb.valBits = vb.pr.ReadBits(4) + 1
		cb.seqp = vb.pr.ReadBits(1) != 0
		if cb.lookupType == 1 {
			cb.numLookVals = lookup1Values(numCodes, cb.codeDims)
		} else {
			assert(cb.lookupType == 2)
			cb.numLookVals = numCodes * cb.codeDims
		}
		cb.muls = make([]uint8, cb.numLookVals) //(uint8*) malloc(sizeof(uint8) * cb.numLookVals);
		for i := uint32(0); i < cb.numLookVals; i++ {
			cb.muls[i] = (uint8)(vb.pr.ReadBits(cb.valBits) & 0xff)
		}
	}
	//fmt.Printf("%+v\n", *cb)
	if !cb.constructHuffman() {
		fmt.Println("corrupped huffman book")
		return false
	}

	return true
}

func (cb *sCodeBook) decode(vb *Vorbis) (sym uint32) {
	return cb.decodeHuffman(vb.pr)
}

func (cb *sCodeBook) decodeVector(r *Vorbis, _vector []float32) bool {
	//defer func() {
	//fmt.Printf("codeDims=%d, valDelta=%f, valMin=%f, seqp=%v\n",
	//cb.codeDims, cb.valDelta, cb.valMin, cb.seqp)
	//fmt.Println("decodeVector:", _vector)
	//}()
	//fmt.Printf("decodeVector: cb.lookupType=%d\n", cb.lookupType)
	assert(cb.lookupType == 1 || cb.lookupType == 2)
	lookOff := cb.decode(r)
	sz := uint32(len(_vector))
	if cb.codeDims < sz {
		sz = cb.codeDims
	}
	switch cb.lookupType {
	case 1:
		var last float32
		idxDiv := uint32(1)
		for i := uint32(0); i < sz; i++ {
			mulsOff := (lookOff / idxDiv) % cb.numLookVals
			_vector[i] = float32(cb.muls[mulsOff])*cb.valDelta + cb.valMin + last
			if cb.seqp {
				last = _vector[i]
			}
			idxDiv *= cb.numLookVals
		}
	case 2:
		var last float32
		mulsOff := lookOff * cb.codeDims
		for i := uint32(0); i < sz; i++ {
			_vector[i] = float32(cb.muls[mulsOff])*cb.valDelta + cb.valMin + last
			if cb.seqp {
				last = _vector[i]
			}
			mulsOff++
		}
	default:
		fmt.Println("unkown vector lookup type")
		return false
	}
	return true
}
