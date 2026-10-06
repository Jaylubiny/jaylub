package handlers

import (
	"jaylub/internal/auth"
	"net/http"
)

func Terms(authService *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			renderer.Render(w, r, "terms")
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			if r.FormValue("accept_terms") != "yes" {
				http.Error(w, "You must accept the terms to continue.", http.StatusBadRequest)
				return
			}
			if err := authService.AcceptTermsOnDevice(w, r); err != nil {
				http.Error(w, "Could not save terms acceptance.", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}
}
