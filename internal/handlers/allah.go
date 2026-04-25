package handlers

import (
	"html/template"
	"net/http"
)

func Allah(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/allah.html",
	))

	data := PageData{
		Title: "Allah Bot",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}