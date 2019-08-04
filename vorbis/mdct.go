package vorbis

import (
	"fmt"
	"math"
)

const pi float32 = math.Pi

// MDCT calculator
type MDCT struct {
	N   int // 1/1
	N2  int // 1/2
	N4  int // 1/4
	N8  int // 1/8
	N43 int // 3/4
	LDN int // ld(n)
	A   []float32
	B   []float32
	C   []float32
	buf []float32
}

func cos(x float32) float32 {
	return float32(math.Cos(float64(x)))
}

func sin(x float32) float32 {
	return float32(math.Sin(float64(x)))
}

func ilog(x uint32) (y int) {
	for x != 0 {
		y++
		x >>= 1
	}
	return y
}

var reverseBytes = [256]uint8{
	0x0, 0x80, 0x40, 0xC0, 0x20, 0xA0, 0x60, 0xE0, 0x10, 0x90, 0x50, 0xD0, 0x30, 0xB0, 0x70, 0xF0,
	0x8, 0x88, 0x48, 0xC8, 0x28, 0xA8, 0x68, 0xE8, 0x18, 0x98, 0x58, 0xD8, 0x38, 0xB8, 0x78, 0xF8,
	0x4, 0x84, 0x44, 0xC4, 0x24, 0xA4, 0x64, 0xE4, 0x14, 0x94, 0x54, 0xD4, 0x34, 0xB4, 0x74, 0xF4,
	0xC, 0x8C, 0x4C, 0xCC, 0x2C, 0xAC, 0x6C, 0xEC, 0x1C, 0x9C, 0x5C, 0xDC, 0x3C, 0xBC, 0x7C, 0xFC,
	0x2, 0x82, 0x42, 0xC2, 0x22, 0xA2, 0x62, 0xE2, 0x12, 0x92, 0x52, 0xD2, 0x32, 0xB2, 0x72, 0xF2,
	0xA, 0x8A, 0x4A, 0xCA, 0x2A, 0xAA, 0x6A, 0xEA, 0x1A, 0x9A, 0x5A, 0xDA, 0x3A, 0xBA, 0x7A, 0xFA,
	0x6, 0x86, 0x46, 0xC6, 0x26, 0xA6, 0x66, 0xE6, 0x16, 0x96, 0x56, 0xD6, 0x36, 0xB6, 0x76, 0xF6,
	0xE, 0x8E, 0x4E, 0xCE, 0x2E, 0xAE, 0x6E, 0xEE, 0x1E, 0x9E, 0x5E, 0xDE, 0x3E, 0xBE, 0x7E, 0xFE,
	0x1, 0x81, 0x41, 0xC1, 0x21, 0xA1, 0x61, 0xE1, 0x11, 0x91, 0x51, 0xD1, 0x31, 0xB1, 0x71, 0xF1,
	0x9, 0x89, 0x49, 0xC9, 0x29, 0xA9, 0x69, 0xE9, 0x19, 0x99, 0x59, 0xD9, 0x39, 0xB9, 0x79, 0xF9,
	0x5, 0x85, 0x45, 0xC5, 0x25, 0xA5, 0x65, 0xE5, 0x15, 0x95, 0x55, 0xD5, 0x35, 0xB5, 0x75, 0xF5,
	0xD, 0x8D, 0x4D, 0xCD, 0x2D, 0xAD, 0x6D, 0xED, 0x1D, 0x9D, 0x5D, 0xDD, 0x3D, 0xBD, 0x7D, 0xFD,
	0x3, 0x83, 0x43, 0xC3, 0x23, 0xA3, 0x63, 0xE3, 0x13, 0x93, 0x53, 0xD3, 0x33, 0xB3, 0x73, 0xF3,
	0xB, 0x8B, 0x4B, 0xCB, 0x2B, 0xAB, 0x6B, 0xEB, 0x1B, 0x9B, 0x5B, 0xDB, 0x3B, 0xBB, 0x7B, 0xFB,
	0x7, 0x87, 0x47, 0xC7, 0x27, 0xA7, 0x67, 0xE7, 0x17, 0x97, 0x57, 0xD7, 0x37, 0xB7, 0x77, 0xF7,
	0xF, 0x8F, 0x4F, 0xCF, 0x2F, 0xAF, 0x6F, 0xEF, 0x1F, 0x9F, 0x5F, 0xDF, 0x3F, 0xBF, 0x7F, 0xFF,
}

