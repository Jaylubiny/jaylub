package handlers

import (
	"net/http"
)

func Services(w http.ResponseWriter, r *http.Request) {
	Render(w, "services")
}