package bot

import (
	"context"
	"fmt"
	"log"
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

	commands map[string]CommandHandler
}

func NewBot(token string) (*Bot, error) {
	s := state.New("Bot " + token)
	s.AddIntents(gateway.IntentGuilds | gateway.IntentGuildVoiceStates | gateway.IntentGuildMessages)

	b := &Bot{
		state:          s,
		cooldownPeriod: 3 * time.Second,
		voiceSessions:  make(map[discord.GuildID]*voice.Session),
	}

	b.commands = map[string]CommandHandler{
		"join":       b.handleJoin,
		"disconnect": b.handleDisconnect,
		"type":       b.handleType,
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
