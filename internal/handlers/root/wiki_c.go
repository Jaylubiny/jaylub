package handlers

import (
	"net/http"
)

func Wiki_C(w http.ResponseWriter, r *http.Request) {
	Render(w, "wiki_c")
}