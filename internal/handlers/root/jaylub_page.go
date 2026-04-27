package handlers

import (
	"net/http"
)

func Jaylub(w http.ResponseWriter, r *http.Request) {
	Render(w, "jaylub_page")
}