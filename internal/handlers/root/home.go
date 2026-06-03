package handlers

import (
	"jaylub/internal/views"
	"net/http"
	"os"
)

var renderer = views.NewRenderer("web/templates")

var (
	Home        = renderer.Page("home")
	About       = renderer.Page("about")
	Me          = renderer.Page("me")
	Allah       = renderer.Page("allah")
	Contacts    = renderer.Page("contacts")
	Discord     = renderer.Page("discord")
	Docs        = renderer.Page("docs")
	GameTest    = renderer.Page("game_test")
	Jaylub      = renderer.Page("jaylub_page")
	Nullc       = renderer.Page("nullc")
	Nullos      = renderer.Page("nullos")
	Page        = renderer.Page("page")
	Services    = renderer.Page("services")
	Wiki_C      = renderer.Page("wiki_c")
	Wiki_Jaylub = renderer.Page("wiki_jaylub")
	Wiki        = renderer.Page("wiki")
	Jayware     = renderer.Page("jayware")
)

func JaywareDownload(w http.ResponseWriter, r *http.Request) {
	filePath := "./internal/services/jayware/jaylub.zip"
	if _, err := os.Stat(filePath); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="jaylub.zip"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, filePath)
}
