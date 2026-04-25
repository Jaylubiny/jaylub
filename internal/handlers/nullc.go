package handlers

import (
	"html/template"
	"net/http"
)

func Nullc(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/nullc.html",
	))

	data := PageData{
		Title: "NullC",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}