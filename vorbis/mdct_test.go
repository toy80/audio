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

func inverseSlow(in []float32, out []float32, n int) {
	n2 := n / 2
	for i := 0; i < n; i++ {
		var sum float64 // must be float64 or unacceeptable error
		for k := 0; k < n2; k++ {
			sum += float64(in[k]) * math.Cos((float64(i)+0.5+float64(n2)*0.5)*(float64(k)+0.5)*math.Pi/float64(n2))
		}
		out[i] = float32(sum)
	}
}

func TestInverse(t *testing.T) {
	for _, n := range []int{16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192} {
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
			if math.Abs(float64(v-d2[i]))/math.Abs(float64(v)) > 0.01 {
				t.Logf("d1[%d] = %g\n", i, v)
				t.Logf("d2[%d] = %g\n", i, d2[i])
				t.Fatal("incorrect result, d1 != d2")
			}
		}
	}
}

/*

func BenchmarkIMDCT2048(b *testing.B) {
	const n = 2048
	const k = 100
	b.StopTimer()
	s, d := make([]float32, n), make([]float32, n)
	for i := 0; i < n; i++ {
		s[i] = rand.Float32()
	}
	var m MDCT
	m.Init(n)
	b.StartTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for i := 0; i < k; i++ {
				m.Inverse(s, d)
			}
		}
	})
	b.SetBytes(n * 4 * k)
}
*/
