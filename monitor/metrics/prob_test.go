package metrics

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// NOTE: Test_phi and Test_cdf contain float64 want values that are 'precise',
// they look like golden test data, they ARE NOT. They have been calculated
// using Wolfram Alpha. The following three links provide examples of calculating
// the phi value:
// - standardDeviation: https://www.wolframalpha.com/input/?i=population+standard+deviation+-2,+-4,+-4,+-4,+-5,+-5,+-7,+-9
// - mean: https://www.wolframalpha.com/input/?i=mean+-2,+-4,+-4,+-4,+-5,+-5,+-7,+-9
// - cdf: https://www.wolframalpha.com/input/?i=(((1.0+%2F+2.0)+*+(1+%2B+Erf((-4--5)%2F(2*Sqrt2)))))
// - phi: https://www.wolframalpha.com/input/?i=-log10(1+-+0.691462461274013103637704610608337739883602175554577936)
//
// Output from the each calculation needs to copy-pasted over. Look at the phi source code
// to understand where each variable should go in the cdf calculation.
func Test_phi(t *testing.T) {
	type args struct {
		v float64
		d []int64
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			"zero values",
			args{0, []int64{0}},
			math.NaN(), // won't actually be used in comparison; see math.IsNaN() def
		},
		{
			"increasing values",
			args{
				4,
				[]int64{2, 4, 4, 4, 5, 5, 7, 9},
			},
			0.160231392277849,
		},
		{
			"decreasing values",
			args{
				-4,
				[]int64{-2, -4, -4, -4, -5, -5, -7, -9},
			},
			0.5106919892652407,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phi(tt.args.v, tt.args.d)
			if got != tt.want && !math.IsNaN(got) {
				t.Errorf("phi() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cdf(t *testing.T) {
	type args struct {
		values []int64
		v      float64
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			"zero values",
			args{[]int64{0}, 0},
			math.NaN(),
		},
		{
			"increasing values",
			args{
				[]int64{2, 4, 4, 4, 5, 5, 7, 9},
				4,
			},
			0.3085375387259869,
		},
		{
			"decreasing values",
			args{
				[]int64{-2, -4, -4, -4, -5, -5, -7, -9},
				-4,
			},
			0.6914624612740131,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mean(tt.args.values)
			sd := standardDeviation(tt.args.values)
			got := cdf(m, sd, tt.args.v)
			if got != tt.want && !math.IsNaN(got) {
				t.Errorf("cdf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mean(t *testing.T) {
	type args struct {
		values []int64
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			"zero values",
			args{[]int64{}},
			0,
		},
		{
			"increasing values",
			args{[]int64{2, 4, 4, 4, 5, 5, 7, 9}},
			5,
		},
		{
			"decreasing values",
			args{[]int64{-2, -4, -4, -4, -5, -5, -7, -9}},
			-5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mean(tt.args.values); got != tt.want {
				t.Errorf("mean() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_standardDeviation(t *testing.T) {
	type args struct {
		v []int64
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			"zero values",
			args{[]int64{}},
			0,
		},
		{
			"increasing values",
			args{[]int64{2, 4, 4, 4, 5, 5, 7, 9}},
			2,
		},
		{
			"decreasing values",
			args{[]int64{-2, -4, -4, -4, -5, -5, -7, -9}},
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := standardDeviation(tt.args.v); got != tt.want {
				t.Errorf("standardDeviation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_variance(t *testing.T) {
	type args struct {
		values []int64
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			"zero values",
			args{[]int64{}},
			0,
		},
		{
			"increasing values",
			args{[]int64{2, 4, 4, 4, 5, 5, 7, 9}},
			4,
		},
		{
			"decreasing values",
			args{[]int64{-2, -4, -4, -4, -5, -5, -7, -9}},
			4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := variance(tt.args.values); got != tt.want {
				t.Errorf("variance() = %.5f, want %v", got, tt.want)
			}
		})
	}
}

func Benchmark_prob_phi(b *testing.B) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	b.Run("distribution size 10", func(b *testing.B) {
		d := makeRandSlice(10)
		v := float64(r.Int63n(25))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			phi(v, d)
		}
	})

	b.Run("distribution size 50", func(b *testing.B) {
		d := makeRandSlice(50)
		v := float64(r.Int63n(25))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			phi(v, d)
		}
	})

	b.Run("distribution size 1000", func(b *testing.B) {
		d := makeRandSlice(1000)
		v := float64(r.Int63n(25))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			phi(v, d)
		}
	})
}

func Benchmark_prob_cdf(b *testing.B) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	b.Run("distribution size 10", func(b *testing.B) {
		d := makeRandSlice(10)
		u := mean(d)
		o := standardDeviation(d)
		v := float64(r.Int63n(25))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cdf(u, o, v)
		}
	})

	b.Run("distribution size 50", func(b *testing.B) {
		d := makeRandSlice(50)
		u := mean(d)
		o := standardDeviation(d)
		v := float64(r.Int63n(25))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cdf(u, o, v)
		}
	})

	b.Run("distribution size 1000", func(b *testing.B) {
		d := makeRandSlice(1000)
		u := mean(d)
		o := standardDeviation(d)
		v := float64(r.Int63n(25))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cdf(u, o, v)
		}
	})
}

func Benchmark_prob_mean(b *testing.B) {
	b.Run("distribution size 10", func(b *testing.B) {
		d := makeRandSlice(10)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mean(d)
		}
	})

	b.Run("distribution size 50", func(b *testing.B) {
		d := makeRandSlice(50)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mean(d)
		}
	})

	b.Run("distribution size 1000", func(b *testing.B) {
		d := makeRandSlice(1000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mean(d)
		}
	})
}

func Benchmark_prob_standardDeviation(b *testing.B) {
	b.Run("distribution size 10", func(b *testing.B) {
		d := makeRandSlice(10)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			standardDeviation(d)
		}
	})

	b.Run("distribution size 50", func(b *testing.B) {
		d := makeRandSlice(50)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			standardDeviation(d)
		}
	})

	b.Run("distribution size 1000", func(b *testing.B) {
		d := makeRandSlice(1000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			standardDeviation(d)
		}
	})
}

func Benchmark_prob_variance(b *testing.B) {
	b.Run("distribution size 10", func(b *testing.B) {
		d := makeRandSlice(10)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			variance(d)
		}
	})

	b.Run("distribution size 50", func(b *testing.B) {
		d := makeRandSlice(50)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			variance(d)
		}
	})

	b.Run("distribution size 1000", func(b *testing.B) {
		d := makeRandSlice(1000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			variance(d)
		}
	})
}

func makeRandSlice(size int) []int64 {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := make([]int64, size, size)

	for i := 0; i < size-1; i++ {
		s[i] = r.Int63n(25)
	}
	return s
}

func makeRandSliceFloat64(size int) []float64 {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := make([]float64, size, size)

	for i := 0; i < size-1; i++ {
		s[i] = float64(r.Int63n(25)) + r.Float64()
	}
	return s
}
