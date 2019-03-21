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

	"gonum.org/v1/gonum/floats"
)

// Phi returns the φ-failure for the given value and distribution.
// Two edge cases that are dealt with in phi:
//	1. phi == math.+Inf
//  2. phi == math.NaN
//
// Edge case 1. is most certainly a failure, the value of v is is so large
// in comparison to the distribution that the cdf function returns a 1,
// which equates to a math.Log10(0) which is one of its special cases, i.e
// returns -Inf. In this case, phi() will return the math.+Inf value, as it
// will be a valid comparison against the threshold value in checker.Failed.
//
// Edge case 2. could be a failure but may not be. phi() will return NaN
// when the standard deviation of the distribution is 0, i.e. the entire
// distribution is the same number, {1,1,1,1,1}. Considering that we are
// using UnixNano timestamps this would be highly unlikely, but just in case
// phi() will return a -1 value, indicating that the caller should retry.
func phi(v float64, d []float64) float64 {
	u, o := meanStdDev(d)
	phi := -math.Log10(1 - cdf(u, o, v))
	if math.IsNaN(phi) {
		return -1
	}
	return phi
}

// CDF returns the cumulative distribution function if the given
// normal function, for the given value.
func cdf(u, o, v float64) float64 {
	return ((1.0 / 2.0) * (1 + math.Erf((v-u)/(o*math.Sqrt2))))
}

func meanStdDev(v []float64) (m, sd float64) {
	var variance float64
	m, variance = meanVariance(v)
	sd = math.Sqrt(variance)
	return
}

func meanVariance(values []float64) (m, v float64) {
	if len(values) == 0 {
		return 0.0, 0.0
	}
	m = floats.Sum(values) / float64(len(values))
	floats.AddConst(-m, values)
	floats.Mul(values, values)
	v = floats.Sum(values) / float64(len(values))
	return
}
