package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"jaylub/internal/auth"
	"net/http"
	"strings"
	"time"
)

const maxGameRunGold = 100000

var errUnknownLeaderboardSort = errors.New("unknown leaderboard sort")

type JayliveService struct {
	db *sql.DB
}

type JayliveProfile struct {
	Username          string `json:"username"`
	Gold              int    `json:"gold"`
	LifetimeKills     int    `json:"lifetimeKills"`
	SelectedCharacter string `json:"selectedCharacter"`
	DamageLevel       int    `json:"damageLevel"`
	MaxHPLevel        int    `json:"maxHpLevel"`
	AttackSpeedLevel  int    `json:"attackSpeedLevel"`
	MoveSpeedLevel    int    `json:"moveSpeedLevel"`
}

type LeaderboardEntry struct {
	Rank           int    `json:"rank"`
	Username       string `json:"username"`
	TotalKills     int    `json:"totalKills"`
	BestRunKills   int    `json:"bestRunKills"`
	BestRunSeconds int    `json:"bestRunSeconds"`
}

func NewJayliveService(db *sql.DB) *JayliveService {
	return &JayliveService{db: db}
}

func (s *JayliveService) Page(w http.ResponseWriter, r *http.Request) {
	s.ensureProfile(r)
	renderer.Render(w, r, "jaylive")
}

func (s *JayliveService) State(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	profile, err := s.profile(r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	leaderboard, err := s.leaderboard("totalKills", 10)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	writeGameJSON(w, map[string]any{
		"profile":     profile,
		"leaderboard": leaderboard,
		"shop":        shopState(profile),
	})
}

func (s *JayliveService) BuyUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Upgrade string `json:"upgrade"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&payload); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	upgrade := strings.TrimSpace(payload.Upgrade)
	column, err := upgradeColumn(upgrade)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if err := ensureJayliveProfile(tx, user); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	profile, err := scanJayliveProfile(tx.QueryRow(`
		SELECT username, gold, lifetime_kills, selected_character, damage_level, max_hp_level, attack_speed_level, move_speed_level
		FROM game_profiles
		WHERE user_id = ?
	`, user.ID))
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	level := upgradeLevel(profile, upgrade)
	cost := upgradeCost(level)
	if profile.Gold < cost {
		http.Error(w, "Not enough gold.", http.StatusBadRequest)
		return
	}

	_, err = tx.Exec(`
		UPDATE game_profiles
		SET gold = gold - ?, `+column+` = `+column+` + 1, username = ?, updated_at = ?
		WHERE user_id = ?
	`, cost, user.Username, time.Now().UTC().Format(time.RFC3339), user.ID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	profile, err = s.profile(r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writeGameJSON(w, map[string]any{
		"profile": profile,
		"shop":    shopState(profile),
	})
}

func (s *JayliveService) SubmitRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var payload struct {
		Kills           int `json:"kills"`
		Gold            int `json:"gold"`
		SurvivalSeconds int `json:"survivalSeconds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&payload); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if payload.Kills < 0 || payload.Gold < 0 || payload.Gold > maxGameRunGold || payload.SurvivalSeconds < 0 {
		http.Error(w, "Invalid run result.", http.StatusBadRequest)
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if err := ensureJayliveProfile(tx, user); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`
		UPDATE game_profiles
		SET gold = gold + ?, lifetime_kills = lifetime_kills + ?, username = ?, updated_at = ?
		WHERE user_id = ?
	`, payload.Gold, payload.Kills, user.Username, now, user.ID); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(`
		INSERT INTO game_leaderboard (user_id, username, total_kills, best_run_kills, best_run_seconds, updated_at)
		SELECT user_id, username, lifetime_kills, ?, ?, ?
		FROM game_profiles
		WHERE user_id = ?
		ON CONFLICT(user_id) DO UPDATE SET
			username = excluded.username,
			total_kills = excluded.total_kills,
			best_run_kills = max(game_leaderboard.best_run_kills, excluded.best_run_kills),
			best_run_seconds = max(game_leaderboard.best_run_seconds, excluded.best_run_seconds),
			updated_at = excluded.updated_at
	`, payload.Kills, payload.SurvivalSeconds, now, user.ID); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	profile, err := s.profile(r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	leaderboard, err := s.leaderboard("totalKills", 10)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writeGameJSON(w, map[string]any{
		"profile":     profile,
		"leaderboard": leaderboard,
		"shop":        shopState(profile),
	})
}

