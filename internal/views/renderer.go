package views

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"
)

type PageData struct {
	Title string
}

type Renderer struct {
	layoutPath  string
	templateDir string
}

func NewRenderer(templateDir string) Renderer {
	return Renderer{
		layoutPath:  "web/templates/layout.html",
		templateDir: templateDir,
	}
}

func (r Renderer) Page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.Render(w, name)
	}
}

func (r Renderer) Render(w http.ResponseWriter, name string) {
	if err := recordVisit(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles(
		r.layoutPath,
		filepath.Join(r.templateDir, name+".html"),
	)
	if err != nil {
		http.Error(w, "Template Error", http.StatusInternalServerError)
		return
	}

	data := PageData{Title: name}
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "Template Error", http.StatusInternalServerError)
	}
}

func recordVisit() error {
	return os.WriteFile("internal/services/users.txt", []byte("1"), 0644)
}