func reverseBits(s, bits uint32) (ret uint32) {
	s <<= 32 - bits
	ret |= uint32(reverseBytes[s>>24])
	ret |= uint32(reverseBytes[(s>>16)&0xFF]) << 8
	ret |= uint32(reverseBytes[(s>>8)&0xFF]) << 16
	ret |= uint32(reverseBytes[s&0xFF]) << 24
	return ret
}

func (m *MDCT) init(_n int) {
	if (_n&-_n) != _n || _n < 16 {
		panic(fmt.Sprintf("mdct: unsupported length %d", _n))
	}

	m.N = _n
	m.N2 = _n / 2
	m.N4 = _n / 4
	m.N8 = _n / 8
	m.N43 = 3 * m.N4
	m.LDN = ilog(uint32(_n)) - 1

	m.A = make([]float32, m.N2)
	m.B = make([]float32, m.N2)
	for k := 0; k < m.N4; k++ {
		m.A[2*k] = cos(4 * float32(k) * pi / float32(m.N))
		m.A[2*k+1] = -sin(4 * float32(k) * pi / float32(m.N))
		m.B[2*k] = cos((2*float32(k) + 1) * pi / float32(m.N) / 2)
		m.B[2*k+1] = sin((2*float32(k) + 1) * pi / float32(m.N) / 2)
	}

	m.C = make([]float32, m.N4)
	for k := 0; k < m.N8; k++ {
		m.C[2*k] = cos(2 * (2*float32(k) + 1) * pi / float32(m.N))
		m.C[2*k+1] = -sin(2 * (2*float32(k) + 1) * pi / float32(m.N))
	}

	m.buf = make([]float32, m.N)
}

func (m *MDCT) inv1(u, v []float32) {
	for k2 := 0; k2 < m.N2; k2 += 2 {
		k4 := k2 << 1
		v[m.N-k4-1] = (u[k4]-u[m.N-k4-1])*m.A[k2] - (u[k4+2]-u[m.N-k4-3])*m.A[k2+1]
		v[m.N-k4-3] = (u[k4]-u[m.N-k4-1])*m.A[k2+1] + (u[k4+2]-u[m.N-k4-3])*m.A[k2]
	}
}

func (m *MDCT) inv2(v, w []float32) {
	for k4 := 0; k4 < m.N2; k4 += 4 {
		w[m.N2+3+k4] = v[m.N2+3+k4] + v[k4+3]
		w[m.N2+1+k4] = v[m.N2+1+k4] + v[k4+1]
		w[k4+3] = (v[m.N2+3+k4]-v[k4+3])*m.A[m.N2-4-k4] - (v[m.N2+1+k4]-v[k4+1])*m.A[m.N2-3-k4]
		w[k4+1] = (v[m.N2+1+k4]-v[k4+1])*m.A[m.N2-4-k4] + (v[m.N2+3+k4]-v[k4+3])*m.A[m.N2-3-k4]
	}
}

func (m *MDCT) inv4(u, v []float32) {
	for i := uint32(0); i < (uint32)(m.N8); i++ {
		j := reverseBits(i, uint32(m.LDN-3)) // TODO: can be pre-calculated
		if i == j {
			i8 := i << 3
			v[i8+1] = u[i8+1]
			v[i8+3] = u[i8+3]
			v[i8+5] = u[i8+5]
			v[i8+7] = u[i8+7]
		} else if i < j {
			i8 := i << 3
			j8 := j << 3
			v[j8+1] = u[i8+1]
			v[i8+1] = u[j8+1]
			v[j8+3] = u[i8+3]
			v[i8+3] = u[j8+3]
			v[j8+5] = u[i8+5]
			v[i8+5] = u[j8+5]
			v[j8+7] = u[i8+7]
			v[i8+7] = u[j8+7]
		}
	}
}

