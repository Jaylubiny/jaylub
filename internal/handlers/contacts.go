package handlers

import (
	"html/template"
	"net/http"
)

func Contacts(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/contacts.html",
	))

	data := PageData{
		Title: "Contacts",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}