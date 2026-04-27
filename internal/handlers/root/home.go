package handlers

import (
	"html/template"
	"net/http"
)

type PageData struct {
	Title string
}

func Render(w http.ResponseWriter, name string) {
	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/"+name+".html",
	))

	data := PageData{
		Title: name,
	}

	tmpl.ExecuteTemplate(w, "layout.html", data)
}

func Home(w http.ResponseWriter, r *http.Request) {
	Render(w, "home")
}



