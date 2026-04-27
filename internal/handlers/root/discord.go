package handlers

import (
	"net/http"
)

func Discord(w http.ResponseWriter, r *http.Request) {
	Render(w, "discord")
}