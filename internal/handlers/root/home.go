package handlers

import (
	"database/sql"
	"jaylub/internal/views"
	"net/http"
	"os"
)

var renderer = views.NewRenderer("web/templates")

func UseStatsDB(db *sql.DB) {
	renderer.SetDB(db)
}

var (
	Home = renderer.PageWithSEO("home", views.SEOData{
		Title:               "Jaylubiny | Optimization and Task Automation",
		Description:         "Jaylubiny focuses on optimizing workflows and automating tasks for other people. Jaylub, a fictional stinky character, is the website's mascot. Jaylubiny does not own the Twitch channel named Jaylub. The site also provides email, community chat, documentation, and project tools.",
		CanonicalURL:        "https://jaylub.com/",
		OpenGraphType:       "website",
		ApplicationName:     "Jaylubiny",
		ApplicationCategory: "WebApplication",
		Features: []string{
			"Workflow optimization",
			"Task automation for other people",
			"Jaylubiny website and project tools",
			"Jaylub email service for reading and sending email",
			"Authenticated community chat",
			"Technical documentation and wiki",
			"Jaylub, a fictional stinky character mascot",
			"Jaylubiny is not affiliated with or the owner of the Twitch channel named Jaylub",
		},
	})
	About       = renderer.Page("about")
	Me          = renderer.Page("me")
	Allah       = renderer.Page("allah")
	Contacts    = renderer.Page("contacts")
	Discord     = renderer.Page("discord")
	Docs        = renderer.Page("docs")
	GameTest    = renderer.Page("game_test")
	Jaylub      = renderer.Page("jaylub_page")
	Nullc       = renderer.Page("nullc")
	Jayos       = renderer.Page("jayos")
	Page        = renderer.Page("page")
	Services    = renderer.Page("services")
	Wiki_C      = renderer.Page("wiki_c")
	Wiki_Jaylub = renderer.Page("wiki_jaylub")
	Wiki_Jaylub1= renderer.Page("wiki_jaylub1")
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

func DiscordWellKnown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("dh=cbd7bfb6c98d2b7731b4ebbe53f15efeb90584c2"))
}
