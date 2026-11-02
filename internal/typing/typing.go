package typing

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"jaylub/internal/auth"
	"jaylub/internal/views"
)

const (
	Mode15Seconds    = "15s"
	Mode30Seconds    = "30s"
	Mode60Seconds    = "60s"
	leaderboardLimit = 20
	maxScoreBodySize = 2048
)

var validModes = map[string]struct{}{
	Mode15Seconds: {},
	Mode30Seconds: {},
	Mode60Seconds: {},
}

//go:embed schema.sql
var schema string

type Service struct {
	db       *sql.DB
	renderer *views.Renderer
}

type ScorePayload struct {
	WPM      int     `json:"wpm"`
	Accuracy float64 `json:"accuracy"`
	Mode     string  `json:"mode"`
}

type LeaderboardEntry struct {
	Rank      int     `json:"rank"`
	Username  string  `json:"username"`
	WPM       int     `json:"wpm"`
	Accuracy  float64 `json:"accuracy"`
	CreatedAt string  `json:"created_at"`
}

type UserStats struct {
	PBWPM           int     `json:"pb_wpm"`
	AverageAccuracy float64 `json:"average_accuracy"`
	TestCount       int     `json:"test_count"`
}

func NewService(db *sql.DB) (*Service, error) {
	if db == nil {
		return nil, errors.New("typing service requires a database")
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("initialize typing scores schema: %w", err)
	}
	if err := migrateTypingScores(db); err != nil {
		return nil, fmt.Errorf("migrate typing scores schema: %w", err)
	}
	renderer := views.NewRenderer("web/templates")
	renderer.SetDB(db)
	return &Service{db: db, renderer: renderer}, nil
}

