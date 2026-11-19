// Package reporter generates test reports in various formats (console, JSON, HTML).
package reporter

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/stats"
)

//go:embed templates/report.html
var reportTemplate string

// Reporter generates test reports in various formats
type Reporter struct {
	results *domain.TestResults
}

// StatusDistributionEntry represents a single status code distribution entry for templates.
type StatusDistributionEntry struct {
	StatusCode  int
	Count       int
	Percentage  float64
	StatusGroup string
}

// URLValidationEntry represents a URL validation result for template rendering.
type URLValidationEntry struct {
	URL           string
	StatusCode    int
	StatusGroup   string
	ResponseTime  string
	ContentLength int64
	LinksFound    int
	Depth         int
}

// SlowRequestEntry represents a slow request for template rendering.
type SlowRequestEntry struct {
	URL          string
	ResponseTime string
	StatusCode   int
	StatusGroup  string
}

// TemplateData contains all data needed for HTML template rendering.
type TemplateData struct {
	Timestamp           string
	Duration            string
	TotalRequests       int64
	SuccessfulRequests  int64
	FailedRequests      int64
	URLsDiscovered      int64
	SuccessRate         float64
	SuccessRateClass    string
	RequestsPerSecond   float64
	AverageResponseTime string
	StatusDistribution  []StatusDistributionEntry
	URLValidations      []URLValidationEntry
	SlowRequests        []SlowRequestEntry
	Errors              []domain.ErrorInfo
	P95ResponseTime     string
	P99ResponseTime     string
	LatencyBars         []LatencyBar
}

// LatencyBar is one response-time histogram bucket drawn as a CSS bar.
type LatencyBar struct {
	Label string  // the bucket's upper bound, e.g. "≤ 12.3ms"
	Count uint64  // requests in the bucket
	Width float64 // bar width, percent of the fullest bucket
}

// New creates a new report generator
func New(results *domain.TestResults) *Reporter {
	return &Reporter{results: results}
}

