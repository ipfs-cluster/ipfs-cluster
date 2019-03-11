/*
Copyright (©) 2015 Timothée Peignier <timothee.peignier@tryphon.org>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

package metrics

import (
	"math"
	"math/big"
)

// Phi returns the φ-failure for the given value and distribution.
func phi(v float64, d []int64) float64 {
	u := mean(d)
	o := standardDeviation(d)
	cdf := cdf(u, o, big.NewFloat(v))
	phi := -math.Log10(1 - cdf)
	if math.IsInf(phi, 1) {
		phi = 0
	}
	return phi
}

// CDF returns the cumulative distribution function if the given
// normal function, for the given value.
func cdf(u, o, v *big.Float) float64 {
	var a, b, c big.Float
	c.Quo(b.Sub(v, u), a.Mul(o, big.NewFloat(math.Sqrt2)))
	cf, _ := c.Float64()
	cdf := ((1.0 / 2.0) * (1 + math.Erf(cf)))
	return cdf
}

// Mean returns the mean of the given sample.
func mean(values []int64) *big.Float {
	if len(values) == 0 {
		return big.NewFloat(0.0)
	}
	var sum int64
	for _, v := range values {
		sum += v
	}
	var q big.Float
	return q.Quo(big.NewFloat(float64(sum)), big.NewFloat(float64(len(values))))
}

// StandardDeviation returns standard deviation of the given sample.
func standardDeviation(v []int64) *big.Float {
	var z big.Float
	z.Sqrt(variance(v)).Float64()
	return &z
}

// Variance returns variance if the given sample.
func variance(values []int64) *big.Float {
	if len(values) == 0 {
		return big.NewFloat(0.0)
	}
	m := mean(values)
	var sum, pwr, res big.Float
	for _, v := range values {
		d := big.NewFloat(float64(v))
		d.Sub(d, m)
		pwr.Mul(d, d)
		sum.Add(&sum, &pwr)
	}
	return res.Quo(&sum, big.NewFloat(float64(len(values))))
}
