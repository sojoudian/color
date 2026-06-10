// Package server implements the colorful demo HTTP handler.
//
// The page background is derived from the pod's deployment name: a pod
// created by a Deployment named "blue" has hostname "blue-<rs>-<pod>", so
// the server extracts everything before the first dash and uses it as the
// inner background color. The namespace is used as the outer background.
package server

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Config carries the pod identity the handler reports. Values are injected
// (instead of read from the environment here) to keep the handler testable.
type Config struct {
	// Hostname is the pod hostname, e.g. "blue-796f87cc56-9dmrx".
	Hostname string
	// Namespace is the Kubernetes namespace, empty when not in a cluster.
	Namespace string
	// Version is the build version reported in the Server header.
	Version string
}

// pageTemplate mirrors the original jpetazzo/color page byte-for-byte in
// spirit: outer background = namespace, inner = color from the hostname,
// one line of text identifying the serving pod.
var pageTemplate = template.Must(template.New("page").Parse(
	`<!DOCTYPE html>` + "\r" +
		`<html>` + "\r" +
		`<body style="background: {{.NamespaceColor}}; text-align: center;">` + "\r" +
		`<div style="padding: 4em;"></div>` + "\r" +
		`<span style="padding: 4em; background: {{.PodColor}};">` + "\r" +
		`<span style="padding: 2px; background: white;">` + "\r" +
		`{{.Circles}}This is {{.DisplayName}} on {{.OS}}/{{.Arch}}, serving {{.URL}} for {{.Client}}.` + "\n" +
		`</span>` + "\r" +
		`</span>` + "\r" +
		`</body>` + "\r" +
		`</html>` + "\r"))

type pageData struct {
	NamespaceColor template.CSS
	PodColor       template.CSS
	Circles        string
	DisplayName    string
	OS             string
	Arch           string
	URL            string
	Client         string
}

var circles = map[string]string{
	"red":    "\U0001F534",
	"orange": "\U0001F7E0",
	"yellow": "\U0001F7E1",
	"green":  "\U0001F7E2",
	"blue":   "\U0001F535",
	"purple": "\U0001F7E3",
	"brown":  "\U0001F7E4",
	"black":  "⚫",
	"white":  "⚪",
}

// circle returns the emoji for a known color name, or "" otherwise.
func circle(color string) string {
	return circles[color]
}

// cssColorPattern matches strings that are safe to emit as a CSS color
// value. Anything else is dropped so hostile hostnames or namespaces can
// never inject styles or script.
var cssColorPattern = regexp.MustCompile(`^[a-zA-Z]{1,32}$`)

// cssColor returns s as a CSS value when it is a plausible color keyword;
// unknown keywords are harmless (browsers ignore invalid colors), unsafe
// strings become the empty value.
func cssColor(s string) template.CSS {
	if cssColorPattern.MatchString(s) {
		return template.CSS(s) //nolint:gosec // validated against cssColorPattern above
	}
	return ""
}

// podColor extracts the color from a pod hostname: the segment before the
// first dash ("blue-796f87cc56-9dmrx" -> "blue").
func podColor(hostname string) string {
	color, _, _ := strings.Cut(hostname, "-")
	return color
}

// New returns the demo handler: "/" serves the color page for any path,
// /healthz and /readyz serve probes.
func New(cfg Config, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", probe)
	mux.HandleFunc("GET /readyz", probe)
	mux.HandleFunc("/", page(cfg, log))
	return accessLog(serverHeader(cfg.Version, mux), log)
}

func probe(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "ok")
}

func page(cfg Config, log *slog.Logger) http.HandlerFunc {
	color := podColor(cfg.Hostname)
	displayName := cfg.Hostname
	if cfg.Namespace != "" {
		displayName = "pod " + cfg.Namespace + "/" + cfg.Hostname
	}
	data := pageData{
		NamespaceColor: cssColor(cfg.Namespace),
		PodColor:       cssColor(color),
		Circles:        circle(cfg.Namespace) + circle(color),
		DisplayName:    displayName,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		d := data
		d.URL = r.URL.String()
		d.Client = r.RemoteAddr
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pageTemplate.Execute(w, d); err != nil {
			// Too late for an error status: the body is partially written.
			// Warn, not Error: this is almost always a client disconnect.
			log.WarnContext(r.Context(), "render page", "error", err)
		}
	}
}

// serverHeader advertises the build version on every response.
func serverHeader(version string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "color/"+version)
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the response code for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying writer so http.ResponseController (and the
// optional Flusher/Hijacker fast paths it brokers) keep working through the
// middleware.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// accessLog emits one structured log line per request.
func accessLog(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.InfoContext(r.Context(), "request",
			"method", r.Method,
			"url", r.URL.String(),
			"proto", r.Proto,
			"status", rec.status,
			"remote", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"duration", time.Since(start),
		)
	})
}
