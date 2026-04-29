package handlers

import (
	"html/template"
	"net/http"
	"os"
)

type PageData struct {
	Title string
}

func Render(w http.ResponseWriter, name string) {
	err := os.WriteFile("internal/services/users.txt", []byte("1"), 0644)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/company/"+name+".html",
	))

	data := PageData{
		Title: name,
	}

	tmpl.ExecuteTemplate(w, "layout.html", data)
}

func Home(w http.ResponseWriter, r *http.Request) {
	Render(w, "home")
}

func About(w http.ResponseWriter, r *http.Request) {
	Render(w, "about")
}


