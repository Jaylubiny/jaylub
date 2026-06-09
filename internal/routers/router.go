package router

import (
	"jaylub/internal/auth"
	"jaylub/internal/handlers/root"
	"net/http"
)

type route struct {
	pattern string
	handler http.HandlerFunc
}

func Basic(authService *auth.Service) http.Handler {
	handlers.UseStatsDB(authService.DB())
	chatService := handlers.NewChatService(authService.DB())
	jayliveService := handlers.NewJayliveService(authService.DB())
	routes := []route{
		{"/", handlers.Home},
		{"/about", handlers.About},
		{"/me", handlers.Me},
		{"/contacts", handlers.Contacts},
		{"/docs", handlers.Docs},
		{"/documentation", handlers.Docs},
		{"/game/test", handlers.GameTest},
		{"/game/jaylive", jayliveService.Page},
		{"/game/jaylive/state", jayliveService.State},
		{"/game/jaylive/start", jayliveService.StartRun},
		{"/game/jaylive/shop", jayliveService.BuyUpgrade},
		{"/game/jaylive/character", jayliveService.Character},
		{"/game/jaylive/run", jayliveService.SubmitRun},
		{"/game/jaylive/leaderboard", jayliveService.Leaderboard},

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
		{"/.well-known/discord", handlers.DiscordWellKnown},

		{"/chat", chatService.Page},
		{"/chat/messages", chatService.Messages},
		{"/chat/send", chatService.SendMessage},
	}
	return newMux(routes, authService)
}

func newMux(routes []route, authService *auth.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", authService.LoginPage())
	mux.HandleFunc("/logout", authService.LogoutHandler())
	for _, route := range routes {
		mux.HandleFunc(route.pattern, route.handler)
	}
	mountStaticFiles(mux)
	return authService.Middleware(mux)
}

func mountStaticFiles(mux *http.ServeMux) {
	fs := http.FileServer(http.Dir("./web/static"))
	mux.Handle("/web/static/", http.StripPrefix("/web/static/", fs))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
}
