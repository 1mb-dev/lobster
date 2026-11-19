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
	targetConfig     domain.PerformanceTargets
	crawlComplete    bool
	incompleteReason string
}

// New creates a new performance validator
func New(targets domain.PerformanceTargets) *Validator {
	return &Validator{
		targets:      make([]domain.PerformanceTarget, 0),
		targetConfig: targets,
	}
}

// ValidateResults judges results against each enabled (non-zero) target and
// records whether the crawl completed.
func (v *Validator) ValidateResults(results *domain.TestResults) {
	v.targets = make([]domain.PerformanceTarget, 0)
	v.crawlComplete = results.CrawlComplete
	v.incompleteReason = results.IncompleteReason

	times := make([]time.Duration, len(results.ResponseTimes))
	for i, entry := range results.ResponseTimes {
		times[i] = entry.ResponseTime
	}
	s := stats.Summarize(times)
	tc := v.targetConfig

	if tc.RequestsPerSecond > 0 {
		v.add("Requests per Second",
			fmt.Sprintf("≥ %.1f req/s", tc.RequestsPerSecond),
			fmt.Sprintf("%.1f req/s", results.RequestsPerSecond),
			results.RequestsPerSecond >= tc.RequestsPerSecond)
	}
	v.addLatency("Average Response Time", s.Mean, s.Count, tc.AvgResponseTimeMs)
	v.addLatency("95th Percentile Response Time", s.P95, s.Count, tc.P95ResponseTimeMs)
	v.addLatency("99th Percentile Response Time", s.P99, s.Count, tc.P99ResponseTimeMs)

	if tc.SuccessRate > 0 {
		target := fmt.Sprintf("≥ %.1f%%", tc.SuccessRate)
		if results.TotalRequests == 0 {
			v.add("Success Rate", target, "no requests", false)
		} else {
			rate := float64(results.SuccessfulRequests) / float64(results.TotalRequests) * 100
			v.add("Success Rate", target, fmt.Sprintf("%.2f%%", rate), rate >= tc.SuccessRate)
		}
	}
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

// Passed reports the verdict: the crawl completed and every enabled target passed.
func (v *Validator) Passed() bool {
	if !v.crawlComplete {
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

	if len(v.targets) == 0 {
		fmt.Fprintln(w, "No performance targets enabled.")
	}
	if !v.crawlComplete {
		fmt.Fprintf(w, "FAIL Crawl incomplete: %s\n", incompleteHint(v.incompleteReason))
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
		"overall_status": v.status(),
		"targets":        targetDetails,
	}
}
