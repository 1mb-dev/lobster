package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	built := func(v string) *debug.BuildInfo { return &debug.BuildInfo{Main: debug.Module{Version: v}} }
	tests := []struct {
		name    string
		ldflags string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"make build (git describe)", "v2.0.2-26-gabc1234", built("(devel)"), true, "2.0.2-26-gabc1234"},
		{"ldflags without v", "2.1.0", nil, false, "2.1.0"},
		{"go install of a tag", "dev", built("v2.1.0"), true, "2.1.0"},
		{"local go build", "dev", built("(devel)"), true, "dev"},
		{"no build info", "dev", nil, false, "dev"},
	}
	for _, tt := range tests {
		if got := resolveVersion(tt.ldflags, tt.info, tt.ok); got != tt.want {
			t.Errorf("%s: resolveVersion = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestVersionFlag_OneV(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })
	version = "v2.0.2-26-gabc1234" // what make build injects

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-version"}, &stdout, &stderr); code != exitPass {
		t.Fatalf("run = %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "Lobster v2.0.2-26-gabc1234" {
		t.Errorf("-version printed %q, want exactly one v", got)
	}
}

func TestRun_UserAgentNamesVersion(t *testing.T) {
	var mu sync.Mutex
	var agents []string
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		agents = append(agents, r.UserAgent())
		mu.Unlock()
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	run(local(server.URL+"/"), &stdout, &stderr)

	want := "lobster/" + currentVersion()
	mu.Lock()
	defer mu.Unlock()
	if len(agents) == 0 {
		t.Fatal("no requests reached the server")
	}
	for _, ua := range agents {
		if ua != want {
			t.Errorf("User-Agent = %q, want %q", ua, want)
		}
	}
}
