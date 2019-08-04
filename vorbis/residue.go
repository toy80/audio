package vorbis

import "fmt"

const maxPartitions = 256

type sResidue struct {
	typ       uint32
	begin     uint32
	end       uint32
	partiSize uint32
	classify  uint32 // don't rename
	classbook uint32
	cascade   [65]uint8
	books     [65][8]int
}

func (rs *sResidue) readConfig(vb *Vorbis) bool {
	rs.typ = vb.pr.ReadBits(16)
	if debug {
		fmt.Println("  read residue config, rs.typ=" + fmt.Sprint(rs.typ))
	}
	if rs.typ > 3 {
		fmt.Println("unsupported residues type")
		return false
	}
	rs.begin = vb.pr.ReadBits(24)
	rs.end = vb.pr.ReadBits(24)
	rs.partiSize = vb.pr.ReadBits(24) + 1
	rs.classify = vb.pr.ReadBits(6) + 1
	rs.classbook = vb.pr.ReadBits(8)
	//rs.cascade = (uint8*) malloc(classifications);
	for i := uint32(0); i < rs.classify; i++ {
		var hibits uint32
		lobits := vb.pr.ReadBits(3)
		bitflag := vb.pr.ReadBits(1) != 0
		if bitflag {
			hibits = vb.pr.ReadBits(5)
		}
		rs.cascade[i] = uint8((hibits << 3) | lobits)
	}

	for i := uint32(0); i < rs.classify; i++ {
		rb := &rs.books[i]
		for j := uint8(0); j < 8; j++ {
			if (rs.cascade[i]>>j)&0x01 != 0 {
				rb[j] = int(vb.pr.ReadBits(8)) // TODO: check for overflow?
			} else {
				rb[j] = -1
			}
		}
	}
	return true
}

func (rs *sResidue) decodePartiFormat0(vb *Vorbis, _vqbook *sCodeBook, v []float32, offset uint32, n uint32) bool {
	//fmt.Println("decodePartiFormat0")
	/*
	   1   1) [step] = [n] / [codebook_dimensions]
	   2   2) iterate [i] over the range 0 ... [step]-1 {
	   4        3) vector [entTemp] = read vector from packet using current codebook in VQ context
	   5        4) iterate [j] over the range 0 ... [codebook_dimensions]-1 {
	   7             5) vector [v] element ([offset]+[i]+[j]*[step]) =
	   8           vector [v] element ([offset]+[i]+[j]*[step]) +
	   9                  vector [entTemp] element [j]
	   11           }
	   13      }
	   15    6) done
	*/
	assert(_vqbook.codeDims <= 64)
	var entTemp [64]float32
	step := n / _vqbook.codeDims
	for i := uint32(0); i < step; i++ {
		if !_vqbook.decodeVector(vb, entTemp[:_vqbook.codeDims]) {
			return false
		}
		for j := uint32(0); j < _vqbook.codeDims; j++ {
			v[offset+i+j*step] += entTemp[j]
		}
	}
	return true
}

func (rs *sResidue) decodePartiFormat1(vb *Vorbis, _vqbook *sCodeBook, v []float32, offset uint32, n uint32) bool {
	//fmt.Println("decodePartiFormat1")
	/*
		1   1) [i] = 0
		2   2) vector [entTemp] = read vector from packet using current codebook in VQ context
		3   3) iterate [j] over the range 0 ... [codebook_dimensions]-1 {
		4
		5        4) vector [v] element ([offset]+[i]) =
		6     vector [v] element ([offset]+[i]) +
		7            vector [entTemp] element [j]
		8        5) increment [i]
		9
		10      }
		11
		12    6) if( [i] is less than [n] ) continue at step 2
		13    7) done
	*/

	assert(_vqbook.codeDims <= 64)
	var entTemp [64]float32
	var i uint32
	assert(i < n)
	for i < n {
		if !_vqbook.decodeVector(vb, entTemp[:_vqbook.codeDims]) {
			return false
		}
		for j := uint32(0); j < _vqbook.codeDims; j++ {
			v[offset+i] += entTemp[j]
			i++
		}
	}

	return true
}

