package handlers

import (
	"jaylub/internal/views"
)

var renderer = views.NewRenderer("web/templates/company")

var (
	Home  = renderer.Page("home")
	About = renderer.Page("about")
)
