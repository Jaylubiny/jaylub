package handlers

import (
	"database/sql"
	"net/http"

	"jaylub/internal/auth"
	"jaylub/internal/views"
)

func EmailPage(db *sql.DB) http.HandlerFunc {
	renderer := views.NewRenderer("web/templates")
	renderer.SetDB(db)
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserFromContext(r.Context()); !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		renderer.Render(w, r, "email")
	}
}
