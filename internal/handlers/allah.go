package handlers

import (
	"net/http"
)

func Allah(w http.ResponseWriter, r *http.Request) {
	Render(w, "allah")
}