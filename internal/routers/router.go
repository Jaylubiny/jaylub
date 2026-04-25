package router

import(
	"net/http"
	"jaylub/internal/handlers"
)

func NewRouter() http.Handler {


	
	mux := http.NewServeMux()

	mux.HandleFunc("/", handlers.Home)
	mux.HandleFunc("/about", handlers.About)
	mux.HandleFunc("/contacts", handlers.Contacts)
	mux.HandleFunc("/docs", handlers.Docs)

	mux.HandleFunc("/wiki", handlers.Wiki)
	mux.HandleFunc("/wiki/C", handlers.Wiki_C)
	mux.HandleFunc("/wiki/jaylub", handlers.Wiki_Jaylub)


	mux.HandleFunc("/pages", handlers.Page)
	mux.HandleFunc("/pages/nullos", handlers.Nullos)
	mux.HandleFunc("/pages/nullc", handlers.Nullc)
	mux.HandleFunc("/pages/allah", handlers.Allah)
	mux.HandleFunc("/pages/jaylub", handlers.Jaylub)
	mux.HandleFunc("/pages/services", handlers.Services)

	mux.HandleFunc("/discord", handlers.Discord)
	mux.HandleFunc("/discord/allah/callback", handlers.AllahCallback)




	fs := http.FileServer(http.Dir("./web/static"))
	mux.Handle("/web/static/", http.StripPrefix("/web/static/", fs))
	return mux

}
