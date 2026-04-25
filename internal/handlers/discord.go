package handlers

import (
	"html/template"
	"net/http"
)

func Discord(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/discord.html",
	))

	data := PageData{
		Title: "Discord",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}