package router

import (
	"jaylub/internal/auth"
	"jaylub/internal/handlers/company"
	"net/http"
)

func Company(authService *auth.Service) http.Handler {
	routes := []route{
		{"/", handlers.Home},
		{"/about", handlers.About},
	}
	return newMux(routes, authService)
}
