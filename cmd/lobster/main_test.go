package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/results-keys.golden")

// testSite serves "/" linking to each of links; "/slow" sleeps, "/missing" is 404.
func testSite(t *testing.T, links ...string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			for _, l := range links {
				_, _ = fmt.Fprintf(w, `<a href="%s">x</a>`, l)
			}
		case "/slow":
			time.Sleep(2 * time.Second)
		case "/ok":
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/"
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// local returns the flags every run against a test server needs.
func local(url string, extra ...string) []string {
	return append([]string{"-url", url, "-allow-private-ips", "-no-progress", "-rate", "50", "-duration", "10s"}, extra...)
}

func TestRun_ExitCodes(t *testing.T) {
	strict := writeFile(t, "strict.json", `{"performance_targets": {"avg_response_time_ms": 0.000001}}`)

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"passing run", local(testSite(t, "/ok")), exitPass},
		{"a 404 page", local(testSite(t, "/missing")), exitFail},
		{"incomplete crawl", local(testSite(t, "/slow"), "-duration", "500ms"), exitFail},
		{"unmeetable target", local(testSite(t, "/ok"), "-config", strict), exitFail},
		{"private base URL", []string{"-url", testSite(t), "-no-progress"}, exitError},
		{"unknown flag", []string{"-nope"}, exitError},
		{"removed flag", []string{"-dry-run"}, exitError},
		{"rate 0", local(testSite(t), "-rate", "0"), exitError},
		{"stray argument", []string{"http://localhost:3000"}, exitError},
		{"version", []string{"-version"}, exitPass},
		{"help", []string{"-help"}, exitPass},
		{"-h", []string{"-h"}, exitPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.want {
				t.Errorf("run(%v) = %d, want %d\nstdout:\n%s\nstderr:\n%s", tt.args, got, tt.want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRun_HeaderValueNotPrinted(t *testing.T) {
	const secret = "hdr-s3cr3t-9f2c"
	url := testSite(t, "/ok")

	tests := []struct {
		name string
		env  string
		args []string
	}{
		{"value from the environment", secret, local(url, "-auth-type", "header", "-auth-header", "X-Api-Key", "-verbose")},
		{"value passed on the flag", "", local(url, "-auth-type", "header", "-auth-header", "X-Api-Key:"+secret)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LOBSTER_AUTH_HEADER", tt.env)
			var stdout, stderr bytes.Buffer
			run(tt.args, &stdout, &stderr)
			if strings.Contains(stdout.String()+stderr.String(), secret) {
				t.Errorf("the header value appears in the output:\n%s\n%s", stdout.String(), stderr.String())
			}
		})
	}
}

// jsonKeys lists every object key path in v; array elements share the path "[]".
func jsonKeys(prefix string, v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			path := prefix + "." + k
			*out = append(*out, path)
			jsonKeys(path, child, out)
		}
	case []any:
		for _, child := range x {
			jsonKeys(prefix+"[]", child, out)
		}
	}
}

func TestRun_JSONKeysMatchGolden(t *testing.T) {
	tests := []struct {
		golden string
		extra  []string
	}{
		{"results-keys.golden", nil},
		{"results-load-keys.golden", []string{"-load", "300ms"}},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "results.json")
			args := local(testSite(t, "/ok", "/missing"), append([]string{"-output", output}, tt.extra...)...)
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != exitFail {
				t.Fatalf("run = %d, want %d (the 404 fails the run)\n%s", code, exitFail, stderr.String())
			}

			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			var doc any
			if err = json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			var keys []string
			jsonKeys("", doc, &keys)
			slices.Sort(keys)
			got := strings.Join(slices.Compact(keys), "\n") + "\n"

			golden := filepath.Join("testdata", tt.golden)
			if *update {
				if err = os.WriteFile(golden, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if got != string(want) {
				t.Errorf("JSON keys differ from %s (rerun with -update if the change is intended):\ngot:\n%s\nwant:\n%s", golden, got, want)
			}
		})
	}
}

func TestRun_BaseRedirectOffHostExits2(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer other.Close()
	target := strings.Replace(other.URL, "127.0.0.1", "localhost", 1) + "/"
	base := httptest.NewServer(http.RedirectHandler(target, http.StatusFound))
	defer base.Close()

	var stdout, stderr bytes.Buffer
	if got := run(local(base.URL+"/"), &stdout, &stderr); got != exitError {
		t.Fatalf("run = %d, want %d\n%s", got, exitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), target) {
		t.Errorf("stderr does not name the final URL %s:\n%s", target, stderr.String())
	}
}

