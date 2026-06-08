package views

import (
	"database/sql"
	"fmt"
	"html/template"
	"jaylub/internal/auth"
	"net/http"
	"os"
	"path/filepath"
)

type PageData struct {
	Title    string
	Username string
	Initials string
	Stats    ProfileStats
}

type ProfileStats struct {
	MessagesSent   int
	Gold           int
	LifetimeKills  int
	BestRunKills   int
	BestRunTime    string
	HasGameProfile bool
}

type Renderer struct {
	layoutPath  string
	templateDir string
	db          *sql.DB
}

func NewRenderer(templateDir string) *Renderer {
	return &Renderer{
		layoutPath:  "web/templates/layout.html",
		templateDir: templateDir,
	}
}

func (r *Renderer) SetDB(db *sql.DB) {
	r.db = db
}

func (r *Renderer) Page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.Render(w, req, name)
	}
}

func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, name string) {
	if err := recordVisit(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles(
		r.layoutPath,
		filepath.Join(r.templateDir, name+".html"),
	)
	if err != nil {
		http.Error(w, "Template Error", http.StatusInternalServerError)
		return
	}

	data := PageData{Title: name}
	if user, ok := auth.UserFromContext(req.Context()); ok {
		data.Username = user.Username
		data.Initials = initials(user.Username)
		data.Stats = r.profileStats(user)
	}

	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "Template Error", http.StatusInternalServerError)
	}
}

func (r *Renderer) profileStats(user auth.User) ProfileStats {
	stats := ProfileStats{BestRunTime: "0:00"}
	if r.db == nil {
		return stats
	}

	_ = r.db.QueryRow(`SELECT COUNT(*) FROM chat_messages WHERE username = ?`, user.Username).Scan(&stats.MessagesSent)

	err := r.db.QueryRow(`
		SELECT gold, lifetime_kills
		FROM game_profiles
		WHERE user_id = ?
	`, user.ID).Scan(&stats.Gold, &stats.LifetimeKills)
	if err == nil {
		stats.HasGameProfile = true
	}

	var bestRunSeconds int
	err = r.db.QueryRow(`
		SELECT best_run_kills, best_run_seconds
		FROM game_leaderboard
		WHERE user_id = ?
	`, user.ID).Scan(&stats.BestRunKills, &bestRunSeconds)
	if err == nil {
		stats.BestRunTime = formatSeconds(bestRunSeconds)
	}

	return stats
}

func recordVisit() error {
	return os.WriteFile("internal/services/users.txt", []byte("1"), 0644)
}

func initials(username string) string {
	if username == "" {
		return "?"
	}
	return string([]rune(username)[0])
}

func formatSeconds(total int) string {
	if total < 0 {
		total = 0
	}
	minutes := total / 60
	seconds := total % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
