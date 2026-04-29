package router

import(
	"net/http"
	"jaylub/internal/handlers/company"
)

func Company() http.Handler {
	mux := http.NewServeMux()


	mux.HandleFunc("/", handlers.Home)
	mux.HandleFunc("/about", handlers.About)
	

	fs := http.FileServer(http.Dir("./web/static"))
	mux.Handle("/web/static/", http.StripPrefix("/web/static/", fs))
	return mux

}

