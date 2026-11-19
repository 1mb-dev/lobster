package stats

import (
	"testing"
	"time"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

// oneTo returns 1ms..n ms in ascending order.
func oneTo(n int) []time.Duration {
	v := make([]time.Duration, n)
	for i := range v {
		v[i] = ms(i + 1)
	}
	return v
}

func TestPercentile_NearestRank(t *testing.T) {
	tests := []struct {
		n    int
		p    float64
		want time.Duration
	}{
		{100, 95, ms(95)}, // the 95th value, not the 96th
		{100, 99, ms(99)},
		{100, 100, ms(100)},
		{20, 95, ms(19)},
		{10, 95, ms(10)}, // rank 9.5 rounds up to 10
		{1, 95, ms(1)},
		{3, 0, ms(1)},
	}
	for _, tt := range tests {
		if got := Percentile(oneTo(tt.n), tt.p); got != tt.want {
			t.Errorf("Percentile(1..%d ms, %g) = %v, want %v", tt.n, tt.p, got, tt.want)
		}
	}
}

func TestPercentile_Empty(t *testing.T) {
	if got := Percentile(nil, 95); got != 0 {
		t.Errorf("Percentile(nil, 95) = %v, want 0", got)
	}
}

func TestSummarize(t *testing.T) {
	values := []time.Duration{ms(30), ms(10), ms(20)}
	got := Summarize(values)
	want := Summary{Count: 3, Min: ms(10), Max: ms(30), Mean: ms(20), P95: ms(30), P99: ms(30)}
	if got != want {
		t.Errorf("Summarize = %+v, want %+v", got, want)
	}
	if values[0] != ms(30) {
		t.Error("Summarize reordered its input")
	}
	if (Summarize(nil) != Summary{}) {
		t.Error("Summarize(nil) is not the zero Summary")
	}
}
