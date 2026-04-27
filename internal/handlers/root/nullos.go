package handlers

import (
	"net/http"
)

func Nullos(w http.ResponseWriter, r *http.Request) {
	Render(w, "nullos")
}