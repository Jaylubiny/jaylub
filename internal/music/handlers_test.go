package music

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"jaylub/internal/auth"
	"jaylub/internal/views"

	"github.com/tcolgate/mp3"
)

func newTestMusicHandler(t *testing.T) (*Store, http.Handler, string) {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "music.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close music store: %v", err)
		}
	})
	uploadDir := filepath.Join(t.TempDir(), "mp3s")
	handler := MockAuthMiddleware(NewHandler(store, uploadDir, func(user User) views.PageData {
		return views.PageData{
			Username: user.Username,
			Initials: "AL",
			Stats:    views.ProfileStats{BestRunTime: "0:00"},
		}
	}))
	return store, handler, uploadDir
}

func TestOpenStoreCreatesDatabaseAndSchemaOnFirstRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "new", "install", "music.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open new music store: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file was not created: %v", err)
	}
	for _, table := range []string{"users", "songs", "user_favorites", "playlists", "playlist_songs"} {
		var exists int
		if err := store.db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`,
			table,
		).Scan(&exists); err != nil {
			t.Fatalf("check %s table: %v", table, err)
		}
		if exists != 1 {
			t.Errorf("table %q was not created", table)
		}
	}
}

func TestOpenStoreAddsPlaylistSchemaWithoutChangingExistingMusicData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "existing", "music.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT NOT NULL);
		CREATE TABLE songs (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			artist TEXT NOT NULL,
			album TEXT NOT NULL,
			duration_seconds INTEGER NOT NULL CHECK (duration_seconds >= 0),
			file_path TEXT NOT NULL UNIQUE,
			uploaded_by INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE CASCADE
		);
		CREATE TABLE user_favorites (
			user_id INTEGER NOT NULL,
			song_id TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, song_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE
		);
		INSERT INTO users (id, username) VALUES (1, 'alice');
		INSERT INTO songs (id, title, artist, album, duration_seconds, file_path, uploaded_by)
		VALUES ('7ca6802d-e997-4f85-b169-9185df172c1a', 'Existing Track', 'Artist', 'Album', 42, 'existing.mp3', 1);
		INSERT INTO user_favorites (user_id, song_id)
		VALUES (1, '7ca6802d-e997-4f85-b169-9185df172c1a');
	`)
	if err != nil {
		_ = db.Close()
		t.Fatalf("create old music schema fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close old music schema fixture: %v", err)
	}

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open existing music database: %v", err)
	}
	defer store.Close()

	for _, table := range []string{"playlists", "playlist_songs"} {
		var exists int
		if err := store.db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`,
			table,
		).Scan(&exists); err != nil {
			t.Fatalf("check %s table: %v", table, err)
		}
		if exists != 1 {
			t.Errorf("upgrade did not create %q table", table)
		}
	}
	var username, title string
	var favorites int
	if err := store.db.QueryRow(`
		SELECT u.username, s.title, COUNT(f.song_id)
		FROM users u
		JOIN songs s ON s.uploaded_by = u.id
		LEFT JOIN user_favorites f ON f.user_id = u.id AND f.song_id = s.id
		WHERE u.id = 1
		GROUP BY u.id, s.id
	`).Scan(&username, &title, &favorites); err != nil {
		t.Fatalf("read existing music data after schema upgrade: %v", err)
	}
	if username != "alice" || title != "Existing Track" || favorites != 1 {
		t.Errorf("existing data changed after schema upgrade: user=%q title=%q favorites=%d",
			username, title, favorites)
	}
}

func withTestUser(request *http.Request, userID, username string) {
	request.Header.Set("X-Music-User-ID", userID)
	request.Header.Set("X-Music-Username", username)
}

func TestMockAuthProvidesLocalUserAndRejectsInvalidOverride(t *testing.T) {
	_, handler, _ := newTestMusicHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/songs", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("default local user status = %d, want 200", response.Code)
	}

	for _, userID := range []string{"0", "not-a-number"} {
		request := httptest.NewRequest(http.MethodGet, "/api/songs", nil)
		request.Header.Set("X-Music-User-ID", userID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("override user ID %q status = %d, want 401", userID, response.Code)
		}
	}
}

func TestMusicHandlerSetsSecurityHeaders(t *testing.T) {
	_, handler, _ := newTestMusicHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("index status = %d, want 200", response.Code)
	}
	for header, expected := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "same-origin",
		"X-Robots-Tag":            "noindex, nofollow, noarchive",
		"Content-Security-Policy": "default-src 'self'",
	} {
		if got := response.Header().Get(header); !strings.HasPrefix(got, expected) {
			t.Errorf("%s = %q, want prefix %q", header, got, expected)
		}
	}
}

func TestSiteAuthMiddlewareMarksUnauthenticatedResponsesNoIndex(t *testing.T) {
	handler := SiteAuthMiddleware(
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			})
		},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("unauthenticated request reached music handler")
		}),
		func(*http.Request) (int64, string, bool) {
			return 0, "", false
		},
	)
	request := httptest.NewRequest(http.MethodGet, "https://music.jaylub.com/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", response.Code)
	}
	if got := response.Header().Get("X-Robots-Tag"); got != "noindex, nofollow, noarchive" {
		t.Errorf("unauthenticated X-Robots-Tag = %q", got)
	}
}

func TestIndexRendersAuthenticatedAccountActions(t *testing.T) {
	store, _, uploadDir := newTestMusicHandler(t)
	_, err := store.db.Exec(`
		CREATE TABLE chat_messages (username TEXT NOT NULL, timestamp TEXT NOT NULL);
		CREATE TABLE chat_reads (user_id INTEGER PRIMARY KEY, last_read_at TEXT);
		CREATE TABLE game_profiles (
			user_id INTEGER PRIMARY KEY,
			gold INTEGER NOT NULL,
			lifetime_kills INTEGER NOT NULL,
			game_level INTEGER NOT NULL
		);
		CREATE TABLE game_leaderboard (
			user_id INTEGER PRIMARY KEY,
			best_run_kills INTEGER NOT NULL,
			best_run_seconds INTEGER NOT NULL
		);
		INSERT INTO chat_messages (username, timestamp) VALUES
			('alice', '2026-10-01T00:00:00Z'),
			('alice', '2026-10-02T00:00:00Z');
		INSERT INTO game_profiles (user_id, gold, lifetime_kills, game_level)
			VALUES (12, 45, 678, 9);
		INSERT INTO game_leaderboard (user_id, best_run_kills, best_run_seconds)
			VALUES (12, 123, 90);
	`)
	if err != nil {
		t.Fatal(err)
	}
	profileRenderer := views.NewRenderer("web/templates")
	profileRenderer.SetDB(store.db)
	handler := MockAuthMiddleware(NewHandler(store, uploadDir, func(user User) views.PageData {
		return profileRenderer.ProfileMenuData(auth.User{ID: user.ID, Username: user.Username})
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	withTestUser(request, "12", "alice")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("index status = %d, want 200: %s", response.Code, response.Body.String())
	}
	page := response.Body.String()
	for _, expected := range []string{
		`class="profile-button"`,
		`class="profile-avatar" aria-hidden="true">a</span>`,
		`href="https://jaylub.com/settings">Settings</a>`,
		`action="https://jaylub.com/logout"`,
		`>Logout</button>`,
		`aria-label="Profile stats"`,
		`<strong>2</strong>`,
		`<strong>45</strong>`,
		`<strong>678</strong>`,
		`<strong>9</strong>`,
		`<strong>123</strong>`,
		`<strong>1:30</strong>`,
		`id="menu-toggle"`,
		`data-tab="playlists">Playlists</button>`,
		`id="playlist-create-form"`,
		`id="playlist-song-form"`,
		`id="global-search" type="search"`,
		`<audio id="audio-player" preload="auto" playsinline>`,
		`<span class="visually-hidden">Search the global library</span>`,
		`name="robots" content="noindex, nofollow, noarchive"`,
		`name="file" type="file" accept=".mp3,audio/mpeg" multiple required`,
		`Select up to 25 files`,
		`rel="manifest" href="/manifest.webmanifest"`,
		`name="description" content="Jaylub Music is a private MP3 library for signed-in Jaylub users.`,
		`name="application-name" content="Jaylub Music"`,
		`property="og:title" content="Jaylub Music"`,
		`property="og:description" content="Private MP3 library for signed-in Jaylub users.`,
		`property="og:url" content="https://music.jaylub.com/"`,
		`property="og:image" content="https://music.jaylub.com/icons/icon-512.png"`,
		`property="og:image:type" content="image/png"`,
		`property="og:image:width" content="512"`,
		`name="twitter:card" content="summary"`,
		`name="twitter:description" content="Private MP3 library for signed-in Jaylub users.`,
		`rel="canonical" href="https://music.jaylub.com/"`,
	} {
		if !strings.Contains(page, expected) {
			t.Errorf("rendered page does not contain %q", expected)
		}
	}
	if strings.Contains(page, `href="https://jaylub.com/login">Login</a>`) {
		t.Error("authenticated page unexpectedly includes the login action")
	}
}

