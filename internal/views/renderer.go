package views

import (
	"html/template"
	"jaylub/internal/auth"
	"net/http"
	"os"
	"path/filepath"
)

type PageData struct {
	Title    string
	Username string
	Initials string
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
		r.Render(w, req, name)
	}
}

func (r Renderer) Render(w http.ResponseWriter, req *http.Request, name string) {
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
	if user, ok := auth.UserFromContext(req.Context()); ok {
		data.Username = user.Username
		data.Initials = initials(user.Username)
	}

	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "Template Error", http.StatusInternalServerError)
	}
}

func recordVisit() error {
	return os.WriteFile("internal/services/users.txt", []byte("1"), 0644)
}

func initials(username string) string {
	if username == "" {
		return "?"
	}
	return string([]rune(username)[0])
}
