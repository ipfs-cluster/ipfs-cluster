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
)

// Phi returns the φ-failure for the given value and distribution.
func phi(v float64, d []int64) float64 {
	u := mean(d)
	o := standardDeviation(d)
	if phi := -math.Log10(1 - cdf(u, o, v)); !math.IsInf(phi, 1) {
		return phi
	}
	return 0
}

// CDF returns the cumulative distribution function if the given
// normal function, for the given value.
func cdf(u, o, v float64) float64 {
	return ((1.0 / 2.0) * (1 + math.Erf((v-u)/(o*math.Sqrt2))))
}

// Mean returns the mean of the given sample.
func mean(values []int64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	var sum int64
	for _, v := range values {
		sum += v
	}

	return float64(sum) / float64(len(values))
}

// StandardDeviation returns standard deviation of the given sample.
func standardDeviation(v []int64) float64 {
	return math.Sqrt(variance(v))
}

// Variance returns variance if the given sample.
func variance(values []int64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	m := mean(values)
	var sum float64
	for _, v := range values {
		d := float64(v) - m
		sum += d * d
	}
	return sum / float64(len(values))
}
