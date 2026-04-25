package handlers

import(
	"net/http"
)

func Html(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../../web/templates/test.html")
}






