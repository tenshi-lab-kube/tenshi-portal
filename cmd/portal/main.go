package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
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
	Services    []Service
}

func main() {
	port := env("PORT", "8080")
	tmpl := template.Must(template.ParseFS(content, "templates/*.html"))

	services := []Service{
		{Name: "Authentik", Kind: "Identity", Description: "Single sign-on, comptes, groupes et politiques d'acces.", URL: "https://auth.tenshi-lab.fr", Status: "SSO", Accent: "teal", Initials: "AK"},
		{Name: "Grafana", Kind: "Observability", Description: "Dashboards, metriques cluster et supervision des services.", URL: "https://grafana.tenshi-lab.fr", Status: "Live", Accent: "amber", Initials: "GF"},
		{Name: "Argo CD", Kind: "GitOps", Description: "Deploiements Kubernetes, sync applicative et etat GitOps.", URL: "https://argocd.tenshi-lab.fr", Status: "GitOps", Accent: "rose", Initials: "AC"},
		{Name: "Discord Tickets", Kind: "Support", Description: "Dashboard du bot Discord, tickets, categories et transcripts.", URL: "https://tickets.tenshi-lab.fr", Status: "Prod", Accent: "violet", Initials: "DT"},
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(content)))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := PageData{Title: "Tenshi Lab", Subtitle: "Console d'acces aux services du cluster Kubernetes.", Environment: env("PORTAL_ENV", "Homelab"), UpdatedAt: time.Now().Format("02 Jan 2006 15:04"), Services: services}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("render error: %v", err)
		}
	})

	server := &http.Server{Addr: ":" + port, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second}
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
