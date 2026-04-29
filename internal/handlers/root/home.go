package handlers

import (
	"html/template"
	"net/http"
	"os"
)

type PageData struct {
	Title string
}

func Render(w http.ResponseWriter, name string) {
	err := os.WriteFile("internal/services/users.txt", []byte("1"), 0644)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tmpl := template.Must(template.ParseFiles(
		"web/templates/layout.html",
		"web/templates/"+name+".html",
	))

	data := PageData{
		Title: name,
	}

	tmpl.ExecuteTemplate(w, "layout.html", data)
}

func Home(w http.ResponseWriter, r *http.Request) {
	Render(w, "home")
}
func About(w http.ResponseWriter, r *http.Request) {
	Render(w, "about")
}
func Allah(w http.ResponseWriter, r *http.Request) {
	Render(w, "allah")
}
func Contacts(w http.ResponseWriter, r *http.Request) {
	Render(w, "contacts")
}
func Discord(w http.ResponseWriter, r *http.Request) {
	Render(w, "discord")
}
func Docs(w http.ResponseWriter, r *http.Request) {
	Render(w, "docs")
}
func Jaylub(w http.ResponseWriter, r *http.Request) {
	Render(w, "jaylub_page")
}
func Nullc(w http.ResponseWriter, r *http.Request) {
	Render(w, "nullc")
}
func Nullos(w http.ResponseWriter, r *http.Request) {
	Render(w, "nullos")
}
func Page(w http.ResponseWriter, r *http.Request) {
	Render(w, "page")
}
func Services(w http.ResponseWriter, r *http.Request) {
	Render(w, "services")
}
func Wiki_C(w http.ResponseWriter, r *http.Request) {
	Render(w, "wiki_c")
}
func Wiki_Jaylub(w http.ResponseWriter, r *http.Request) {
	Render(w, "wiki_jaylub")
}
func Wiki(w http.ResponseWriter, r *http.Request) {
	Render(w, "wiki")
}