// GenerateHTML creates an HTML report with interactive charts
func (r *Reporter) GenerateHTML(outputPath string) error {
	tmpl := r.getHTMLTemplate()

	// Prepare template data
	data := r.prepareTemplateData()

	// Parse and execute template
	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	// Create file with secure permissions (0o600) consistent with JSON output
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("cannot create HTML report %s: %w\nCheck directory exists and has write permissions", outputPath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	err = t.Execute(file, data)
	if err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	return nil
}

// GenerateJSON creates a JSON report
func (r *Reporter) GenerateJSON(outputPath string) error {
	data, err := json.MarshalIndent(r.results, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	err = os.WriteFile(outputPath, data, 0o600)
	if err != nil {
		return fmt.Errorf("cannot write JSON report %s: %w\nCheck directory exists, has write permissions, and sufficient disk space", outputPath, err)
	}

	return nil
}

// PrintSummary prints a console summary of the results
func (r *Reporter) PrintSummary(w io.Writer) {
	fmt.Fprintf(w, "\n%s\n", strings.Repeat("=", 60))
	fmt.Fprintf(w, "RESULTS\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 60))
	fmt.Fprintf(w, "Duration:             %s\n", r.results.Duration)
	fmt.Fprintf(w, "URLs Discovered:      %d\n", r.results.URLsDiscovered)
	fmt.Fprintf(w, "Total Requests:       %d\n", r.results.TotalRequests)
	fmt.Fprintf(w, "Successful Requests:  %d\n", r.results.SuccessfulRequests)
	fmt.Fprintf(w, "Failed Requests:      %d\n", r.results.FailedRequests)
	fmt.Fprintf(w, "Average Response Time: %s\n", r.results.AverageResponseTime)
	fmt.Fprintf(w, "Min Response Time:    %s\n", r.results.MinResponseTime)
	fmt.Fprintf(w, "Max Response Time:    %s\n", r.results.MaxResponseTime)
	fmt.Fprintf(w, "p95 / p99:            %s / %s\n", r.results.P95ResponseTime, r.results.P99ResponseTime)
	fmt.Fprintf(w, "Requests/Second:      %.2f\n", r.results.RequestsPerSecond)
	fmt.Fprintf(w, "Success Rate:         %.2f%%\n", r.results.SuccessRate)
	if r.results.CrawlComplete {
		fmt.Fprintf(w, "Crawl Complete:       yes\n")
	} else {
		fmt.Fprintf(w, "Crawl Complete:       no (%s)\n", r.results.IncompleteReason)
	}
	for _, c := range []struct {
		label string
		n     int64
	}{
		{"Robots Blocked:", r.results.RobotsBlocked},
		{"URLs Dropped:", r.results.URLsDropped},
		{"Abandoned Requests:", r.results.AbandonedRequests},
	} {
		if c.n > 0 {
			fmt.Fprintf(w, "%-22s%d\n", c.label, c.n)
		}
	}

	if len(r.results.Errors) > 0 {
		fmt.Fprintf(w, "\n%s\n", strings.Repeat("-", 60))
		fmt.Fprintf(w, "ERRORS SUMMARY\n")
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
		errorCounts := make(map[string]int)
		for _, err := range r.results.Errors {
			errorCounts[err.Error]++
		}
		for errorMsg, count := range errorCounts {
			fmt.Fprintf(w, "  %s: %d occurrence(s)\n", errorMsg, count)
		}
	}

	if len(r.results.SlowRequests) > 0 {
		fmt.Fprintf(w, "\n%s\n", strings.Repeat("-", 60))
		fmt.Fprintf(w, "SLOWEST REQUESTS (top 10)\n")
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
		for i, req := range r.results.SlowRequests {
			if i >= 10 {
				break
			}
			fmt.Fprintf(w, "  %s: %s (HTTP %d)\n", req.URL, req.ResponseTime, req.StatusCode)
		}
	}

	fmt.Fprintf(w, "\n%s\n", strings.Repeat("-", 60))
	fmt.Fprintf(w, "URL VALIDATION SUMMARY\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
	statusCounts := make(map[int]int)
	for _, validation := range r.results.URLValidations {
		statusCounts[validation.StatusCode]++
	}

	for status, count := range statusCounts {
		fmt.Fprintf(w, "HTTP %d: %d URL(s)\n", status, count)
	}

	if l := r.results.Load; l != nil {
		printLoad(w, l)
	}
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("=", 60))
}

// prepareTemplateData prepares data for HTML template rendering
func (r *Reporter) prepareTemplateData() *TemplateData {
	// Calculate status distribution
	statusCounts := make(map[int]int)
	for _, validation := range r.results.URLValidations {
		statusCounts[validation.StatusCode]++
	}

	statusDistribution := make([]StatusDistributionEntry, 0, len(statusCounts))
	totalValidations := len(r.results.URLValidations)
	for status, count := range statusCounts {
		percentage := float64(count) / float64(totalValidations) * 100
		statusDistribution = append(statusDistribution, StatusDistributionEntry{
			StatusCode:  status,
			Count:       count,
			Percentage:  percentage,
			StatusGroup: statusGroupFromCode(status),
		})
	}

	// Sort by status code
	sort.Slice(statusDistribution, func(i, j int) bool {
		return statusDistribution[i].StatusCode < statusDistribution[j].StatusCode
	})

	// Prepare URL validations with status groups
	urlValidations := make([]URLValidationEntry, 0, len(r.results.URLValidations))
	for _, validation := range r.results.URLValidations {
		urlValidations = append(urlValidations, URLValidationEntry{
			URL:           validation.URL,
			StatusCode:    validation.StatusCode,
			StatusGroup:   statusGroupFromCode(validation.StatusCode),
			ResponseTime:  validation.ResponseTime.String(),
			ContentLength: validation.ContentLength,
			LinksFound:    validation.LinksFound,
			Depth:         validation.Depth,
		})
	}

	// Prepare slow requests
	slowRequests := make([]SlowRequestEntry, 0, len(r.results.SlowRequests))
	for _, req := range r.results.SlowRequests {
		slowRequests = append(slowRequests, SlowRequestEntry{
			URL:          req.URL,
			ResponseTime: req.ResponseTime.String(),
			StatusCode:   req.StatusCode,
			StatusGroup:  statusGroupFromCode(req.StatusCode),
		})
	}

	latencyBars := latencyBars(r.results.ResponseTimes.Buckets())

	// Determine success rate class
	successRateClass := "success-high"
	if r.results.SuccessRate < 90 {
		successRateClass = "success-medium"
	}
	if r.results.SuccessRate < 70 {
		successRateClass = "success-low"
	}

	return &TemplateData{
		Timestamp:           time.Now().Format("2006-01-02 15:04:05 MST"),
		Duration:            r.results.Duration,
		TotalRequests:       r.results.TotalRequests,
		SuccessfulRequests:  r.results.SuccessfulRequests,
		FailedRequests:      r.results.FailedRequests,
		URLsDiscovered:      r.results.URLsDiscovered,
		SuccessRate:         r.results.SuccessRate,
		SuccessRateClass:    successRateClass,
		RequestsPerSecond:   r.results.RequestsPerSecond,
		AverageResponseTime: r.results.AverageResponseTime,
		StatusDistribution:  statusDistribution,
		URLValidations:      urlValidations,
		SlowRequests:        slowRequests,
		Errors:              r.results.Errors,
		P95ResponseTime:     r.results.P95ResponseTime,
		P99ResponseTime:     r.results.P99ResponseTime,
		LatencyBars:         latencyBars,
	}
}

// latencyBars scales each bucket's bar to the fullest bucket.
func latencyBars(buckets []stats.Bucket) []LatencyBar {
	var most uint64
	for _, b := range buckets {
		most = max(most, b.Count)
	}
	bars := make([]LatencyBar, len(buckets))
	for i, b := range buckets {
		bars[i] = LatencyBar{
			Label: "≤ " + b.UpperBound.Round(10*time.Microsecond).String(),
			Count: b.Count,
			Width: float64(b.Count) / float64(most) * 100,
		}
	}
	return bars
}

// statusGroupFromCode returns the status group CSS class for a given HTTP status code.
func statusGroupFromCode(status int) string {
	switch {
	case status >= 400:
		return "400"
	case status >= 300:
		return "300"
	default:
		return "200"
	}
}

// getHTMLTemplate returns the HTML template string from embedded file
func (r *Reporter) getHTMLTemplate() string {
	return reportTemplate
}

// printLoad prints the -load phase's section of the console summary.
func printLoad(w io.Writer, l *domain.LoadResults) {
	fmt.Fprintf(w, "\n%s\n", strings.Repeat("-", 60))
	fmt.Fprintf(w, "LOAD PHASE\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
	fmt.Fprintf(w, "Duration:             %s\n", l.Duration)
	fmt.Fprintf(w, "Target Rate:          %.2f req/s\n", l.TargetRate)
	fmt.Fprintf(w, "Achieved Rate:        %.2f req/s\n", l.AchievedRate)
	fmt.Fprintf(w, "Requests Sent:        %d\n", l.Sent)
	fmt.Fprintf(w, "Successful Requests:  %d\n", l.Successful)
	fmt.Fprintf(w, "Failed Requests:      %d\n", l.Failed)
	fmt.Fprintf(w, "Late Requests:        %d\n", l.Late)
	fmt.Fprintf(w, "Unsent Requests:      %d\n", l.Unsent)
	fmt.Fprintf(w, "Success Rate:         %.2f%%\n", l.SuccessRate)
	fmt.Fprintf(w, "Average Response Time: %s\n", l.AverageResponseTime)
	fmt.Fprintf(w, "p95 / p99:            %s / %s\n", l.P95ResponseTime, l.P99ResponseTime)
	fmt.Fprintf(w, "Latency is measured from each request's scheduled send time.\n")
	if l.Aborted {
		fmt.Fprintf(w, "Aborted:              %s\n", l.AbortReason)
	}
}
