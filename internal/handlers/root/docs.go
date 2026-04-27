package handlers

import (
	"net/http"
)

func Docs(w http.ResponseWriter, r *http.Request) {
	Render(w, "docs")
}