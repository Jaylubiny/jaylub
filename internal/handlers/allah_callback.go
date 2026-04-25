package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type User struct {
	AccessToken string `json:"access_token"`
	Time        string `json:"time"`
}

var (
	dbPath     = "internal/db/users.json"
	lock       = sync.Mutex{}
	usedCodes  = make(map[string]bool)

	CLIENT_ID     = os.Getenv("CLIENT_ID")
	CLIENT_SECRET = os.Getenv("CLIENT_SECRET")
	REDIRECT_URI  = os.Getenv("REDIRECT_URI")
	BOT_TOKEN     = os.Getenv("BOT_TOKEN")
	GUILD_ID      = os.Getenv("GUILD_ID")
	WEBHOOK_URL   = os.Getenv("WEBHOOK_URL")
)

func loadData() map[string]User {
	data := make(map[string]User)

	f, err := os.ReadFile(dbPath)
	if err != nil {
		return data
	}

	json.Unmarshal(f, &data)
	return data
}

func saveData(data map[string]User) {
	lock.Lock()
	defer lock.Unlock()

	os.MkdirAll("internal/db", 0755)

	b, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(dbPath, b, 0644)
}


func sendWebhook(content string) {
	if WEBHOOK_URL == "" {
		return
	}

	http.Post(
		WEBHOOK_URL,
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"content":"%s"}`, content)),
	)
}



func AllahCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")

	if code == "" {
		http.Error(w, "Missing code", 400)
		return
	}

	if usedCodes[code] {
		http.Error(w, "Code already used", 429)
		return
	}
	usedCodes[code] = true

	resp, _ := http.PostForm(
		"https://discord.com/api/oauth2/token",
		map[string][]string{
			"client_id":     {CLIENT_ID},
			"client_secret": {CLIENT_SECRET},
			"grant_type":    {"authorization_code"},
			"code":          {code},
			"redirect_uri":  {REDIRECT_URI},
		},
	)

	body, _ := io.ReadAll(resp.Body)

	var tokenData map[string]any
	json.Unmarshal(body, &tokenData)

	accessToken, ok := tokenData["access_token"].(string)
	if !ok {
		http.Error(w, "Token error", 400)
		return
	}

	req, _ := http.NewRequest("GET", "https://discord.com/api/users/@me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	res, _ := client.Do(req)

	userBody, _ := io.ReadAll(res.Body)

	var user map[string]any
	json.Unmarshal(userBody, &user)

	userID := user["id"].(string)
	username := fmt.Sprintf("%v#%v", user["username"], user["discriminator"])

	db := loadData()
	db[userID] = User{
		AccessToken: accessToken,
		Time:        time.Now().UTC().String(),
	}
	saveData(db)

	sendWebhook(fmt.Sprintf(
		"New OAuth user\nUser: %s\nID: %s\nTotal: %d",
		username, userID, len(db),
	))

	w.Write([]byte("Authorized: " + username))
}