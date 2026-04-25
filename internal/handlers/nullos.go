package handlers

import (
	"html/template"
	"net/http"
)

func Nullos(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/nullos.html",
	))

	data := PageData{
		Title: "NullOS",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}