func (m *MDCT) inv3(w, u []float32) {
	for l := 0; l < m.LDN-3; l++ {
		k0 := m.N >> uint(l+2)
		k1 := 0x00000001 << uint(l+3)

		rn := m.N >> uint(l+4)
		s2n := 0x00000001 << uint(l+2)
		for r := 0; r < rn; r++ {
			for s2 := 0; s2 < s2n; s2 += 2 {
				n1s0 := m.N - 1 - k0*s2 - 4*r
				n3s0 := m.N - 3 - k0*s2 - 4*r
				n1s1 := n1s0 - k0
				n3s1 := n3s0 - k0
				u[n1s0] = w[n1s0] + w[n1s1]
				u[n3s0] = w[n3s0] + w[n3s1]
				u[n1s1] = (w[n1s0]-w[n1s1])*m.A[r*k1] - (w[n3s0]-w[n3s1])*m.A[r*k1+1]
				u[n3s1] = (w[n3s0]-w[n3s1])*m.A[r*k1] + (w[n1s0]-w[n1s1])*m.A[r*k1+1]
			}
		}

		if l+1 < m.LDN-3 {
			copy(w[:m.N], u[:m.N])
		}
	}
}

// inverse MDCT algorithm from the paper
// "The use of multirate filter banks for coding of high quality digital audio" 1992
// TODO: inverse MDCT is bottle neck, need optimizatiion
func (m *MDCT) inverse(x []float32) {
	Y := x
	y := x

	var u, v, w, X []float32

	// init: Y => u
	u = m.buf
	for k := 0; k < m.N2; k++ {
		u[k] = Y[k]
	}

	for k := m.N2; k < m.N; k++ {
		u[k] = -Y[m.N-k-1]
	}

	// step1
	v = x
	m.inv1(u, v)

	// step2
	w = m.buf
	m.inv2(v, w)

	// step3
	u = x
	m.inv3(w, u)

	// step4
	v = m.buf
	m.inv4(u, v)

	// step5
	w = x
	for k := 0; k < m.N2; k++ {
		w[k] = v[k*2+1]
	}

	// step6
	u = m.buf
	for k := 0; k < m.N8; k++ {
		u[m.N-1-2*k] = w[4*k]
		u[m.N-2-2*k] = w[4*k+1]
		u[m.N43-1-2*k] = w[4*k+2]
		u[m.N43-2-2*k] = w[4*k+3]
	}

	// step7
	v = x

	for k, k2 := 0, 0; k < m.N8; k, k2 = k+1, k2+2 {
		v[m.N2+k2] = (u[m.N2+k2] + u[m.N-2-k2] + m.C[k2+1]*(u[m.N2+k2]-u[m.N-2-k2]) + m.C[k2]*(u[m.N2+k2+1]+u[m.N-2-k2+1])) / 2
		v[m.N-2-k2] = (u[m.N2+k2] + u[m.N-2-k2] - m.C[k2+1]*(u[m.N2+k2]-u[m.N-2-k2]) - m.C[k2]*(u[m.N2+k2+1]+u[m.N-2-k2+1])) / 2
		v[m.N2+1+k2] = (u[m.N2+1+k2] - u[m.N-1-k2] + m.C[k2+1]*(u[m.N2+1+k2]+u[m.N-1-k2]) - m.C[k2]*(u[m.N2+k2]-u[m.N-2-k2])) / 2
		v[m.N-1-k2] = (-u[m.N2+1+k2] + u[m.N-1-k2] + m.C[k2+1]*(u[m.N2+1+k2]+u[m.N-1-k2]) - m.C[k2]*(u[m.N2+k2]-u[m.N-2-k2])) / 2
	}

	// step8
	X = m.buf
	for k, k2 := 0, 0; k < m.N4; k, k2 = k+1, k2+2 {
		X[k] = v[k2+m.N2]*m.B[k2] + v[k2+1+m.N2]*m.B[k2+1]
		X[m.N2-1-k] = v[k2+m.N2]*m.B[k2+1] - v[k2+1+m.N2]*m.B[k2]
	}

	// final X --> y
	for k := 0; k < m.N4; k++ {
		y[k] = X[k+m.N4] * 0.5
	}

	for k := m.N4; k < m.N43; k++ {
		y[k] = -X[m.N43-k-1] * 0.5
	}

	for k := m.N43; k < m.N; k++ {
		y[k] = -X[k-m.N43] * 0.5
	}
}
