package handlers

import (
	"html/template"
	"net/http"
)

func Page(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/page.html",
	))

	data := PageData{
		Title: "Pages",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}