func TestPageTemplateRendersLoginForUnauthenticatedContext(t *testing.T) {
	pageTemplate := template.Must(template.ParseFS(webFiles, "web/index.html"))
	var page strings.Builder
	if err := pageTemplate.Execute(&page, struct{ Username string }{}); err != nil {
		t.Fatalf("render unauthenticated page: %v", err)
	}
	content := page.String()
	if !strings.Contains(content, `href="https://jaylub.com/login">Login</a>`) {
		t.Error("unauthenticated page does not include login action")
	}
	if strings.Contains(content, `href="https://jaylub.com/me">Profile</a>`) ||
		strings.Contains(content, `action="https://jaylub.com/logout"`) {
		t.Error("unauthenticated page unexpectedly includes authenticated actions")
	}
}

func TestFrontendAssetsServeIconAndDoNotCacheOldTrackDetails(t *testing.T) {
	_, handler, _ := newTestMusicHandler(t)
	for _, assetPath := range []string{
		"/assets/app.js",
		"/assets/styles.css",
		"/favicon.ico",
		"/icons/icon-192.png",
		"/icons/icon-512.png",
		"/manifest.webmanifest",
		"/offline.html",
	} {
		request := httptest.NewRequest(http.MethodGet, assetPath, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", assetPath, response.Code)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", assetPath, got)
		}
		if assetPath == "/favicon.ico" {
			if got := response.Header().Get("Content-Type"); got != "image/png" {
				t.Errorf("favicon Content-Type = %q, want image/png", got)
			}
			if !bytes.HasPrefix(response.Body.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
				t.Error("favicon response is not a PNG")
			}
		}
		if strings.HasPrefix(assetPath, "/icons/") {
			size := 192
			if strings.Contains(assetPath, "512") {
				size = 512
			}
			if width, height := pngDimensions(t, response.Body.Bytes()); width != size || height != size {
				t.Errorf("%s dimensions = %dx%d, want %dx%d", assetPath, width, height, size, size)
			}
		}
		if assetPath == "/manifest.webmanifest" {
			if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/manifest+json") {
				t.Errorf("manifest Content-Type = %q", got)
			}
			var manifest struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Display     string `json:"display"`
				StartURL    string `json:"start_url"`
				Scope       string `json:"scope"`
				Icons       []struct {
					Src   string `json:"src"`
					Sizes string `json:"sizes"`
					Type  string `json:"type"`
				} `json:"icons"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &manifest); err != nil {
				t.Fatalf("decode manifest: %v", err)
			}
			if manifest.Name != "Jaylub Music" ||
				!strings.Contains(manifest.Description, "up to 25 MP3 files per batch") ||
				!strings.Contains(manifest.Description, "100 MiB per file") ||
				!strings.Contains(manifest.Description, "Offline mode caches the public app shell") ||
				manifest.Display != "standalone" ||
				manifest.StartURL != "/" || manifest.Scope != "/" || len(manifest.Icons) != 2 {
				t.Errorf("manifest configuration = %+v", manifest)
			}
			if len(manifest.Icons) == 2 &&
				(manifest.Icons[0].Sizes != "192x192" || manifest.Icons[1].Sizes != "512x512") {
				t.Errorf("manifest icon sizes = %q and %q", manifest.Icons[0].Sizes, manifest.Icons[1].Sizes)
			}
		}
		if assetPath == "/assets/app.js" {
			body := response.Body.String()
			if strings.Contains(body, "textContent = song.artist") || strings.Contains(body, "textContent = song.album") {
				t.Error("track UI still renders artist or album details")
			}
			if !strings.Contains(body, "`ID: ${song.id}`") {
				t.Error("track UI does not render each song ID")
			}
			if !strings.Contains(body, "[song.title, song.artist, song.album, song.id]") {
				t.Error("global library search does not include track metadata and ID")
			}
			for _, behavior := range []string{
				`audio.addEventListener("ended", () => moveQueue(1))`,
				`navigator.mediaSession.setActionHandler("nexttrack", () => moveQueue(1))`,
				`navigator.mediaSession.setPositionState`,
				`artwork: [`,
			} {
				if !strings.Contains(body, behavior) {
					t.Errorf("player is missing background playback behavior %q", behavior)
				}
			}
		}
	}
}

func pngDimensions(t *testing.T, data []byte) (int, int) {
	t.Helper()
	if len(data) < 24 || !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("icon response is not a valid PNG")
	}
	return int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19]),
		int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
}

func TestServiceWorkerOnlyCachesPublicShellAndUsesRootScope(t *testing.T) {
	_, handler, _ := newTestMusicHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/sw.js", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("service worker status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Errorf("service worker Content-Type = %q", got)
	}
	if got := response.Header().Get("Service-Worker-Allowed"); got != "/" {
		t.Errorf("Service-Worker-Allowed = %q, want /", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("service worker Cache-Control = %q, want no-cache", got)
	}
	script := response.Body.String()
	if !strings.Contains(script, "jaylub-music-shell-v8") ||
		!strings.Contains(script, "/assets/app.js?v=pwa-6") {
		t.Error("service worker shell cache is not versioned to refresh stale app assets")
	}
	if strings.Contains(script, `"/api/`) || strings.Contains(script, `"/api/stream`) {
		t.Error("service worker must not cache authenticated APIs or streams")
	}
	if !strings.Contains(script, `fetch(request).catch`) || !strings.Contains(script, `"/offline.html"`) {
		t.Error("service worker does not use the offline page as navigation fallback")
	}
}

func TestFavoritesAreScopedToAuthenticatedUser(t *testing.T) {
	store, handler, _ := newTestMusicHandler(t)
	if err := store.ensureUser(1, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureUser(2, "bob"); err != nil {
		t.Fatal(err)
	}
	const songID = "7ca6802d-e997-4f85-b169-9185df172c1a"
	if _, err := store.db.Exec(`
		INSERT INTO songs (id, title, artist, album, duration_seconds, file_path, uploaded_by)
		VALUES (?, 'Test Track', 'Test Artist', 'Test Album', 123, 'test.mp3', 1)
	`, songID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO user_favorites (user_id, song_id) VALUES (1, ?)`, songID); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		userID        string
		wantFavorites int
	}{
		{userID: "1", wantFavorites: 1},
		{userID: "2", wantFavorites: 0},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/favorites", nil)
		withTestUser(request, test.userID, "test user")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("favorites status = %d, want 200: %s", response.Code, response.Body.String())
		}
		var payload struct {
			Songs []Song `json:"songs"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Songs) != test.wantFavorites {
			t.Errorf("user %s favorites = %d, want %d", test.userID, len(payload.Songs), test.wantFavorites)
		}
	}
}

func TestPlaylistsArePrivateAndSupportTrackManagement(t *testing.T) {
	store, handler, _ := newTestMusicHandler(t)
	if err := store.ensureUser(1, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureUser(2, "bob"); err != nil {
		t.Fatal(err)
	}
	const songID = "7ca6802d-e997-4f85-b169-9185df172c1a"
	if _, err := store.db.Exec(`
		INSERT INTO songs (id, title, artist, album, duration_seconds, file_path, uploaded_by)
		VALUES (?, 'Test Track', 'Test Artist', 'Test Album', 123, 'test.mp3', 1)
	`, songID); err != nil {
		t.Fatal(err)
	}

	createBody, _ := json.Marshal(map[string]string{"name": "Road Trip"})
	request := httptest.NewRequest(http.MethodPost, "/api/playlists", bytes.NewReader(createBody))
	withTestUser(request, "1", "alice")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create playlist status = %d, want 201: %s", response.Code, response.Body.String())
	}
	var created struct {
		Playlist Playlist `json:"playlist"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Playlist.Name != "Road Trip" || created.Playlist.ID <= 0 {
		t.Fatalf("created playlist = %+v", created.Playlist)
	}

	addBody, _ := json.Marshal(map[string]string{"song_id": songID})
	for i := 0; i < 2; i++ {
		request = httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/playlists/%d/songs", created.Playlist.ID), bytes.NewReader(addBody))
		withTestUser(request, "1", "alice")
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("add track attempt %d status = %d: %s", i+1, response.Code, response.Body.String())
		}
	}

	request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/playlists/%d", created.Playlist.ID), nil)
	withTestUser(request, "1", "alice")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("get playlist status = %d: %s", response.Code, response.Body.String())
	}
	var detail struct {
		Playlist Playlist `json:"playlist"`
		Songs    []Song   `json:"songs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Playlist.SongCount != 1 || len(detail.Songs) != 1 || detail.Songs[0].ID != songID {
		t.Fatalf("playlist detail did not contain the unique track: %+v", detail)
	}

	request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/playlists/%d", created.Playlist.ID), nil)
	withTestUser(request, "2", "bob")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Errorf("other user's playlist status = %d, want 404", response.Code)
	}

	request = httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/playlists/%d/songs/%s", created.Playlist.ID, songID), nil)
	withTestUser(request, "1", "alice")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("remove track status = %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/playlists/%d", created.Playlist.ID), nil)
	withTestUser(request, "2", "bob")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Errorf("other user's delete status = %d, want 404", response.Code)
	}

	request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/playlists/%d", created.Playlist.ID), nil)
	withTestUser(request, "1", "alice")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete playlist status = %d: %s", response.Code, response.Body.String())
	}
	var remaining int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM playlist_songs WHERE playlist_id = ?`, created.Playlist.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("deleted playlist retained %d track memberships", remaining)
	}
}