func (s *JayliveService) Leaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	sort := r.URL.Query().Get("sort")
	leaderboard, err := s.leaderboard(sort, 25)
	if err != nil {
		if errors.Is(err, errUnknownLeaderboardSort) {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writeGameJSON(w, map[string]any{"leaderboard": leaderboard})
}

func (s *JayliveService) ensureProfile(r *http.Request) {
	if user, ok := auth.UserFromContext(r.Context()); ok {
		_, _ = s.db.Exec(`
			INSERT INTO game_profiles (user_id, username)
			VALUES (?, ?)
			ON CONFLICT(user_id) DO UPDATE SET username = excluded.username
		`, user.ID, user.Username)
	}
}

func (s *JayliveService) profile(r *http.Request) (JayliveProfile, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return JayliveProfile{}, errors.New("unauthorized")
	}
	if _, err := s.db.Exec(`
		INSERT INTO game_profiles (user_id, username)
		VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET username = excluded.username
	`, user.ID, user.Username); err != nil {
		return JayliveProfile{}, err
	}
	return scanJayliveProfile(s.db.QueryRow(`
		SELECT username, gold, lifetime_kills, selected_character, damage_level, max_hp_level, attack_speed_level, move_speed_level
		FROM game_profiles
		WHERE user_id = ?
	`, user.ID))
}

func (s *JayliveService) leaderboard(sort string, limit int) ([]LeaderboardEntry, error) {
	orderColumn, err := leaderboardOrderColumn(sort)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT username, total_kills, best_run_kills, best_run_seconds
		FROM game_leaderboard
		ORDER BY `+orderColumn+` DESC, username ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]LeaderboardEntry, 0, limit)
	rank := 1
	for rows.Next() {
		var entry LeaderboardEntry
		entry.Rank = rank
		if err := rows.Scan(&entry.Username, &entry.TotalKills, &entry.BestRunKills, &entry.BestRunSeconds); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
		rank++
	}
	return entries, rows.Err()
}

func leaderboardOrderColumn(sort string) (string, error) {
	switch sort {
	case "", "totalKills":
		return "total_kills", nil
	case "bestRunKills":
		return "best_run_kills", nil
	case "bestRunSeconds":
		return "best_run_seconds", nil
	default:
		return "", errUnknownLeaderboardSort
	}
}

func ensureJayliveProfile(tx *sql.Tx, user auth.User) error {
	_, err := tx.Exec(`
		INSERT INTO game_profiles (user_id, username)
		VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET username = excluded.username
	`, user.ID, user.Username)
	return err
}

func scanJayliveProfile(row interface{ Scan(dest ...any) error }) (JayliveProfile, error) {
	var profile JayliveProfile
	err := row.Scan(
		&profile.Username,
		&profile.Gold,
		&profile.LifetimeKills,
		&profile.SelectedCharacter,
		&profile.DamageLevel,
		&profile.MaxHPLevel,
		&profile.AttackSpeedLevel,
		&profile.MoveSpeedLevel,
	)
	return profile, err
}

func upgradeColumn(upgrade string) (string, error) {
	switch upgrade {
	case "damage":
		return "damage_level", nil
	case "maxHp":
		return "max_hp_level", nil
	case "attackSpeed":
		return "attack_speed_level", nil
	case "moveSpeed":
		return "move_speed_level", nil
	default:
		return "", errors.New("Unknown upgrade.")
	}
}

func upgradeLevel(profile JayliveProfile, upgrade string) int {
	switch upgrade {
	case "damage":
		return profile.DamageLevel
	case "maxHp":
		return profile.MaxHPLevel
	case "attackSpeed":
		return profile.AttackSpeedLevel
	case "moveSpeed":
		return profile.MoveSpeedLevel
	default:
		return 0
	}
}

func upgradeCost(level int) int {
	return 35 + level*28 + level*level*7
}

func shopState(profile JayliveProfile) map[string]map[string]int {
	return map[string]map[string]int{
		"damage": {
			"level": profile.DamageLevel,
			"cost":  upgradeCost(profile.DamageLevel),
		},
		"maxHp": {
			"level": profile.MaxHPLevel,
			"cost":  upgradeCost(profile.MaxHPLevel),
		},
		"attackSpeed": {
			"level": profile.AttackSpeedLevel,
			"cost":  upgradeCost(profile.AttackSpeedLevel),
		},
		"moveSpeed": {
			"level": profile.MoveSpeedLevel,
			"cost":  upgradeCost(profile.MoveSpeedLevel),
		},
	}
}

func writeGameJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
