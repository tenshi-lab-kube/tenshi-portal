package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed templates static
var content embed.FS

type Service struct {
	Name        string
	Kind        string
	Description string
	URL         string
	Status      string
	Accent      string
	Initials    string
}

type PageData struct {
	Title       string
	Subtitle    string
	Environment string
	UpdatedAt   string
	CSSVersion  string
	Services    []Service
}

type routeMetric struct {
	Method          string
	Path            string
	Status          int
	Count           uint64
	DurationSeconds float64
}

type metricsStore struct {
	startedAt time.Time
	requests  atomic.Uint64
	mu        sync.Mutex
	routes    map[string]*routeMetric
}

var metrics = &metricsStore{startedAt: time.Now(), routes: map[string]*routeMetric{}}

func main() {
	port := env("PORT", "8080")
	cssVersion := time.Now().Format("20060102150405")
	tmpl := template.Must(template.ParseFS(content, "templates/*.html"))

	services := []Service{
		{Name: "Authentik", Kind: "Identity", Description: "Single sign-on, comptes, groupes et politiques d'acces.", URL: "https://auth.tenshi-lab.fr", Status: "SSO", Accent: "teal", Initials: "AK"},
		{Name: "Grafana", Kind: "Observability", Description: "Dashboards, metriques cluster et supervision des services.", URL: "https://grafana.tenshi-lab.fr", Status: "Live", Accent: "amber", Initials: "GF"},
		{Name: "Argo CD", Kind: "GitOps", Description: "Deploiements Kubernetes, sync applicative et etat GitOps.", URL: "https://argocd.tenshi-lab.fr", Status: "GitOps", Accent: "rose", Initials: "AC"},
		{Name: "Discord Tickets", Kind: "Support", Description: "Dashboard du bot Discord, tickets, categories et transcripts.", URL: "https://tickets.tenshi-lab.fr", Status: "Prod", Accent: "violet", Initials: "DT"},
		{Name: "Observability", Kind: "Runtime", Description: "Etat local du portail, metriques HTTP et liens Grafana.", URL: "/observability", Status: "Local", Accent: "teal", Initials: "OB"},
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(content)))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/observability", observabilityHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := PageData{Title: "Tenshi Lab", Subtitle: "Console d'acces aux services du cluster Kubernetes.", Environment: env("PORTAL_ENV", "Homelab"), UpdatedAt: time.Now().Format("02 Jan 2006 15:04"), CSSVersion: cssVersion, Services: services}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("render error: %v", err)
		}
	})

	server := &http.Server{Addr: ":" + port, Handler: securityHeaders(observeRequests(mux)), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("tenshi-portal listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func observeRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		if r.URL.Path == "/metrics" {
			return
		}
		metrics.requests.Add(1)
		metrics.addRoute(r.Method, normalizePath(r.URL.Path), recorder.status, time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func normalizePath(path string) string {
	if path == "/" || path == "/healthz" || path == "/observability" {
		return path
	}
	if strings.HasPrefix(path, "/static/") {
		return "/static/*"
	}
	return "/*"
}

func (m *metricsStore) addRoute(method, path string, status int, duration float64) {
	key := fmt.Sprintf("%s %s %d", method, path, status)
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.routes[key]
	if entry == nil {
		entry = &routeMetric{Method: method, Path: path, Status: status}
		m.routes[key] = entry
	}
	entry.Count++
	entry.DurationSeconds += duration
}

func (m *metricsStore) routeSnapshot() []routeMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	routes := make([]routeMetric, 0, len(m.routes))
	for _, route := range m.routes {
		routes = append(routes, *route)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Count > routes[j].Count })
	return routes
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	uptime := time.Since(metrics.startedAt).Seconds()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# HELP tenshi_portal_up Application health status.")
	fmt.Fprintln(w, "# TYPE tenshi_portal_up gauge")
	fmt.Fprintln(w, "tenshi_portal_up 1")
	fmt.Fprintln(w, "# HELP tenshi_portal_uptime_seconds Application uptime in seconds.")
	fmt.Fprintln(w, "# TYPE tenshi_portal_uptime_seconds gauge")
	fmt.Fprintf(w, "tenshi_portal_uptime_seconds %.0f\n", uptime)
	fmt.Fprintln(w, "# HELP tenshi_portal_info Static service information.")
	fmt.Fprintln(w, "# TYPE tenshi_portal_info gauge")
	fmt.Fprintf(w, "tenshi_portal_info{service=\"tenshi-portal\",go_version=\"%s\",env=\"%s\"} 1\n", label(runtime.Version()), label(env("PORTAL_ENV", "Homelab")))
	fmt.Fprintln(w, "# HELP tenshi_portal_process_memory_bytes Process memory usage.")
	fmt.Fprintln(w, "# TYPE tenshi_portal_process_memory_bytes gauge")
	fmt.Fprintf(w, "tenshi_portal_process_memory_bytes{type=\"alloc\"} %d\n", mem.Alloc)
	fmt.Fprintf(w, "tenshi_portal_process_memory_bytes{type=\"sys\"} %d\n", mem.Sys)
	fmt.Fprintf(w, "tenshi_portal_process_memory_bytes{type=\"heap_alloc\"} %d\n", mem.HeapAlloc)
	fmt.Fprintln(w, "# HELP tenshi_portal_http_requests_total HTTP requests handled by the app.")
	fmt.Fprintln(w, "# TYPE tenshi_portal_http_requests_total counter")
	for _, route := range metrics.routeSnapshot() {
		fmt.Fprintf(w, "tenshi_portal_http_requests_total{method=\"%s\",route=\"%s\",status=\"%d\"} %d\n", label(route.Method), label(route.Path), route.Status, route.Count)
	}
	fmt.Fprintln(w, "# HELP tenshi_portal_http_request_duration_seconds_sum Total HTTP request duration.")
	fmt.Fprintln(w, "# TYPE tenshi_portal_http_request_duration_seconds_sum counter")
	for _, route := range metrics.routeSnapshot() {
		fmt.Fprintf(w, "tenshi_portal_http_request_duration_seconds_sum{method=\"%s\",route=\"%s\",status=\"%d\"} %.6f\n", label(route.Method), label(route.Path), route.Status, route.DurationSeconds)
	}
}

