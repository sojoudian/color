package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestPodColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		hostname string
		want     string
	}{
		{"deployment pod", "blue-796f87cc56-9dmrx", "blue"},
		{"bare name", "green", "green"},
		{"empty", "", ""},
		{"leading dash", "-x", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := podColor(tt.hostname); got != tt.want {
				t.Errorf("podColor(%q) = %q, want %q", tt.hostname, got, tt.want)
			}
		})
	}
}

func TestCSSColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"keyword", "blue", "blue"},
		{"unknown keyword passes", "test", "test"},
		{"empty", "", ""},
		{"injection blocked", `red;background:url(//evil)`, ""},
		{"markup blocked", `</span><script>`, ""},
		{"hyphenated namespace dropped", "kube-system", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := string(cssColor(tt.in)); got != tt.want {
				t.Errorf("cssColor(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		cfg       Config
		path      string
		wantParts []string
	}{
		{
			name: "in cluster",
			cfg:  Config{Hostname: "blue-796f87cc56-9dmrx", Namespace: "default"},
			path: "/",
			wantParts: []string{
				"\U0001F535This is pod default/blue-796f87cc56-9dmrx on " + runtime.GOOS + "/" + runtime.GOARCH + ", serving / for ",
				`background: blue;`,
				`background: default;`,
			},
		},
		{
			name: "outside cluster",
			cfg:  Config{Hostname: "laptop"},
			path: "/some/path?x=1",
			wantParts: []string{
				"This is laptop on " + runtime.GOOS + "/" + runtime.GOARCH + ", serving /some/path?x=1 for ",
			},
		},
		{
			name: "green deployment emoji",
			cfg:  Config{Hostname: "green-abc-def", Namespace: "apps"},
			path: "/",
			wantParts: []string{
				"\U0001F7E2This is pod apps/green-abc-def",
				`background: green;`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := New(tt.cfg, discard())
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			body := rec.Body.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(body, part) {
					t.Errorf("body missing %q\nbody: %s", part, body)
				}
			}
		})
	}
}

// TestPageByteCompatibility locks the exact page bytes to the original
// jpetazzo/color output, including the \r line terminators and the \n after
// the sentence. httptest fixes RemoteAddr to 192.0.2.1:1234, which makes the
// body fully deterministic.
func TestPageByteCompatibility(t *testing.T) {
	t.Parallel()
	h := New(Config{Hostname: "blue-796f87cc56-9dmrx", Namespace: "default"}, discard())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	want := `<!DOCTYPE html>` + "\r" +
		`<html>` + "\r" +
		`<body style="background: default; text-align: center;">` + "\r" +
		`<div style="padding: 4em;"></div>` + "\r" +
		`<span style="padding: 4em; background: blue;">` + "\r" +
		`<span style="padding: 2px; background: white;">` + "\r" +
		"\U0001F535This is pod default/blue-796f87cc56-9dmrx on " +
		runtime.GOOS + "/" + runtime.GOARCH + ", serving / for 192.0.2.1:1234.\n" +
		`</span>` + "\r" +
		`</span>` + "\r" +
		`</body>` + "\r" +
		`</html>` + "\r"
	if got := rec.Body.String(); got != want {
		t.Errorf("page bytes diverged from the original format\ngot:  %q\nwant: %q", got, want)
	}
}

func TestPageEscapesHostileIdentity(t *testing.T) {
	t.Parallel()
	h := New(Config{
		Hostname:  `<script>alert(1)</script>-x`,
		Namespace: `";background:url(//evil)`,
	}, discard())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Errorf("body contains unescaped script tag: %s", body)
	}
	// Both hostile values must be dropped from the CSS contexts entirely;
	// they may only appear HTML-escaped inside the text content.
	for _, want := range []string{
		`<body style="background: ; text-align: center;">`,
		`<span style="padding: 4em; background: ;">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing sanitized style %q\nbody: %s", want, body)
		}
	}
}

func TestProbes(t *testing.T) {
	t.Parallel()
	h := New(Config{Hostname: "blue-x-y"}, discard())
	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
			t.Errorf("GET %s body = %q, want %q", path, got, "ok")
		}
	}
}

func TestServerHeader(t *testing.T) {
	t.Parallel()
	h := New(Config{Hostname: "blue-x-y", Version: "1.2.3"}, discard())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Server"); got != "color/1.2.3" {
		t.Errorf("Server header = %q, want %q", got, "color/1.2.3")
	}
}

func TestAnyMethodOnRoot(t *testing.T) {
	t.Parallel()
	h := New(Config{Hostname: "blue-x-y"}, discard())
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("POST / = %d, want %d", rec.Code, http.StatusOK)
	}
	if _, err := io.ReadAll(rec.Body); err != nil {
		t.Fatal(err)
	}
}
