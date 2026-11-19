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
	output := filepath.Join(t.TempDir(), "results.json")
	var stdout, stderr bytes.Buffer
	if code := run(local(testSite(t, "/ok", "/missing"), "-output", output), &stdout, &stderr); code != exitFail {
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

	golden := filepath.Join("testdata", "results-keys.golden")
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
}
