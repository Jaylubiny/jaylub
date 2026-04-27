package handlers

import (
	"net/http"
)

func Page(w http.ResponseWriter, r *http.Request) {
	Render(w, "page")
}