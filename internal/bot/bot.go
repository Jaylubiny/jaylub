package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/diamondburned/arikawa/v3/voice"
)

type CommandHandler func(ctx context.Context, ev *gateway.InteractionCreateEvent, data *discord.CommandInteraction) error

type Bot struct {
	state          *state.State
	appID          discord.AppID
	cooldownPeriod time.Duration
	userCooldowns  sync.Map

	voiceMutex    sync.Mutex
	voiceSessions map[discord.GuildID]*voice.Session

	httpClient *http.Client
	commands   map[string]CommandHandler
}

func NewBot(token string) (*Bot, error) {
	s := state.New("Bot " + token)
	s.AddIntents(gateway.IntentGuilds | gateway.IntentGuildVoiceStates | gateway.IntentGuildMessages)

	b := &Bot{
		state:          s,
		cooldownPeriod: 3 * time.Second,
		voiceSessions:  make(map[discord.GuildID]*voice.Session),
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}

	b.commands = map[string]CommandHandler{
		"join":       b.handleJoin,
		"disconnect": b.handleDisconnect,
		"type":       b.handleType,
		"lol":        b.handleLolEsports,
	}

	return b, nil
}

func (b *Bot) Start(ctx context.Context) error {
	b.state.AddHandler(b.interactionHandler)

	app, err := b.state.CurrentApplication()
	if err != nil {
		return fmt.Errorf("failed to get current application info: %w", err)
	}
	b.appID = app.ID

	if err := b.state.Open(ctx); err != nil {
		return fmt.Errorf("error opening state connection: %w", err)
	}

	definitions := []api.CreateCommandData{
		{
			Name:        "join",
			Description: "Join your current voice channel",
		},
		{
			Name:        "disconnect",
			Description: "Leave the voice channel",
		},
		{
			Name:        "type",
			Description: "Echo text back to the channel",
			Options: discord.CommandOptions{
				&discord.StringOption{
					OptionName:  "text",
					Description: "Text to send",
					Required:    true,
				},
			},
		},
		{
			Name:        "lol",
			Description: "Get live & upcoming LoL Esports tournaments and matches",
		},
	}

	log.Println("Registering slash commands...")
	if _, err := b.state.BulkOverwriteCommands(b.appID, definitions); err != nil {
		return fmt.Errorf("failed to register commands: %w", err)
	}

	log.Printf("Bot initialized as %s (App ID: %s)", app.Name, b.appID)
	return nil
}

func (b *Bot) Stop() {
	if b.state == nil {
		return
	}

	b.voiceMutex.Lock()
	for _, vs := range b.voiceSessions {
		_ = vs.Leave(context.Background())
	}
	b.voiceMutex.Unlock()

	if err := b.state.Close(); err != nil {
		log.Printf("Error closing bot session: %v", err)
	} else {
		log.Println("Bot session stopped successfully.")
	}
}

func (b *Bot) interactionHandler(ev *gateway.InteractionCreateEvent) {
	data, ok := ev.Data.(*discord.CommandInteraction)
	if !ok {
		return
	}

	userID := ev.SenderID()
	now := time.Now()
	if lastTime, loaded := b.userCooldowns.Load(userID); loaded {
		if now.Sub(lastTime.(time.Time)) < b.cooldownPeriod {
			_ = b.state.RespondInteraction(ev.ID, ev.Token, api.InteractionResponse{
				Type: api.MessageInteractionWithSource,
				Data: &api.InteractionResponseData{
					Content: option.NewNullableString("You are sending commands too fast. Please wait."),
					Flags:   discord.EphemeralMessage,
				},
			})
			return
		}
	}
	b.userCooldowns.Store(userID, now)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := b.state.RespondInteraction(ev.ID, ev.Token, api.InteractionResponse{
		Type: api.DeferredMessageInteractionWithSource,
	})
	if err != nil {
		log.Printf("Failed to defer interaction: %v", err)
		return
	}

	handler, exists := b.commands[data.Name]
	if !exists {
		b.editResponse(ctx, ev, "Unknown command.")
		return
	}

	if err := handler(ctx, ev, data); err != nil {
		log.Printf("Error handling command %q: %v", data.Name, err)
		b.editResponse(ctx, ev, "An error occurred while executing the command.")
	}
}

func (b *Bot) getOrCreateVoiceSession(guildID discord.GuildID) (*voice.Session, error) {
	b.voiceMutex.Lock()
	defer b.voiceMutex.Unlock()

	vs, exists := b.voiceSessions[guildID]
	if !exists {
		var err error
		vs, err = voice.NewSession(b.state)
		if err != nil {
			return nil, err
		}
		b.voiceSessions[guildID] = vs
	}
	return vs, nil
}

func (b *Bot) handleJoin(ctx context.Context, ev *gateway.InteractionCreateEvent, _ *discord.CommandInteraction) error {
	vsState, err := b.state.VoiceState(ev.GuildID, ev.SenderID())
	if err != nil || !vsState.ChannelID.IsValid() {
		b.editResponse(ctx, ev, "You must be in a voice channel to use this command.")
		return nil
	}

	vs, err := b.getOrCreateVoiceSession(ev.GuildID)
	if err != nil {
		b.editResponse(ctx, ev, "Failed to initialize voice session.")
		return err
	}

	err = vs.JoinChannel(ctx, vsState.ChannelID, false, true)
	if err != nil {
		b.editResponse(ctx, ev, fmt.Sprintf("Failed to join voice channel: %v", err))
		return err
	}

	b.editResponse(ctx, ev, "Successfully joined your voice channel!")
	return nil
}

