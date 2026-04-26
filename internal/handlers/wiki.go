package handlers

import (
	"net/http"
)

func Wiki(w http.ResponseWriter, r *http.Request) {
	Render(w, "wiki")
}