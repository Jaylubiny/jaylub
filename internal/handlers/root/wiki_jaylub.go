package handlers

import (
	"net/http"
)

func Wiki_Jaylub(w http.ResponseWriter, r *http.Request) {
	Render(w, "wiki_jaylub")
}