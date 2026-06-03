package router

import (
	"jaylub/internal/handlers/root"
	"net/http"
)

type route struct {
	pattern string
	handler http.HandlerFunc
}

func Basic() http.Handler {
	routes := []route{
		{"/", handlers.Home},
		{"/about", handlers.About},
		{"/me", handlers.Me},
		{"/contacts", handlers.Contacts},
		{"/docs", handlers.Docs},
		{"/game/test", handlers.GameTest},

		{"/wiki", handlers.Wiki},
		{"/wiki/C", handlers.Wiki_C},
		{"/wiki/jaylub", handlers.Wiki_Jaylub},

		{"/pages", handlers.Page},
		{"/pages/jayware", handlers.Jayware},
		{"/pages/jayware/download", handlers.JaywareDownload},
		{"/pages/nullos", handlers.Nullos},
		{"/pages/nullc", handlers.Nullc},
		{"/pages/allah", handlers.Allah},
		{"/pages/jaylub", handlers.Jaylub},
		{"/pages/services", handlers.Services},

		{"/discord", handlers.Discord},
		{"/discord/allah/callback", handlers.AllahCallback},
	}
	return newMux(routes)
}

func newMux(routes []route) *http.ServeMux {
	mux := http.NewServeMux()
	for _, route := range routes {
		mux.HandleFunc(route.pattern, route.handler)
	}
	mountStaticFiles(mux)
	return mux
}

func mountStaticFiles(mux *http.ServeMux) {
	fs := http.FileServer(http.Dir("./web/static"))
	mux.Handle("/web/static/", http.StripPrefix("/web/static/", fs))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
}
