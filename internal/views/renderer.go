package views

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"jaylub/internal/auth"
	"net/http"
	"os"
	"path/filepath"
)

type PageData struct {
	Title           string
	MetaTitle       string
	MetaDescription string
	CanonicalURL    string
	OpenGraphTitle  string
	OpenGraphDesc   string
	OpenGraphType   string
	OpenGraphURL    string
	StructuredData  template.JS
	Indexable       bool
	Username        string
	Initials        string
	Query           string
	Stats           ProfileStats
	ChatUnreadCount int
	ChatSettings    auth.ChatSettings
}

type SEOData struct {
	Title               string
	Description         string
	CanonicalURL        string
	OpenGraphType       string
	ApplicationName     string
	ApplicationCategory string
	Features            []string
}

type softwareApplicationSchema struct {
	Context             string   `json:"@context"`
	Type                string   `json:"@type"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	URL                 string   `json:"url"`
	ApplicationCategory string   `json:"applicationCategory"`
	OperatingSystem     string   `json:"operatingSystem"`
	FeatureList         []string `json:"featureList"`
}

type ProfileStats struct {
	MessagesSent   int
	Gold           int
	LifetimeKills  int
	GameLevel      int
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
	return r.PageWithSEO(name, SEOData{})
}

func (r *Renderer) PageWithSEO(name string, seo SEOData) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.RenderWithSEO(w, req, name, seo)
	}
}

func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, name string) {
	r.RenderWithSEO(w, req, name, SEOData{})
}

func (r *Renderer) RenderWithSEO(w http.ResponseWriter, req *http.Request, name string, seo SEOData) {
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

	data := PageData{Title: name, Query: req.URL.Query().Get("saved")}
	if err := populateSEO(&data, seo); err != nil {
		http.Error(w, "Could not render page metadata.", http.StatusInternalServerError)
		return
	}
	if user, ok := auth.UserFromContext(req.Context()); ok {
		data.Username = user.Username
		data.Initials = initials(user.Username)
		data.Stats = r.profileStats(user)
		data.ChatUnreadCount = r.chatUnreadCount(user)
		if settings, err := r.chatSettings(user); err == nil {
			data.ChatSettings = settings
		} else {
			data.ChatSettings = auth.DefaultChatSettings()
		}
	}

	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "Template Error", http.StatusInternalServerError)
	}
}

func populateSEO(data *PageData, seo SEOData) error {
	if seo.Title == "" {
		return nil
	}

	data.MetaTitle = seo.Title
	data.MetaDescription = seo.Description
	data.CanonicalURL = seo.CanonicalURL
	data.OpenGraphTitle = seo.Title
	data.OpenGraphDesc = seo.Description
	data.OpenGraphType = seo.OpenGraphType
	data.OpenGraphURL = seo.CanonicalURL
	data.Indexable = true

	schema, err := json.Marshal(softwareApplicationSchema{
		Context:             "https://schema.org",
		Type:                "SoftwareApplication",
		Name:                seo.ApplicationName,
		Description:         seo.Description,
		URL:                 seo.CanonicalURL,
		ApplicationCategory: seo.ApplicationCategory,
		OperatingSystem:     "Web",
		FeatureList:         seo.Features,
	})
	if err != nil {
		return err
	}
	data.StructuredData = template.JS(schema)
	return nil
}

func (r *Renderer) chatSettings(user auth.User) (auth.ChatSettings, error) {
	if r.db == nil {
		return auth.DefaultChatSettings(), nil
	}

	settings := auth.DefaultChatSettings()
	var showTimestamps, showImagePreviews int
	err := r.db.QueryRow(`
		SELECT time_format, message_density, show_timestamps, show_image_previews
		FROM chat_settings
		WHERE user_id = ?
	`, user.ID).Scan(
		&settings.TimeFormat,
		&settings.MessageDensity,
		&showTimestamps,
		&showImagePreviews,
	)
	if err == sql.ErrNoRows {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	settings.ShowTimestamps = showTimestamps != 0
	settings.ShowImagePreviews = showImagePreviews != 0
	return settings, nil
}

func (r *Renderer) profileStats(user auth.User) ProfileStats {
	stats := ProfileStats{BestRunTime: "0:00"}
	if r.db == nil {
		return stats
	}

	_ = r.db.QueryRow(`SELECT COUNT(*) FROM chat_messages WHERE username = ?`, user.Username).Scan(&stats.MessagesSent)

	err := r.db.QueryRow(`
		SELECT gold, lifetime_kills, game_level
		FROM game_profiles
		WHERE user_id = ?
	`, user.ID).Scan(&stats.Gold, &stats.LifetimeKills, &stats.GameLevel)
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

func (r *Renderer) chatUnreadCount(user auth.User) int {
	if r.db == nil {
		return 0
	}

	var count int
	_ = r.db.QueryRow(`
		SELECT COUNT(*)
		FROM chat_messages
		WHERE username != ?
			AND timestamp > COALESCE((SELECT last_read_at FROM chat_reads WHERE user_id = ?), '1970-01-01T00:00:00Z')
	`, user.Username, user.ID).Scan(&count)
	return count
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
