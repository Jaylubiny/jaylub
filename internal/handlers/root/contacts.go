package handlers

import (
	"net/http"
)

func Contacts(w http.ResponseWriter, r *http.Request) {
	Render(w, "contacts")
}