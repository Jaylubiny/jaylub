package handlers

import (
	"net/http"
)

func Nullc(w http.ResponseWriter, r *http.Request) {
	Render(w, "nullc")
}