func TestCreatePlaylistValidatesAndUniquifiesNames(t *testing.T) {
	_, handler, _ := newTestMusicHandler(t)
	for _, name := range []string{"   ", strings.Repeat("x", 81)} {
		body, _ := json.Marshal(map[string]string{"name": name})
		request := httptest.NewRequest(http.MethodPost, "/api/playlists", bytes.NewReader(body))
		withTestUser(request, "1", "alice")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("playlist name of %d runes status = %d, want 400", utf8.RuneCountInString(name), response.Code)
		}
	}
	for _, name := range []string{"Road Trip", "road trip"} {
		body, _ := json.Marshal(map[string]string{"name": name})
		request := httptest.NewRequest(http.MethodPost, "/api/playlists", bytes.NewReader(body))
		withTestUser(request, "1", "alice")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if name == "Road Trip" && response.Code != http.StatusCreated {
			t.Fatalf("create first playlist status = %d: %s", response.Code, response.Body.String())
		}
		if name == "road trip" && response.Code != http.StatusConflict {
			t.Errorf("duplicate playlist status = %d, want 409", response.Code)
		}
	}
}

func TestStreamSupportsHTTPRangeRequests(t *testing.T) {
	store, handler, uploadDir := newTestMusicHandler(t)
	if err := store.ensureUser(1, "alice"); err != nil {
		t.Fatal(err)
	}
	const songID = "7ca6802d-e997-4f85-b169-9185df172c1a"
	const content = "0123456789"
	if err := os.MkdirAll(uploadDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, songID+".mp3"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO songs (id, title, artist, album, duration_seconds, file_path, uploaded_by)
		VALUES (?, 'Test', 'Artist', 'Album', 1, ?, 1)
	`, songID, songID+".mp3"); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/stream?id="+songID, nil)
	request.Header.Set("Range", "bytes=2-5")
	withTestUser(request, "1", "alice")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206; body = %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q, want bytes 2-5/10", got)
	}
	if !bytes.Equal(response.Body.Bytes(), []byte("2345")) {
		t.Fatalf("range response = %q, want %q", response.Body.Bytes(), "2345")
	}
}

func TestUploadRejectsNonMP3Extension(t *testing.T) {
	_, handler, _ := newTestMusicHandler(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "not-music.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("fake audio")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	withTestUser(request, "1", "alice")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("upload status = %d, want 400: %s", response.Code, response.Body.String())
	}
}

func TestUploadRejectsMP3LargerThanLimit(t *testing.T) {
	_, handler, _ := newTestMusicHandler(t)
	const boundary = "music-upload-limit-test"
	prefix := "--" + boundary + "\r\n" +
		"Content-Disposition: form-data; name=\"file\"; filename=\"large.mp3\"\r\n" +
		"Content-Type: audio/mpeg\r\n\r\n"
	suffix := "\r\n--" + boundary + "--\r\n"
	fileSize := int64(maxUploadBytes + 1)
	body := io.MultiReader(
		bytes.NewBufferString(prefix),
		io.LimitReader(zeroReader{}, fileSize),
		bytes.NewBufferString(suffix),
	)

	request := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	request.ContentLength = int64(len(prefix)+len(suffix)) + fileSize
	request.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	withTestUser(request, "1", "alice")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized upload status = %d, want 413: %s", response.Code, response.Body.String())
	}
}

func TestUploadStoresMP3AndExtractsDuration(t *testing.T) {
	store, handler, uploadDir := newTestMusicHandler(t)
	silence := mp3.MakeSilence()
	defer silence.Close()
	decoder := mp3.NewDecoder(silence)
	var frame mp3.Frame
	var skipped int
	if err := decoder.Decode(&frame, &skipped); err != nil {
		t.Fatalf("read generated silence frame: %v", err)
	}
	frameBytes, err := io.ReadAll(frame.Reader())
	if err != nil {
		t.Fatalf("copy silence frame: %v", err)
	}
	var audio []byte
	for range 50 {
		audio = append(audio, frameBytes...)
	}
	if duration, err := mp3Duration(bytes.NewReader(audio)); err != nil {
		t.Fatalf("parse generated MP3 fixture duration: %v", err)
	} else if duration <= 0 {
		t.Fatalf("generated MP3 duration = %v, want positive", duration)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "silence.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(audio); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	withTestUser(request, "12", "alice")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Song Song `json:"song"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Song.ID == "" || payload.Song.Title != "silence" {
		t.Fatalf("uploaded song metadata = %+v", payload.Song)
	}
	if payload.Song.DurationSeconds <= 0 {
		t.Fatalf("duration = %d, want positive duration", payload.Song.DurationSeconds)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, payload.Song.ID+".mp3")); err != nil {
		t.Fatalf("stored MP3 file: %v", err)
	}

	var userID int64
	if err := store.db.QueryRow(`SELECT uploaded_by FROM songs WHERE id = ?`, payload.Song.ID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if userID != 12 {
		t.Fatalf("uploaded_by = %d, want 12", userID)
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 0
	}
	return len(buffer), nil
}
