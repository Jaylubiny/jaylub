package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"jaylub/internal/auth"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxGameRunGold = 1000000

var errUnknownLeaderboardSort = errors.New("unknown leaderboard sort")

type JayliveService struct {
	db         *sql.DB
	runMu      sync.Mutex
	activeRuns map[int64]activeJayliveRun
}

type activeJayliveRun struct {
	token     string
	startedAt time.Time
}

type JayliveProfile struct {
	Username            string `json:"username"`
	Gold                int    `json:"gold"`
	LifetimeKills       int    `json:"lifetimeKills"`
	SelectedCharacter   string `json:"selectedCharacter"`
	GoblinUnlocked      bool   `json:"goblinJaylubUnlocked"`
	DamageLevel         int    `json:"damageLevel"`
	PiercingLevel       int    `json:"piercingLevel"`
	MaxHPLevel          int    `json:"maxHpLevel"`
	AttackSpeedLevel    int    `json:"attackSpeedLevel"`
	MoveSpeedLevel      int    `json:"moveSpeedLevel"`
	AbilityDamageLevel  int    `json:"abilityDamageLevel"`
	AuraDamageLevel     int    `json:"auraDamageLevel"`
	FootballDamageLevel int    `json:"footballDamageLevel"`
	BikeDamageLevel     int    `json:"bikeDamageLevel"`
	PigeonDamageLevel   int    `json:"pigeonDamageLevel"`
	GameLevel           int    `json:"gameLevel"`
	GameXP              int    `json:"gameXP"`
}

type LeaderboardEntry struct {
	Rank           int    `json:"rank"`
	Username       string `json:"username"`
	TotalKills     int    `json:"totalKills"`
	BestRunKills   int    `json:"bestRunKills"`
	BestRunSeconds int    `json:"bestRunSeconds"`
	Level          int    `json:"level"`
}

