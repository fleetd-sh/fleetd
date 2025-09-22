package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

//go:embed index.html.tmpl
var indexTemplate string

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	specFile := os.Getenv("SPEC_FILE")
	if specFile == "" {
		specFile = "docs/api/fleetd.swagger.json"
	}

	mux := http.NewServeMux()

	// Serve the OpenAPI spec
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(specFile)
		if err != nil {
			http.Error(w, "Failed to read spec file", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	// Swagger UI will be loaded from CDN in index.html.tmpl

	// Serve the main page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		tmpl, err := template.New("index").Parse(indexTemplate)
		if err != nil {
			http.Error(w, "Failed to parse template", http.StatusInternalServerError)
			return
		}

		data := struct {
			Title string
			APIURL string
		}{
			Title: "fleetd API Documentation",
			APIURL: "/openapi.json",
		}

		w.Header().Set("Content-Type", "text/html")
		tmpl.Execute(w, data)
	})

	// Redirect /docs to /
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Swagger UI server starting on http://localhost%s", addr)
	log.Printf("Serving OpenAPI spec from: %s", specFile)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}