package handlers

import (
	"html/template"
	"net/http"
)

func About(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/about.html",
	))

	data := PageData{
		Title: "About",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}