func migrateTypingScores(db *sql.DB) error {
	var tableSQL string
	if err := db.QueryRow(`
		SELECT sql FROM sqlite_master
		WHERE type = 'table' AND name = 'typing_scores'
	`).Scan(&tableSQL); err != nil {
		return fmt.Errorf("read typing scores schema: %w", err)
	}
	if strings.Contains(tableSQL, "'15s'") {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin typing scores migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		CREATE TABLE typing_scores_v2 (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			wpm INTEGER NOT NULL CHECK (wpm BETWEEN 5 AND 260),
			accuracy REAL NOT NULL CHECK (accuracy BETWEEN 0 AND 100),
			mode TEXT NOT NULL CHECK (mode IN ('15s', '30s', '60s', 'code-go')),
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		INSERT INTO typing_scores_v2 (id, user_id, wpm, accuracy, mode, created_at)
			SELECT id, user_id, wpm, accuracy, mode, created_at FROM typing_scores;
		DROP TABLE typing_scores;
		ALTER TABLE typing_scores_v2 RENAME TO typing_scores;
		CREATE INDEX idx_typing_scores_user_wpm
			ON typing_scores(user_id, wpm DESC);
		CREATE INDEX idx_typing_scores_mode_wpm
			ON typing_scores(mode, wpm DESC);
	`); err != nil {
		return fmt.Errorf("rebuild typing scores table: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit typing scores migration: %w", err)
	}
	return nil
}

func (s *Service) Page(w http.ResponseWriter, r *http.Request) {
	s.renderer.Render(w, r, "typing")
}

func (s *Service) SettingsPage(w http.ResponseWriter, r *http.Request) {
	s.renderer.Render(w, r, "typing_settings")
}

func (s *Service) Score(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed.")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Authentication required.")
		return
	}

	var payload ScorePayload
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxScoreBodySize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid score payload.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "Request must contain one JSON object.")
		return
	}
	if payload.WPM < 5 || payload.WPM > 260 ||
		math.IsNaN(payload.Accuracy) || math.IsInf(payload.Accuracy, 0) ||
		payload.Accuracy < 0 || payload.Accuracy > 100 {
		writeJSONError(w, http.StatusBadRequest, "Score is outside the allowed range.")
		return
	}
	if _, ok := validModes[payload.Mode]; !ok {
		writeJSONError(w, http.StatusBadRequest, "Unknown typing mode.")
		return
	}

	isNewPB, err := s.saveScore(user.ID, payload)
	if err != nil {
		log.Printf("save typing score for user %d: %v", user.ID, err)
		writeJSONError(w, http.StatusInternalServerError, "Could not save typing score.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"success":   true,
		"is_new_pb": isNewPB,
	})
}

func (s *Service) saveScore(userID int64, score ScorePayload) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin score transaction: %w", err)
	}
	defer tx.Rollback()

	var previousPB int
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(wpm), 0)
		FROM typing_scores
		WHERE user_id = ? AND mode = ?
	`, userID, score.Mode).Scan(&previousPB)
	if err != nil {
		return false, fmt.Errorf("load previous personal best: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO typing_scores (user_id, wpm, accuracy, mode)
		VALUES (?, ?, ?, ?)
	`, userID, score.WPM, score.Accuracy, score.Mode); err != nil {
		return false, fmt.Errorf("insert typing score: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit typing score: %w", err)
	}
	return score.WPM > previousPB, nil
}

func (s *Service) Leaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed.")
		return
	}
	mode := r.URL.Query().Get("mode")
	if _, ok := validModes[mode]; !ok {
		writeJSONError(w, http.StatusBadRequest, "Unknown typing mode.")
		return
	}

	rows, err := s.db.Query(`
		WITH best_scores AS (
			SELECT
				users.username,
				typing_scores.wpm,
				typing_scores.accuracy,
				typing_scores.created_at,
				ROW_NUMBER() OVER (
					PARTITION BY typing_scores.user_id
					ORDER BY typing_scores.wpm DESC,
						typing_scores.accuracy DESC,
						typing_scores.created_at ASC,
						typing_scores.id ASC
				) AS personal_rank
			FROM typing_scores
			JOIN users ON users.id = typing_scores.user_id
			WHERE typing_scores.mode = ?
		),
		ranked_scores AS (
			SELECT
				username,
				wpm,
				accuracy,
				created_at,
				ROW_NUMBER() OVER (
					ORDER BY wpm DESC, accuracy DESC, created_at ASC, username COLLATE NOCASE ASC
				) AS rank
			FROM best_scores
			WHERE personal_rank = 1
		)
		SELECT rank, username, wpm, accuracy, created_at
		FROM ranked_scores
		ORDER BY rank
		LIMIT ?
	`, mode, leaderboardLimit)
	if err != nil {
		log.Printf("load typing leaderboard for mode %q: %v", mode, err)
		writeJSONError(w, http.StatusInternalServerError, "Could not load leaderboard.")
		return
	}
	defer rows.Close()

	entries := make([]LeaderboardEntry, 0, leaderboardLimit)
	for rows.Next() {
		var entry LeaderboardEntry
		var createdAt time.Time
		if err := rows.Scan(&entry.Rank, &entry.Username, &entry.WPM, &entry.Accuracy, &createdAt); err != nil {
			log.Printf("scan typing leaderboard row: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Could not load leaderboard.")
			return
		}
		entry.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		log.Printf("iterate typing leaderboard rows: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Could not load leaderboard.")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Service) UserStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed.")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Authentication required.")
		return
	}
	mode := r.URL.Query().Get("mode")
	if _, ok := validModes[mode]; !ok {
		writeJSONError(w, http.StatusBadRequest, "Unknown typing mode.")
		return
	}

	var stats UserStats
	err := s.db.QueryRow(`
		SELECT COALESCE(MAX(wpm), 0), COALESCE(AVG(accuracy), 0), COUNT(*)
		FROM typing_scores
		WHERE user_id = ? AND mode = ?
	`, user.ID, mode).Scan(&stats.PBWPM, &stats.AverageAccuracy, &stats.TestCount)
	if err != nil {
		log.Printf("load typing stats for user %d mode %q: %v", user.ID, mode, err)
		writeJSONError(w, http.StatusInternalServerError, "Could not load personal stats.")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write typing JSON response: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(message)})
}
