package handlers

import (
	"html/template"
	"net/http"
)

func Jaylub(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/jaylub_page.html",
	))

	data := PageData{
		Title: "Jaylub Bot",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}