func TestRun_HTMLOnlyWhenAsked(t *testing.T) {
	dir := t.TempDir()
	url := testSite(t, "/ok")
	var stdout, stderr bytes.Buffer

	if code := run(local(url, "-output", filepath.Join(dir, "x.json")), &stdout, &stderr); code != exitPass {
		t.Fatalf("run = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "x.html")); !os.IsNotExist(err) {
		t.Errorf("-output x.json also wrote x.html (stat error %v)", err)
	}

	report := filepath.Join(dir, "report.html")
	if code := run(local(url, "-html", report), &stdout, &stderr); code != exitPass {
		t.Fatalf("run = %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(report); err != nil {
		t.Errorf("-html did not write the report: %v", err)
	}

	if code := run(local(url, "-html", filepath.Join(dir, "missing", "r.html")), &stdout, &stderr); code != exitError {
		t.Errorf("unwritable -html path: run = %d, want %d", code, exitError)
	}
}

// crawlDelaySite serves one page and a robots.txt with Crawl-delay: 1; hits counts page requests.
func crawlDelaySite(t *testing.T, status int) (string, *atomic.Int64) {
	t.Helper()
	hits := new(atomic.Int64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nCrawl-delay: 1\n"))
			return
		}
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/", hits
}

func TestRun_LoadOwnershipGate(t *testing.T) {
	tests := []struct {
		name string
		flag bool
		env  string
		want int
	}{
		{"no acknowledgement", false, "", exitError},
		{"flag without env", true, "", exitError},
		{"env naming another host", true, "example.com", exitError},
		{"flag and matching env", true, "127.0.0.1", exitPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, hits := crawlDelaySite(t, http.StatusOK)
			t.Setenv("LOBSTER_I_OWN_THIS", tt.env)
			args := []string{"-url", url, "-allow-private-ips", "-no-progress", "-rate", "20", "-duration", "10s", "-load", "1s"}
			if tt.flag {
				args = append(args, "-i-own-this")
			}
			var stdout, stderr bytes.Buffer
			if got := run(args, &stdout, &stderr); got != tt.want {
				t.Fatalf("run = %d, want %d\n%s", got, tt.want, stderr.String())
			}
			if tt.want == exitError && hits.Load() != 0 {
				t.Errorf("a refused load still crawled: %d page requests", hits.Load())
			}
			if tt.want == exitPass && !strings.Contains(stdout.String(), "LOAD PHASE") {
				t.Errorf("no load section in the output:\n%s", stdout.String())
			}
		})
	}
}

func TestRun_LoadAbortExits1(t *testing.T) {
	// The crawl's one page request gets 200; every load request after it gets 503.
	var pageHits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if pageHits.Add(1) > 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	output := filepath.Join(t.TempDir(), "r.json")

	var stdout, stderr bytes.Buffer
	start := time.Now()
	if got := run(local(server.URL+"/", "-load", "5s", "-output", output), &stdout, &stderr); got != exitFail {
		t.Fatalf("run = %d, want %d\n%s", got, exitFail, stderr.String())
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("run took %v; the load should abort early", time.Since(start))
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"load_aborted": true`) {
		t.Errorf("JSON has no load_aborted: true:\n%s", data)
	}
}

// A crawl that fetched no page skips the load: the crawl's verdict decides,
// with the report printed.
func TestRun_LoadSkippedWhenNoPageResponded(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL + "/"
	server.Close()

	var stdout, stderr bytes.Buffer
	if got := run(local(url, "-load", "1s", "-ignore-robots"), &stdout, &stderr); got != exitFail {
		t.Fatalf("run = %d, want %d\n%s", got, exitFail, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Overall: FAIL") || !strings.Contains(stderr.String(), "Load skipped") {
		t.Errorf("want the crawl's FAIL report and a skip warning\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

// The load replays only pages that responded 200-399. The crawl's 404 fails
// the success-rate target, so the run exits 1 after the load.
func TestRun_LoadReplaysOnlyPagesThatResponded(t *testing.T) {
	var root, missing atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			root.Add(1)
			_, _ = w.Write([]byte(`<a href="/gone">x</a>`))
		case "/gone":
			missing.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	if got := run(local(server.URL+"/", "-load", "1s", "-ignore-robots"), &stdout, &stderr); got != exitFail {
		t.Fatalf("run = %d, want %d\n%s", got, exitFail, stderr.String())
	}
	if n := root.Load(); n < 2 {
		t.Errorf("/ requested %d times; want the crawl's request and the load's", n)
	}
	if n := missing.Load(); n != 1 {
		t.Errorf("/gone requested %d times; want once, by the crawl", n)
	}
}

func TestRun_VerboseFromConfigFile(t *testing.T) {
	config := writeFile(t, "c.json", `{"verbose": true}`)
	var stdout, stderr bytes.Buffer
	if got := run(local(testSite(t), "-config", config, "-ignore-robots"), &stdout, &stderr); got != exitPass {
		t.Fatalf("run = %d, want %d\n%s", got, exitPass, stderr.String())
	}
	if !strings.Contains(stderr.String(), `"level":"DEBUG"`) {
		t.Errorf("\"verbose\": true in the config file logged no debug lines:\n%s", stderr.String())
	}
}
