package music

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"jaylub/internal/views"

	"github.com/dhowden/tag"
	"github.com/tcolgate/mp3"
)

const (
	maxUploadBytes       = 50 << 20
	multipartOverheadMax = 64 << 10
	multipartMemoryLimit = 2 << 20
)

//go:embed web/index.html web/app.js web/styles.css web/icon.png
var webFiles embed.FS

type Handler struct {
	store        *Store
	uploadDir    string
	mux          *http.ServeMux
	pageTemplate *template.Template
	profileData  func(User) views.PageData
}

type Song struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	Album           string `json:"album"`
	DurationSeconds int    `json:"duration_seconds"`
	UploadedBy      int64  `json:"uploaded_by"`
	CreatedAt       string `json:"created_at"`
	IsFavorite      bool   `json:"is_favorite"`
}

func NewHandler(store *Store, uploadDir string, profileData func(User) views.PageData) *Handler {
	h := &Handler{
		store:        store,
		uploadDir:    uploadDir,
		mux:          http.NewServeMux(),
		pageTemplate: template.Must(template.ParseFS(webFiles, "web/index.html")),
		profileData:  profileData,
	}
	h.mux.HandleFunc("GET /", h.index)
	h.mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://jaylub.com/login", http.StatusSeeOther)
	})
	h.mux.HandleFunc("GET /terms", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://jaylub.com/terms", http.StatusSeeOther)
	})
	h.mux.HandleFunc("GET /assets/app.js", h.asset("web/app.js", "text/javascript; charset=utf-8"))
	h.mux.HandleFunc("GET /assets/styles.css", h.asset("web/styles.css", "text/css; charset=utf-8"))
	h.mux.HandleFunc("GET /favicon.ico", h.asset("web/icon.png", "image/png"))
	h.mux.HandleFunc("GET /api/songs", h.listSongs)
	h.mux.HandleFunc("GET /api/favorites", h.listFavorites)
	h.mux.HandleFunc("POST /api/favorites", h.addFavorite)
	h.mux.HandleFunc("DELETE /api/favorites/{id}", h.removeFavorite)
	h.mux.HandleFunc("POST /api/upload", h.uploadSong)
	h.mux.HandleFunc("GET /api/stream", h.streamSong)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; media-src 'self'; connect-src 'self'; form-action 'self' https://jaylub.com; base-uri 'none'; frame-ancestors 'none'; object-src 'none'")
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	user, _ := userFromRequest(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := h.pageTemplate.Execute(w, h.profileData(user)); err != nil {
		log.Printf("render music page: %v", err)
		http.Error(w, "Could not load music app.", http.StatusInternalServerError)
	}
}

func (h *Handler) asset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		file, err := webFiles.ReadFile(name)
		if err != nil {
			http.Error(w, "Asset not found.", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(file)
	}
}

func (h *Handler) listSongs(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromRequest(r)
	if err := h.store.ensureUser(user.ID, user.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load songs.")
		return
	}
	rows, err := h.store.db.Query(`
		SELECT s.id, s.title, s.artist, s.album, s.duration_seconds,
			s.uploaded_by, s.created_at,
			EXISTS(SELECT 1 FROM user_favorites f WHERE f.song_id = s.id AND f.user_id = ?)
		FROM songs s
		ORDER BY s.created_at DESC, s.id DESC
	`, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load songs.")
		return
	}
	defer rows.Close()
	songs, err := scanSongs(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load songs.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"songs": songs})
}

func (h *Handler) listFavorites(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromRequest(r)
	if err := h.store.ensureUser(user.ID, user.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load favorites.")
		return
	}
	rows, err := h.store.db.Query(`
		SELECT s.id, s.title, s.artist, s.album, s.duration_seconds,
			s.uploaded_by, s.created_at, 1
		FROM user_favorites f
		JOIN songs s ON s.id = f.song_id
		WHERE f.user_id = ?
		ORDER BY f.created_at DESC, s.title COLLATE NOCASE
	`, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load favorites.")
		return
	}
	defer rows.Close()
	songs, err := scanSongs(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load favorites.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"songs": songs})
}