func (rs *sResidue) decodeFormat01(vb *Vorbis, bufChOrder []*sChannelBuf, chCount uint32, _sz uint32) bool {
	assert(rs.typ != 2)

	for i := uint32(0); i < chCount; i++ {
		v := bufChOrder[i].residue[:_sz]
		// zero out the memory, write as std for-range, maybe compiler can optimize it
		for j := range v {
			v[j] = 0
		}
	}

	// 8.6.2
	// 1
	actualSize := _sz
	// 2

	limitResidueBegin := rs.begin
	limitResidueEnd := rs.end
	if actualSize < rs.end {
		limitResidueEnd = actualSize
	}
	bytesToRead := limitResidueEnd - limitResidueBegin
	if bytesToRead == 0 {
		return true // ???
	}
	partitionCount := bytesToRead / rs.partiSize

	// 1
	classbook := &vb.codebooks[rs.classbook]
	classwordsPerCodeword := classbook.codeDims
	// 2
	// 3
	if _sz > 4096 || classwordsPerCodeword > 64 || partitionCount > maxPartitions || chCount > 64 {
		fmt.Println("internal errors")
		return false
	}

	var classifications [64][maxPartitions + 64]int
	for pass := 0; pass < 8; pass++ {
		var idPart uint32
		for idPart < partitionCount {
			// 6
			if pass == 0 {
				for ch := uint32(0); ch < chCount; ch++ {
					// 8
					if !bufChOrder[ch].floorUnused {
						// 9
						temp := classbook.decode(vb)
						// 10
						for i := int(classwordsPerCodeword) - 1; i >= 0; i-- {
							cls := temp % rs.classify
							classifications[ch][i+int(idPart)] = int(cls)
							temp = temp / rs.classify
						}
					}
				}
			}
			// 13
			for i := uint32(0); i < classwordsPerCodeword && idPart < partitionCount; i++ {
				// 14
				for ch := uint32(0); ch < chCount; ch++ {
					buf := bufChOrder[ch]
					if !buf.floorUnused {
						// 16
						vqclass := classifications[ch][idPart]
						// 17
						vqbookid := rs.books[vqclass][pass]
						// 18
						if vqbookid >= 0 {
							// 19
							vqbook := &vb.codebooks[vqbookid]
							n := rs.partiSize
							v := buf.residue[:]
							offset := limitResidueBegin + idPart*rs.partiSize
							switch rs.typ {
							case 0:
								if !rs.decodePartiFormat0(vb, vqbook, v, offset, n) {
									return false
								}
							case 1:
								if !rs.decodePartiFormat1(vb, vqbook, v, offset, n) {
									return false
								}
							}
						}
					}
					idPart++
				}
			}
		}
	}
	// 21done
	return true
}

// format 2 is like format 1, but interlacing multi-channel into single vector
func (rs *sResidue) decodeFormat2(vb *Vorbis, bufChOrder []*sChannelBuf, chCount uint32, _sz uint32) bool {
	assert(rs.typ == 2)
	needDecode := false
	for ch := uint32(0); ch < chCount; ch++ {
		if !bufChOrder[ch].floorUnused {
			needDecode = true
		}
	}

	// 8.6.2
	actualSize := _sz * chCount
	vb.requireTempBufSize(actualSize, true)

	v := vb.tempBuf[:actualSize]
	// zero out the memory, write as std for-range, hope the compiler can optimize it
	for i := range v {
		v[i] = 0
	}

	if needDecode {
		limitResidueBegin := rs.begin
		limitResidueEnd := rs.end
		if actualSize < rs.end {
			limitResidueEnd = actualSize
		}
		bytesToRead := limitResidueEnd - limitResidueBegin
		if bytesToRead == 0 {
			return true
		}
		partitionCount := bytesToRead / rs.partiSize

		// 1
		classbook := vb.codebooks[rs.classbook]
		classwordsPerCodeword := classbook.codeDims
		// 2
		if _sz > 4096 || classwordsPerCodeword > 64 || partitionCount > maxPartitions || chCount > 64 {
			fmt.Println("internal errors")
			return false
		}

		//var format2_buf[4096 * 2]float32;

		// classifications [65536] int
		var classifications [maxPartitions + 64]int
		for pass := 0; pass < 8; pass++ {
			var idPart uint32
			for idPart < partitionCount {
				// 6
				if pass == 0 {
					// 9
					temp := classbook.decode(vb)
					// 10
					for i := int(classwordsPerCodeword) - 1; i >= 0; i-- {
						cls := temp % rs.classify
						classifications[i+int(idPart)] = int(cls)
						temp = temp / rs.classify
					}
				}
				// 13
				for i := uint32(0); i < classwordsPerCodeword && idPart < partitionCount; i++ {
					// 16
					vqclass := classifications[idPart]
					// 17
					vqbookid := rs.books[vqclass][pass]
					// 18
					if vqbookid >= 0 {
						// 19
						vqbook := &vb.codebooks[vqbookid]
						n := rs.partiSize
						offset := limitResidueBegin + idPart*n
						if !rs.decodePartiFormat1(vb, vqbook, v, offset, n) {
							return false
						}
					}
					idPart++
				}
			}
		}
	}

	// post decode, de-interlacing into origin channel
	for ch := uint32(0); ch < chCount; ch++ {
		p := v[ch:]
		for i, j := uint32(0), uint32(0); j < actualSize; i, j = i+1, j+chCount {
			bufChOrder[ch].residue[i] = p[j]
		}
	}

	return true
}