func NewJayliveService(db *sql.DB) *JayliveService {
	return &JayliveService{
		db:         db,
		activeRuns: make(map[int64]activeJayliveRun),
	}
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
		SELECT username, gold, lifetime_kills, selected_character, goblin_jaylub_unlocked, damage_level, piercing_level, max_hp_level, attack_speed_level, move_speed_level, ability_damage_level, aura_damage_level, football_damage_level, bike_damage_level, pigeon_damage_level, game_level, game_xp
		FROM game_profiles
		WHERE user_id = ?
	`, user.ID))
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	level := upgradeLevel(profile, upgrade)
	if upgrade == "piercing" && !profile.GoblinUnlocked {
		http.Error(w, "Goblin Jaylub is locked.", http.StatusBadRequest)
		return
	}
	if upgrade == "piercing" && level >= 10 {
		http.Error(w, "Piercing is already maxed.", http.StatusBadRequest)
		return
	}
	cost := upgradeCost(level)
	if isAbilityDamageUpgrade(upgrade) {
		cost = abilityDamageUpgradeCost(upgrade, level)
	}
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

func (s *JayliveService) Character(w http.ResponseWriter, r *http.Request) {
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
		Action    string `json:"action"`
		Character string `json:"character"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&payload); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
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
		SELECT username, gold, lifetime_kills, selected_character, goblin_jaylub_unlocked, damage_level, piercing_level, max_hp_level, attack_speed_level, move_speed_level, ability_damage_level, aura_damage_level, football_damage_level, bike_damage_level, pigeon_damage_level, game_level, game_xp
		FROM game_profiles
		WHERE user_id = ?
	`, user.ID))
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	switch payload.Action {
	case "buy":
		if payload.Character != "goblin_jaylub" {
			http.Error(w, "Unknown character.", http.StatusBadRequest)
			return
		}
		if profile.GoblinUnlocked {
			break
		}
		if profile.Gold < 100 {
			http.Error(w, "Not enough gold.", http.StatusBadRequest)
			return
		}
		_, err = tx.Exec(`
			UPDATE game_profiles
			SET gold = gold - 100, goblin_jaylub_unlocked = 1, selected_character = 'goblin_jaylub', username = ?, updated_at = ?
			WHERE user_id = ?
		`, user.Username, time.Now().UTC().Format(time.RFC3339), user.ID)
	case "select":
		if payload.Character != "jaylub" && payload.Character != "goblin_jaylub" {
			http.Error(w, "Unknown character.", http.StatusBadRequest)
			return
		}
		if payload.Character == "goblin_jaylub" && !profile.GoblinUnlocked {
			http.Error(w, "Character is locked.", http.StatusBadRequest)
			return
		}
		_, err = tx.Exec(`
			UPDATE game_profiles
			SET selected_character = ?, username = ?, updated_at = ?
			WHERE user_id = ?
		`, payload.Character, user.Username, time.Now().UTC().Format(time.RFC3339), user.ID)
	default:
		http.Error(w, "Unknown action.", http.StatusBadRequest)
		return
	}
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

func (s *JayliveService) StartRun(w http.ResponseWriter, r *http.Request) {
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
	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	token, err := randomRunToken()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.runMu.Lock()
	s.activeRuns[user.ID] = activeJayliveRun{
		token:     token,
		startedAt: time.Now().UTC(),
	}
	s.runMu.Unlock()

	writeGameJSON(w, map[string]string{"runToken": token})
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
		Kills           int    `json:"kills"`
		Gold            int    `json:"gold"`
		SurvivalSeconds int    `json:"survivalSeconds"`
		RunToken        string `json:"runToken"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&payload); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if payload.Kills < 0 || payload.Gold < 0 || payload.Gold > maxGameRunGold || payload.SurvivalSeconds < 0 || payload.RunToken == "" {
		http.Error(w, "Invalid run result.", http.StatusBadRequest)
		return
	}

	if err := s.validateRunResult(user.ID, payload.RunToken, payload.SurvivalSeconds, payload.Kills, payload.Gold); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	var currentLevel int
	var currentXP int
	if err := tx.QueryRow(`SELECT game_level, game_xp FROM game_profiles WHERE user_id = ?`, user.ID).Scan(&currentLevel, &currentXP); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	newLevel, newXP := advanceJayliveLevel(currentLevel, currentXP, payload.Kills)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`
		UPDATE game_profiles
		SET gold = gold + ?, lifetime_kills = lifetime_kills + ?, game_level = ?, game_xp = ?, username = ?, updated_at = ?
		WHERE user_id = ?
	`, payload.Gold, payload.Kills, newLevel, newXP, user.Username, now, user.ID); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(`
		INSERT INTO game_leaderboard (user_id, username, total_kills, best_run_kills, best_run_seconds, level, updated_at)
		SELECT user_id, username, lifetime_kills, ?, ?, game_level, ?
		FROM game_profiles
		WHERE user_id = ?
		ON CONFLICT(user_id) DO UPDATE SET
			username = excluded.username,
			total_kills = excluded.total_kills,
			best_run_kills = max(game_leaderboard.best_run_kills, excluded.best_run_kills),
			best_run_seconds = max(game_leaderboard.best_run_seconds, excluded.best_run_seconds),
			level = excluded.level,
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

func (s *JayliveService) validateRunResult(userID int64, token string, survivalSeconds, kills, gold int) error {
	s.runMu.Lock()
	run, ok := s.activeRuns[userID]
	if ok && run.token == token {
		delete(s.activeRuns, userID)
	}
	s.runMu.Unlock()

	if !ok || run.token != token {
		return errors.New("Invalid or expired run.")
	}

	actualSeconds := int(time.Since(run.startedAt).Seconds())
	if survivalSeconds > actualSeconds+5 {
		return errors.New("Run time is not valid.")
	}
	if kills > maxPlausibleRunKills(survivalSeconds) {
		return errors.New("Run kills are not valid.")
	}
	if gold > maxPlausibleRunGold(survivalSeconds, kills) {
		return errors.New("Run gold is not valid.")
	}
	return nil
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
		SELECT username, gold, lifetime_kills, selected_character, goblin_jaylub_unlocked, damage_level, piercing_level, max_hp_level, attack_speed_level, move_speed_level, ability_damage_level, aura_damage_level, football_damage_level, bike_damage_level, pigeon_damage_level, game_level, game_xp
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
		SELECT username, total_kills, best_run_kills, best_run_seconds, level
		FROM game_leaderboard
		WHERE `+orderColumn+` > 0
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
		if err := rows.Scan(&entry.Username, &entry.TotalKills, &entry.BestRunKills, &entry.BestRunSeconds, &entry.Level); err != nil {
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
	case "level":
		return "level", nil
	default:
		return "", errUnknownLeaderboardSort
	}
}

func maxPlausibleRunKills(survivalSeconds int) int {
	return 80 + survivalSeconds*40
}

func maxPlausibleRunGold(survivalSeconds, kills int) int {
	itemChestGold := (survivalSeconds/60 + 1) * 100
	levelRewardGold := (kills/100 + 1) * 150
	return kills*12 + maxPlausibleBossGold(survivalSeconds) + itemChestGold + levelRewardGold + 150
}

func maxPlausibleBossGold(survivalSeconds int) int {
	encounters := survivalSeconds / 180
	if encounters <= 0 {
		return 0
	}

	total := 0
	rewards := []int{250, 500, 1000}
	for i := 0; i < encounters; i++ {
		if i < len(rewards) {
			total += rewards[i]
		} else {
			total += rewards[len(rewards)-1]
		}
	}
	return total
}

func advanceJayliveLevel(level, xp, gainedXP int) (int, int) {
	if level < 0 {
		level = 0
	}
	if xp < 0 {
		xp = 0
	}
	xp += gainedXP
	for xp >= jayliveLevelRequirement(level+1) {
		xp -= jayliveLevelRequirement(level + 1)
		level++
	}
	return level, xp
}

func jayliveLevelRequirement(level int) int {
	if level < 1 {
		level = 1
	}
	step := float64(level - 1)
	return 100 + (level-1)*70 + int(math.Pow(step, 1.35)*30)
}

func randomRunToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
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
	var goblinUnlocked int
	err := row.Scan(
		&profile.Username,
		&profile.Gold,
		&profile.LifetimeKills,
		&profile.SelectedCharacter,
		&goblinUnlocked,
		&profile.DamageLevel,
		&profile.PiercingLevel,
		&profile.MaxHPLevel,
		&profile.AttackSpeedLevel,
		&profile.MoveSpeedLevel,
		&profile.AbilityDamageLevel,
		&profile.AuraDamageLevel,
		&profile.FootballDamageLevel,
		&profile.BikeDamageLevel,
		&profile.PigeonDamageLevel,
		&profile.GameLevel,
		&profile.GameXP,
	)
	profile.GoblinUnlocked = goblinUnlocked == 1
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
	case "piercing":
		return "piercing_level", nil
	case "abilityDamage":
		return "ability_damage_level", nil
	case "auraDamage":
		return "aura_damage_level", nil
	case "footballDamage":
		return "football_damage_level", nil
	case "bikeDamage":
		return "bike_damage_level", nil
	case "pigeonDamage":
		return "pigeon_damage_level", nil
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
	case "piercing":
		return profile.PiercingLevel
	case "abilityDamage":
		return profile.AbilityDamageLevel
	case "auraDamage":
		return profile.AuraDamageLevel
	case "footballDamage":
		return profile.FootballDamageLevel
	case "bikeDamage":
		return profile.BikeDamageLevel
	case "pigeonDamage":
		return profile.PigeonDamageLevel
	default:
		return 0
	}
}

func upgradeCost(level int) int {
	return 35 + level*28 + level*level*7
}

func abilityDamageUpgradeCost(upgrade string, level int) int {
	base := 220
	step := 130
	curve := 55
	switch upgrade {
	case "footballDamage":
		base, step, curve = 320, 175, 78
	case "bikeDamage":
		base, step, curve = 470, 235, 110
	case "pigeonDamage":
		base, step, curve = 700, 330, 160
	}
	return base + level*step + level*level*curve
}

func isAbilityDamageUpgrade(upgrade string) bool {
	return upgrade == "abilityDamage" ||
		upgrade == "auraDamage" ||
		upgrade == "footballDamage" ||
		upgrade == "bikeDamage" ||
		upgrade == "pigeonDamage"
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
		"abilityDamage": {
			"level": profile.AbilityDamageLevel,
			"cost":  abilityDamageUpgradeCost("abilityDamage", profile.AbilityDamageLevel),
		},
		"auraDamage": {
			"level": profile.AuraDamageLevel,
			"cost":  abilityDamageUpgradeCost("auraDamage", profile.AuraDamageLevel),
		},
		"footballDamage": {
			"level": profile.FootballDamageLevel,
			"cost":  abilityDamageUpgradeCost("footballDamage", profile.FootballDamageLevel),
		},
		"bikeDamage": {
			"level": profile.BikeDamageLevel,
			"cost":  abilityDamageUpgradeCost("bikeDamage", profile.BikeDamageLevel),
		},
		"pigeonDamage": {
			"level": profile.PigeonDamageLevel,
			"cost":  abilityDamageUpgradeCost("pigeonDamage", profile.PigeonDamageLevel),
		},
		"piercing": {
			"level": profile.PiercingLevel,
			"cost":  upgradeCost(profile.PiercingLevel),
			"max":   10,
		},
	}
}

func writeGameJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
