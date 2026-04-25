package handlers

import (
	"html/template"
	"net/http"
)

type PageData struct {
	Title string
}

func Home(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/home.html",
	))

	data := PageData{
		Title: "Home",
	}

	err := tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

}



