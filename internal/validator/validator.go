// Package validator validates test results against performance targets.
package validator

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/stats"
)

// Verdicts reported under the overall_status key.
const (
	StatusPass = "PASS"
	StatusFail = "FAIL"
)

// Validator validates test results against performance targets
type Validator struct {
	targets          []domain.PerformanceTarget
	skipped          []string
	targetConfig     domain.PerformanceTargets
	crawlComplete    bool
	incompleteReason string
	loadAbortReason  string
	loadUnsent       int64
}

// New creates a new performance validator
func New(targets domain.PerformanceTargets) *Validator {
	return &Validator{
		targets:      make([]domain.PerformanceTarget, 0),
		targetConfig: targets,
	}
}

// ValidateResults judges results against each enabled (non-zero) target and
// records whether the crawl completed. With a load phase, the targets judge
// the load and the crawl is judged by completeness and success rate. Without
// one, the requests-per-second target is skipped: a crawl's rate is set by -rate.
func (v *Validator) ValidateResults(results *domain.TestResults) {
	v.targets = make([]domain.PerformanceTarget, 0)
	v.skipped = nil
	v.crawlComplete = results.CrawlComplete
	v.incompleteReason = results.IncompleteReason
	v.loadAbortReason = ""
	v.loadUnsent = 0
	tc := v.targetConfig

	l := results.Load
	if l == nil {
		if tc.RequestsPerSecond > 0 {
			v.skipped = append(v.skipped, "Requests per Second: judged only on a -load phase")
		}
		v.addTargets("", results.ResponseTimes.Summary(), results.TotalRequests, results.SuccessfulRequests)
		return
	}

	if l.Aborted {
		v.loadAbortReason = l.AbortReason
	}
	v.loadUnsent = l.Unsent
	v.addSuccess("Crawl Success Rate", results.TotalRequests, results.SuccessfulRequests)
	if tc.RequestsPerSecond > 0 {
		v.add("Load Requests per Second",
			fmt.Sprintf("≥ %.1f req/s", tc.RequestsPerSecond),
			fmt.Sprintf("%.1f req/s", l.AchievedRate),
			l.AchievedRate >= tc.RequestsPerSecond)
	}
	v.addTargets("Load ", l.ResponseTimes.Summary(), l.Successful+l.Failed, l.Successful)
}

// addTargets adds the latency and success-rate targets, their names prefixed.
func (v *Validator) addTargets(prefix string, s stats.Summary, total, successful int64) {
	tc := v.targetConfig
	v.addLatency(prefix+"Average Response Time", s.Mean, s.Count, tc.AvgResponseTimeMs)
	v.addLatency(prefix+"95th Percentile Response Time", s.P95, s.Count, tc.P95ResponseTimeMs)
	v.addLatency(prefix+"99th Percentile Response Time", s.P99, s.Count, tc.P99ResponseTimeMs)
	v.addSuccess(prefix+"Success Rate", total, successful)
}

// addSuccess adds a minimum success-rate target; with no requests it fails.
func (v *Validator) addSuccess(name string, total, successful int64) {
	if v.targetConfig.SuccessRate <= 0 {
		return
	}
	target := fmt.Sprintf("≥ %.1f%%", v.targetConfig.SuccessRate)
	if total == 0 {
		v.add(name, target, "no requests", false)
		return
	}
	rate := float64(successful) / float64(total) * 100
	v.add(name, target, fmt.Sprintf("%.2f%%", rate), rate >= v.targetConfig.SuccessRate)
}

// addLatency adds a maximum-latency target; with no measured requests it fails.
func (v *Validator) addLatency(name string, actual time.Duration, count int, targetMs float64) {
	if targetMs <= 0 {
		return
	}
	target := fmt.Sprintf("≤ %.1fms", targetMs)
	if count == 0 {
		v.add(name, target, "no requests", false)
		return
	}
	actualMs := float64(actual.Nanoseconds()) / 1e6
	v.add(name, target, fmt.Sprintf("%.1fms", actualMs), actualMs <= targetMs)
}

func (v *Validator) add(name, target, actual string, passed bool) {
	v.targets = append(v.targets, domain.PerformanceTarget{
		Name:        name,
		Target:      target,
		Actual:      actual,
		Description: "Target: " + target,
		Passed:      passed,
	})
}

// Passed reports the verdict: the crawl completed, a load phase (if any) was
// not aborted and sent every scheduled request, and every enabled target passed.
func (v *Validator) Passed() bool {
	if !v.crawlComplete || v.loadAbortReason != "" || v.loadUnsent > 0 {
		return false
	}
	for _, target := range v.targets {
		if !target.Passed {
			return false
		}
	}
	return true
}

func (v *Validator) status() string {
	if v.Passed() {
		return StatusPass
	}
	return StatusFail
}

func (v *Validator) targetsMet() int {
	met := 0
	for _, target := range v.targets {
		if target.Passed {
			met++
		}
	}
	return met
}

// PrintValidationReport prints one PASS or FAIL line per enabled target, the
// crawl's completeness, and the verdict.
func (v *Validator) PrintValidationReport(w io.Writer) {
	fmt.Fprintln(w, "\n"+strings.Repeat("=", 60))
	fmt.Fprintln(w, "PERFORMANCE TARGET VALIDATION")
	fmt.Fprintln(w, strings.Repeat("=", 60))

	for _, target := range v.targets {
		status := StatusFail
		if target.Passed {
			status = StatusPass
		}
		fmt.Fprintf(w, "%s %-30s %s\n", status, target.Name+":", target.Actual)
		fmt.Fprintf(w, "    %s\n", target.Description)
		fmt.Fprintln(w)
	}

	for _, skipped := range v.skipped {
		fmt.Fprintf(w, "SKIP %s\n\n", skipped)
	}
	if len(v.targets) == 0 {
		fmt.Fprintln(w, "No performance targets enabled.")
	}
	if !v.crawlComplete {
		fmt.Fprintf(w, "FAIL Crawl incomplete: %s\n", incompleteHint(v.incompleteReason))
	}
	if v.loadAbortReason != "" {
		fmt.Fprintf(w, "FAIL Load aborted: %s\n", v.loadAbortReason)
	}
	if v.loadUnsent > 0 {
		fmt.Fprintf(w, "FAIL Load fell behind: %d requests unsent\n", v.loadUnsent)
	}

	fmt.Fprintf(w, "\nOverall: %s (%d/%d targets met)\n", v.status(), v.targetsMet(), len(v.targets))
}

func incompleteHint(reason string) string {
	switch reason {
	case domain.IncompleteQueueFull:
		return "URLs were dropped from a full queue; raise -queue-size"
	case domain.IncompleteDuration:
		return "-duration ended before every page was fetched; raise -duration or -rate"
	default:
		return reason
	}
}

// GetValidationSummary returns the verdict and per-target results for the JSON report.
func (v *Validator) GetValidationSummary() map[string]interface{} {
	targetDetails := make([]map[string]interface{}, len(v.targets))
	for i, target := range v.targets {
		targetDetails[i] = map[string]interface{}{
			"name":        target.Name,
			"target":      target.Target,
			"actual":      target.Actual,
			"description": target.Description,
			"passed":      target.Passed,
		}
	}

	return map[string]interface{}{
		"targets_met":    v.targetsMet(),
		"total_targets":  len(v.targets),
		"crawl_complete": v.crawlComplete,
		"load_aborted":   v.loadAbortReason != "",
		"load_unsent":    v.loadUnsent,
		"skipped":        append([]string{}, v.skipped...),
		"overall_status": v.status(),
		"targets":        targetDetails,
	}
}
