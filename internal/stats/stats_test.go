package stats

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestNearestRank(t *testing.T) {
	tests := []struct {
		p    float64
		n    int
		want int
	}{
		{95, 100, 95}, // the 95th value, not the 96th
		{99, 100, 99},
		{100, 100, 100},
		{95, 20, 19},
		{95, 10, 10}, // rank 9.5 rounds up to 10
		{95, 1, 1},
		{0, 3, 1},
	}
	for _, tt := range tests {
		if got := NearestRank(tt.p, tt.n); got != tt.want {
			t.Errorf("NearestRank(%g, %d) = %d, want %d", tt.p, tt.n, got, tt.want)
		}
	}
}

// TestQuantile_WithinBucketWidth checks every percentile of skewed samples
// against an exact sort: true <= reported < true*BucketRatio.
func TestQuantile_WithinBucketWidth(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for _, n := range []int{1, 7, 100, 1000, 50000} {
		h := NewHistogram()
		values := make([]time.Duration, n)
		for i := range values {
			// log-uniform from 50µs to 30s, the range a crawl sees
			values[i] = time.Duration(50e3 * math.Pow(6e5, rng.Float64()))
			h.Record(values[i])
		}
		slices.Sort(values)
		for _, p := range []float64{50, 90, 95, 99, 99.9, 100} {
			exact := values[NearestRank(p, n)-1]
			got := h.Quantile(p)
			if got < exact || float64(got) > float64(exact)*BucketRatio {
				t.Errorf("n=%d p%g: histogram %v, exact %v; want within one bucket (x%.4f)", n, p, got, exact, BucketRatio)
			}
		}
		s := h.Summary()
		if s.Min != values[0] || s.Max != values[n-1] || s.Count != n {
			t.Errorf("n=%d summary min/max/count = %v/%v/%d, want %v/%v/%d", n, s.Min, s.Max, s.Count, values[0], values[n-1], n)
		}
	}
}

func TestSummary_Empty(t *testing.T) {
	if (NewHistogram().Summary() != Summary{}) {
		t.Error("empty histogram Summary is not zero")
	}
	var nilHist *Histogram
	if nilHist.Count() != 0 || (nilHist.Summary() != Summary{}) {
		t.Error("nil histogram is not empty")
	}
}

func TestSummary_Exact(t *testing.T) {
	h := NewHistogram()
	for _, d := range []time.Duration{ms(30), ms(10), ms(20)} {
		h.Record(d)
	}
	s := h.Summary()
	if s.Count != 3 || s.Min != ms(10) || s.Max != ms(30) || s.Mean != ms(20) || s.P99 != ms(30) {
		t.Errorf("Summary = %+v", s)
	}
}

func TestHistogram_ConstantMemory(t *testing.T) {
	h := NewHistogram()
	before := len(h.counts)
	for i := range 100000 {
		h.Record(time.Duration(i) * time.Microsecond)
	}
	if len(h.counts) != before || h.Count() != 100000 {
		t.Errorf("buckets %d -> %d, count %d", before, len(h.counts), h.Count())
	}
}

func TestHistogram_MarshalJSON(t *testing.T) {
	h := NewHistogram()
	h.Record(time.Millisecond)
	h.Record(time.Millisecond)
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"count":2`) || !strings.Contains(string(data), `"le_us":1024`) {
		t.Errorf("JSON = %s", data)
	}
}
