// Package stats computes response-time statistics from a fixed log-bucket histogram.
package stats

import (
	"encoding/json"
	"math"
	"time"
)

const (
	bucketsPerOctave = 8
	octaves          = 28 // 1µs to about 268s
	numBuckets       = bucketsPerOctave * octaves
	baseBucket       = time.Microsecond
)

// BucketRatio is the ratio between consecutive bucket bounds, and so the
// largest factor by which a reported quantile can exceed the true value.
var BucketRatio = math.Pow(2, 1.0/bucketsPerOctave)

// Summary describes a set of response times.
type Summary struct {
	Count int
	Min   time.Duration
	Max   time.Duration
	Mean  time.Duration
	P95   time.Duration
	P99   time.Duration
}

// Histogram counts durations in fixed logarithmic buckets: bucket i holds
// values in (bound(i-1), bound(i)], bound(i) = 1µs * 2^(i/8). Memory is
// constant however many values are recorded. Min, max and mean are exact.
// A Histogram is not safe for concurrent use.
type Histogram struct {
	counts [numBuckets]uint64
	n      uint64
	sum    time.Duration
	min    time.Duration
	max    time.Duration
}

// NewHistogram returns an empty histogram.
func NewHistogram() *Histogram {
	return &Histogram{}
}

// UpperBound returns bucket i's inclusive upper bound.
func UpperBound(i int) time.Duration {
	return time.Duration(float64(baseBucket) * math.Pow(2, float64(i)/bucketsPerOctave))
}

func bucketIndex(d time.Duration) int {
	if d <= baseBucket {
		return 0
	}
	i := int(math.Ceil(bucketsPerOctave * math.Log2(float64(d)/float64(baseBucket))))
	return min(i, numBuckets-1)
}

// Record adds one duration.
func (h *Histogram) Record(d time.Duration) {
	if h.n == 0 || d < h.min {
		h.min = d
	}
	if d > h.max {
		h.max = d
	}
	h.counts[bucketIndex(d)]++
	h.n++
	h.sum += d
}

// Count returns how many durations were recorded.
func (h *Histogram) Count() int {
	if h == nil {
		return 0
	}
	return int(h.n)
}

// NearestRank returns the 1-based nearest rank of the p-th percentile among n
// values: ceil(p*n/100), at least 1 and at most n.
func NearestRank(p float64, n int) int {
	exact := p * float64(n) / 100
	rank := int(exact)
	if float64(rank) < exact {
		rank++
	}
	return max(1, min(rank, n))
}

// Quantile returns the nearest-rank p-th percentile as the upper bound of the
// bucket holding that rank, clamped to the recorded maximum. It is at least
// the true value and less than BucketRatio times it. An empty histogram gives 0.
func (h *Histogram) Quantile(p float64) time.Duration {
	if h.Count() == 0 {
		return 0
	}
	rank := uint64(NearestRank(p, int(h.n)))
	var cum uint64
	for i, c := range &h.counts {
		cum += c
		if cum >= rank {
			return min(UpperBound(i), h.max)
		}
	}
	return h.max
}

// Summary returns the histogram's statistics; an empty or nil histogram gives a zero Summary.
func (h *Histogram) Summary() Summary {
	if h.Count() == 0 {
		return Summary{}
	}
	return Summary{
		Count: int(h.n),
		Min:   h.min,
		Max:   h.max,
		Mean:  h.sum / time.Duration(h.n),
		P95:   h.Quantile(95),
		P99:   h.Quantile(99),
	}
}

// Bucket is one non-empty histogram bucket.
type Bucket struct {
	UpperBound time.Duration
	Count      uint64
}

// Buckets returns the non-empty buckets in ascending order.
func (h *Histogram) Buckets() []Bucket {
	if h == nil {
		return nil
	}
	var out []Bucket
	for i, c := range &h.counts {
		if c > 0 {
			out = append(out, Bucket{UpperBound: UpperBound(i), Count: c})
		}
	}
	return out
}

// MarshalJSON writes the non-empty buckets, upper bounds in microseconds.
func (h *Histogram) MarshalJSON() ([]byte, error) {
	type bucket struct {
		UpperBoundUs float64 `json:"le_us"`
		Count        uint64  `json:"count"`
	}
	nonEmpty := h.Buckets()
	buckets := make([]bucket, 0, len(nonEmpty))
	for _, b := range nonEmpty {
		buckets = append(buckets, bucket{UpperBoundUs: math.Round(float64(b.UpperBound)/1e3*1000) / 1000, Count: b.Count})
	}
	return json.Marshal(struct {
		Count   int      `json:"count"`
		Buckets []bucket `json:"buckets"`
	}{h.Count(), buckets})
}
