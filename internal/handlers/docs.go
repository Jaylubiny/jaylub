package handlers

import (
	"html/template"
	"net/http"
)

func Docs(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/docs.html",
	))

	data := PageData{
		Title: "Documentation",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}