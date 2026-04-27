package handlers

import (
	"net/http"
)

func About(w http.ResponseWriter, r *http.Request) {
	Render(w, "about")
}