package router

import (
	"jaylub/internal/auth"
	"jaylub/internal/handlers/company"
	rootHandlers "jaylub/internal/handlers/root"
	"net/http"
)

func Company(authService *auth.Service) http.Handler {
	routes := []route{
		{"/", handlers.Home},
		{"/about", handlers.About},
		{"/terms", rootHandlers.Terms(authService)},
	}
	return newMux(routes, authService)
}