func scanSongs(rows *sql.Rows) ([]Song, error) {
	songs := make([]Song, 0)
	for rows.Next() {
		var song Song
		if err := rows.Scan(
			&song.ID,
			&song.Title,
			&song.Artist,
			&song.Album,
			&song.DurationSeconds,
			&song.UploadedBy,
			&song.CreatedAt,
			&song.IsFavorite,
		); err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	return songs, rows.Err()
}

func (h *Handler) addFavorite(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Request origin is not allowed.")
		return
	}
	user, _ := userFromRequest(r)
	var payload struct {
		SongID string `json:"song_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid favorite request.")
		return
	}
	if !validUUID(payload.SongID) {
		writeError(w, http.StatusBadRequest, "Invalid song ID.")
		return
	}
	if err := h.store.ensureUser(user.ID, user.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save favorite.")
		return
	}
	_, err := h.store.db.Exec(`
		INSERT INTO user_favorites (user_id, song_id) VALUES (?, ?)
		ON CONFLICT(user_id, song_id) DO NOTHING
	`, user.ID, payload.SongID)
	if err != nil {
		var exists int
		if queryErr := h.store.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM songs WHERE id = ?)`, payload.SongID).Scan(&exists); queryErr != nil {
			writeError(w, http.StatusInternalServerError, "Could not save favorite.")
			return
		}
		if exists == 0 {
			writeError(w, http.StatusNotFound, "Song not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not save favorite.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) removeFavorite(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Request origin is not allowed.")
		return
	}
	songID := r.PathValue("id")
	if !validUUID(songID) {
		writeError(w, http.StatusBadRequest, "Invalid song ID.")
		return
	}
	user, _ := userFromRequest(r)
	if _, err := h.store.db.Exec(`DELETE FROM user_favorites WHERE user_id = ? AND song_id = ?`, user.ID, songID); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove favorite.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) uploadSong(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Request origin is not allowed.")
		return
	}
	user, _ := userFromRequest(r)
	if err := os.MkdirAll(h.uploadDir, 0750); err != nil {
		writeError(w, http.StatusInternalServerError, "Music storage is unavailable.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+multipartOverheadMax)
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		status := http.StatusBadRequest
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, uploadError(err))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		writeError(w, http.StatusBadRequest, "Upload exactly one MP3 file.")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Choose an MP3 file to upload.")
		return
	}
	defer file.Close()
	if header.Size > maxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "MP3 files must be 50 MB or smaller.")
		return
	}
	if strings.ToLower(filepath.Ext(header.Filename)) != ".mp3" {
		writeError(w, http.StatusBadRequest, "Only .mp3 files are accepted.")
		return
	}

	tempFile, err := os.CreateTemp(h.uploadDir, ".music-upload-*.tmp")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save uploaded file.")
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	written, err := io.Copy(tempFile, io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		tempFile.Close()
		writeError(w, http.StatusBadRequest, uploadError(err))
		return
	}
	if written > maxUploadBytes {
		tempFile.Close()
		writeError(w, http.StatusRequestEntityTooLarge, "MP3 files must be 50 MB or smaller.")
		return
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		writeError(w, http.StatusInternalServerError, "Could not save uploaded file.")
		return
	}
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		tempFile.Close()
		writeError(w, http.StatusInternalServerError, "Could not inspect uploaded file.")
		return
	}
	metadata, err := tag.ReadFrom(tempFile)
	duration, err := mp3Duration(tempFile)
	if err != nil {
		tempFile.Close()
		writeError(w, http.StatusBadRequest, "The uploaded file is not a readable MP3.")
		return
	}
	if err := tempFile.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not finish uploaded file.")
		return
	}

	title, artist, album := "", "", ""
	if metadata != nil {
		title = strings.TrimSpace(metadata.Title())
		artist = strings.TrimSpace(metadata.Artist())
		album = strings.TrimSpace(metadata.Album())
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	}
	if artist == "" {
		artist = "Unknown Artist"
	}
	if album == "" {
		album = "Unknown Album"
	}
	songID, err := newUUID()
	if err != nil {
		os.Remove(tempPath)
		writeError(w, http.StatusInternalServerError, "Could not create song identifier.")
		return
	}
	fileName := songID + ".mp3"
	finalPath := filepath.Join(h.uploadDir, fileName)

	if err := h.store.ensureUser(user.ID, user.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save song.")
		return
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not store uploaded song.")
		return
	}
	_, err = h.store.db.Exec(`
		INSERT INTO songs (id, title, artist, album, duration_seconds, file_path, uploaded_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, songID, title, artist, album, int(duration.Seconds()), fileName, user.ID)
	if err != nil {
		os.Remove(finalPath)
		writeError(w, http.StatusInternalServerError, "Could not save song metadata.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"song": Song{
			ID: songID, Title: title, Artist: artist, Album: album,
			DurationSeconds: int(duration.Seconds()), UploadedBy: user.ID,
		},
	})
}

func mp3Duration(file io.ReadSeeker) (time.Duration, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	decoder := mp3.NewDecoder(file)
	var duration time.Duration
	var frame mp3.Frame
	var skipped int
	for {
		err := decoder.Decode(&frame, &skipped)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}
		duration += frame.Duration()
	}
	if duration <= 0 {
		return 0, fmt.Errorf("MP3 contains no frames")
	}
	return duration, nil
}

func uploadError(err error) string {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return "MP3 files must be 50 MB or smaller."
	}
	return "Could not read the MP3 upload."
}

func (h *Handler) streamSong(w http.ResponseWriter, r *http.Request) {
	songID := r.URL.Query().Get("id")
	if !validUUID(songID) {
		writeError(w, http.StatusBadRequest, "Invalid song ID.")
		return
	}
	var fileName string
	if err := h.store.db.QueryRow(`SELECT file_path FROM songs WHERE id = ?`, songID).Scan(&fileName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Song not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not load song.")
		return
	}
	if filepath.Base(fileName) != fileName || strings.ToLower(filepath.Ext(fileName)) != ".mp3" {
		writeError(w, http.StatusInternalServerError, "Song file path is invalid.")
		return
	}
	path := filepath.Join(h.uploadDir, fileName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "Song file is missing.")
		} else {
			writeError(w, http.StatusInternalServerError, "Could not access song file.")
		}
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, path)
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if r.URL.Scheme != "" && r.URL.Host != "" {
		return origin == r.URL.Scheme+"://"+r.URL.Host
	}
	return strings.EqualFold(origin, "https://"+r.Host) || strings.EqualFold(origin, "http://"+r.Host)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func newUUID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	encoded := hex.EncodeToString(id[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}
