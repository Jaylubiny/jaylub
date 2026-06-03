package router

import (
	"jaylub/internal/handlers/company"
	"net/http"
)

func Company() http.Handler {
	routes := []route{
		{"/", handlers.Home},
		{"/about", handlers.About},
	}
	return newMux(routes)
}
