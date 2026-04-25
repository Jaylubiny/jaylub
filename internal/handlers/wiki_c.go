package handlers

import (
	"html/template"
	"net/http"
)

func Wiki_C(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/wiki_c.html",
	))

	data := PageData{
		Title: "Wiki C",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}