func (b *Bot) handleDisconnect(ctx context.Context, ev *gateway.InteractionCreateEvent, _ *discord.CommandInteraction) error {
	b.voiceMutex.Lock()
	vs, exists := b.voiceSessions[ev.GuildID]
	b.voiceMutex.Unlock()

	if !exists {
		b.editResponse(ctx, ev, "I am not currently in a voice channel.")
		return nil
	}

	if err := vs.Leave(ctx); err != nil {
		b.editResponse(ctx, ev, "Failed to disconnect.")
		return err
	}

	b.voiceMutex.Lock()
	delete(b.voiceSessions, ev.GuildID)
	b.voiceMutex.Unlock()

	b.editResponse(ctx, ev, "Disconnected from the voice channel.")
	return nil
}

func (b *Bot) handleType(ctx context.Context, ev *gateway.InteractionCreateEvent, data *discord.CommandInteraction) error {
	opt := data.Options.Find("text")
	if opt.Name == "" {
		b.editResponse(ctx, ev, "Missing required option: text")
		return nil
	}

	strVal := opt.String()

	b.editResponse(ctx, ev, strVal)
	return nil
}

// LoL Esports API structs
type lolEsportsSchedule struct {
	Data struct {
		Schedule struct {
			Events []struct {
				StartTime string `json:"startTime"`
				State     string `json:"state"` // "inProgress", "unstarted", "completed"
				Type      string `json:"type"`  // "match"
				League    struct {
					Name string `json:"name"`
				} `json:"league"`
				Match struct {
					Teams []struct {
						Name string `json:"name"`
						Code string `json:"code"`
					} `json:"teams"`
				} `json:"match"`
			} `json:"events"`
		} `json:"schedule"`
	} `json:"data"`
}

func parseRiotTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func (b *Bot) handleLolEsports(ctx context.Context, ev *gateway.InteractionCreateEvent, _ *discord.CommandInteraction) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://esports-api.lolesports.com/persisted/gw/getSchedule?hl=en-US", nil)
	if err != nil {
		b.editResponse(ctx, ev, "Failed to create request for esports data.")
		return err
	}

	// Set required headers for Riot API Gateway
	req.Header.Set("x-api-key", "0da1510442f2ed721ec4a1104b426d91")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.editResponse(ctx, ev, "Failed to fetch LoL esports data.")
		return err
	}
	defer resp.Body.Close()

	var schedule lolEsportsSchedule
	if err := json.NewDecoder(resp.Body).Decode(&schedule); err != nil {
		b.editResponse(ctx, ev, "Failed to parse esports schedule.")
		return err
	}

	now := time.Now()
	pastCutoff := now.Add(-3 * time.Hour) // Include matches starting today
	futureCutoff := now.Add(7 * 24 * time.Hour)

	var liveMatches []string
	var upcomingMatches []string

	for _, event := range schedule.Data.Schedule.Events {
		if event.Type != "match" {
			continue
		}

		t, err := parseRiotTime(event.StartTime)
		if err != nil {
			continue
		}

		// Extract team identifiers
		team1, team2 := "TBD", "TBD"
		if len(event.Match.Teams) >= 2 {
			if event.Match.Teams[0].Code != "" {
				team1 = event.Match.Teams[0].Code
			} else if event.Match.Teams[0].Name != "" {
				team1 = event.Match.Teams[0].Name
			}

			if event.Match.Teams[1].Code != "" {
				team2 = event.Match.Teams[1].Code
			} else if event.Match.Teams[1].Name != "" {
				team2 = event.Match.Teams[1].Name
			}
		}

		matchTitle := fmt.Sprintf("**%s**: %s vs %s", event.League.Name, team1, team2)

		if event.State == "inProgress" {
			liveMatches = append(liveMatches, fmt.Sprintf("🔴 %s — [Watch Live](https://lolesports.com/live)", matchTitle))
		} else if event.State == "unstarted" && t.After(pastCutoff) && t.Before(futureCutoff) {
			if len(upcomingMatches) < 10 {
				unixTime := t.Unix()
				upcomingMatches = append(upcomingMatches, fmt.Sprintf("📅 %s (<t:%d:R>)", matchTitle, unixTime))
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("🏆 **League of Legends Esports Schedule**\n\n")

	if len(liveMatches) > 0 {
		sb.WriteString("🔴 **LIVE NOW**\n")
		for _, m := range liveMatches {
			sb.WriteString(m + "\n")
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("🔴 **LIVE NOW**: No matches currently live.\n\n")
	}

	if len(upcomingMatches) > 0 {
		sb.WriteString("📅 **UPCOMING (Next 7 Days)**\n")
		for _, m := range upcomingMatches {
			sb.WriteString(m + "\n")
		}
		sb.WriteString("\n📺 Watch all matches on [lolesports.com](https://lolesports.com)")
	} else {
		sb.WriteString("📅 **UPCOMING**: No upcoming matches scheduled for next week.")
	}

	b.editResponse(ctx, ev, sb.String())
	return nil
}

func (b *Bot) editResponse(ctx context.Context, ev *gateway.InteractionCreateEvent, content string) {
	appID := ev.AppID
	if appID == 0 {
		appID = b.appID
	}

	_, err := b.state.EditInteractionResponse(appID, ev.Token, api.EditInteractionResponseData{
		Content: option.NewNullableString(content),
	})
	if err != nil {
		log.Printf("Failed to edit interaction response: %v", err)
	}
}