func observabilityHandler(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	routes := metrics.routeSnapshot()
	rows := ""
	for _, route := range routes {
		rows += fmt.Sprintf("<tr><td>%s %s</td><td>%d</td><td>%d</td><td>%.3fs</td></tr>", template.HTMLEscapeString(route.Method), template.HTMLEscapeString(route.Path), route.Status, route.Count, route.DurationSeconds)
	}
	if rows == "" {
		rows = "<tr><td colspan=\"4\" class=\"muted\">Aucune requete observee.</td></tr>"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="fr"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>Tenshi Portal Observability</title>
<style>*{box-sizing:border-box}body{margin:0;background:#101418;color:#e5e7eb;font-family:Inter,Segoe UI,system-ui,sans-serif}main{max-width:1120px;margin:0 auto;padding:32px}header{display:flex;justify-content:space-between;gap:16px;align-items:flex-start;margin-bottom:24px}h1{margin:0;font-size:30px}.muted{color:#9ca3af}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:14px}.card{background:#171d23;border:1px solid #27313a;border-radius:8px;padding:18px}.label{font-size:12px;color:#9ca3af;text-transform:uppercase;letter-spacing:.04em}.value{font-size:28px;font-weight:800;margin-top:8px}.ok{color:#34d399}table{width:100%%;border-collapse:collapse;margin-top:10px}td,th{border-bottom:1px solid #27313a;padding:10px;text-align:left}a{color:#93c5fd}.toolbar{display:flex;gap:10px;flex-wrap:wrap}.btn{background:#2563eb;color:white;border:0;border-radius:8px;padding:10px 14px;text-decoration:none;font-weight:700}</style></head>
<body><main><header><div><h1>Tenshi Portal Observability</h1><p class="muted">Etat local du portail, runtime Go et metriques HTTP scrapees par Prometheus.</p></div><div class="toolbar"><a class="btn" href="/">Accueil</a><a class="btn" href="/metrics">Metrics</a><a class="btn" href="https://grafana.tenshi-lab.fr/d/site-tenshi-portal/site-tenshi-portal">Grafana</a></div></header>
<section class="grid"><article class="card"><div class="label">Status</div><div class="value ok">ok</div></article><article class="card"><div class="label">Uptime</div><div class="value">%.0f min</div></article><article class="card"><div class="label">Go</div><div class="value">%s</div></article><article class="card"><div class="label">Requests</div><div class="value">%d</div></article><article class="card"><div class="label">Alloc</div><div class="value">%d MiB</div></article><article class="card"><div class="label">Goroutines</div><div class="value">%d</div></article></section>
<section class="card" style="margin-top:14px"><h2>Routes HTTP</h2><table><thead><tr><th>Route</th><th>Status</th><th>Requetes</th><th>Duree totale</th></tr></thead><tbody>%s</tbody></table></section>
</main></body></html>`, time.Since(metrics.startedAt).Minutes(), runtime.Version(), metrics.requests.Load(), mem.Alloc/1024/1024, runtime.NumGoroutine(), rows)
}

func label(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}
