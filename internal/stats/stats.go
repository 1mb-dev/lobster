// Package stats computes response-time statistics.
package stats

import (
	"slices"
	"time"
)

// Summary describes a set of response times.
type Summary struct {
	Count int
	Min   time.Duration
	Max   time.Duration
	Mean  time.Duration
	P95   time.Duration
	P99   time.Duration
}

// Summarize computes a Summary of values without modifying them. An empty
// input gives a zero Summary.
func Summarize(values []time.Duration) Summary {
	if len(values) == 0 {
		return Summary{}
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)

	var sum time.Duration
	for _, v := range sorted {
		sum += v
	}
	return Summary{
		Count: len(sorted),
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		Mean:  sum / time.Duration(len(sorted)),
		P95:   Percentile(sorted, 95),
		P99:   Percentile(sorted, 99),
	}
}

// Percentile returns the nearest-rank p-th percentile of ascending values:
// the value at rank ceil(p*n/100), counting from 1. It returns 0 for no values.
func Percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	rank := int(p * float64(n) / 100)
	if float64(rank) < p*float64(n)/100 {
		rank++
	}
	rank = max(1, min(rank, n))
	return sorted[rank-1]
}
