package handlers

import (
	"html/template"
	"net/http"
)

func Wiki(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/wiki.html",
	))

	data := PageData{
		Title: "Wiki",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}