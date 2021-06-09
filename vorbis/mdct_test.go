package vorbis

import (
	"math"
	"math/rand"
	"testing"
)

func TestReverseBits(t *testing.T) {
	cases := [][3]uint32{
		{32, 0, 0},
		{32, 0x10101010, 0x08080808},
		{31, 0xFFFFFFFF, 0x7FFFFFFF},
		{30, 0xFFFFFFFF, 0x3FFFFFFF},
		{25, 0xFFFFFFFF, 0x01FFFFFF},
		{8, 0x10101010, 0x00000008},
	}
	for _, v := range cases {
		if reverseBits(v[1], v[0]) != v[2] {
			t.Fatalf("reverseBits(0x%X, %d) != 0x%X\n", v[1], v[0], v[2])
		}
	}
}

func TestInverse(t *testing.T) {
	// 我们只测到8192, 再高用不到, 而且计算太慢, 累积误差也大
	for n := 1; n <= 8192; n = n << 1 {
		s, d1, d2 := make([]float32, n), make([]float32, n), make([]float32, n)
		for i := 0; i < n; i++ {
			s[i] = rand.Float32()
		}
		var m MDCT
		m.init(n)
		copy(d2, s)
		m.inverse(d2)
		inverseSlow(s, d1, n)
		for i, v := range d1 {
			if math.Abs(float64(v-d2[i])) > 0.0001 {
				t.Logf("d1[%d] = %g\n", i, v)
				t.Logf("d2[%d] = %g\n", i, d2[i])
				t.Fatal("incorrect result, d1 != d2")
			}
		}
	}
}

func benchmarkIMDCT(b *testing.B, n int) {
	b.StopTimer()
	s, d := make([]float32, n), make([]float32, n)
	for i := 0; i < n; i++ {
		s[i] = rand.Float32()
	}
	var m MDCT
	m.init(n)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		copy(d[:], s[:]) // 每次都用原始数据计算, 不迭代
		m.inverse(d)
	}
	b.SetBytes(int64(n * 4))
}

func BenchmarkIMDCT32(b *testing.B) {
	benchmarkIMDCT(b, 32)
}

func BenchmarkIMDCT512(b *testing.B) {
	benchmarkIMDCT(b, 512)
}

func BenchmarkIMDCT2048(b *testing.B) {
	benchmarkIMDCT(b, 2048)
}

func BenchmarkIMDCT8192(b *testing.B) {
	benchmarkIMDCT(b, 8192)
}
