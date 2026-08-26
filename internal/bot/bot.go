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

// CommandHandler defines the signature for slash command handlers.
type CommandHandler func(ctx context.Context, ev *gateway.InteractionCreateEvent, data *discord.CommandInteraction) error

// Bot wraps state, voice sessions, and command handling logic.
type Bot struct {
	state          *state.State
	voiceSession   *voice.Session
	cooldownPeriod time.Duration
	userCooldowns  sync.Map // map[discord.UserID]time.Time
	voiceMutex     sync.Mutex
	commands       map[string]CommandHandler
}

// NewBot initializes a new Bot instance.
func NewBot(token string) (*Bot, error) {
	s := state.New("Bot " + token)
	s.AddIntents(gateway.IntentGuilds | gateway.IntentGuildVoiceStates | gateway.IntentGuildMessages)

	v, err := voice.NewSession(s)
	if err != nil {
		return nil, fmt.Errorf("failed to create voice session: %w", err)
	}

	b := &Bot{
		state:          s,
		voiceSession:   v,
		cooldownPeriod: 3 * time.Second,
	}

	// Register command mapping
	b.commands = map[string]CommandHandler{
		"join":       b.handleJoin,
		"disconnect": b.handleDisconnect,
		"type":       b.handleType,
	}

	return b, nil
}

// Start opens gateway connection and registers application slash commands.
func (b *Bot) Start(ctx context.Context) error {
	b.state.AddHandler(b.interactionHandler)

	if err := b.state.Open(ctx); err != nil {
		return fmt.Errorf("error opening state connection: %w", err)
	}

	app, err := b.state.CurrentApplication()
	if err != nil {
		return fmt.Errorf("failed to get current application info: %w", err)
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
	if _, err := b.state.BulkOverwriteCommands(app.ID, definitions); err != nil {
		return fmt.Errorf("failed to register commands: %w", err)
	}

	log.Printf("Bot initialized as %s (App ID: %s)", app.Name, app.ID)
	return nil
}

// Stop gracefully leaves voice channels and closes gateway session.
func (b *Bot) Stop() {
	if b.state == nil {
		return
	}

	b.voiceMutex.Lock()
	if b.voiceSession != nil {
		_ = b.voiceSession.Leave(context.Background())
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

	// Per-User Cooldown Check
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

	ctx := context.Background()

	// Defer response to allow execution time
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

func (b *Bot) handleJoin(ctx context.Context, ev *gateway.InteractionCreateEvent, _ *discord.CommandInteraction) error {
	vs, err := b.state.VoiceState(ev.GuildID, ev.SenderID())
	if err != nil || !vs.ChannelID.IsValid() {
		b.editResponse(ctx, ev, "You must be in a voice channel to use this command.")
		return nil
	}

	b.voiceMutex.Lock()
	defer b.voiceMutex.Unlock()

	err = b.voiceSession.JoinChannel(ctx, vs.ChannelID, false, true)
	if err != nil {
		b.editResponse(ctx, ev, fmt.Sprintf("Failed to join voice channel: %v", err))
		return err
	}

	b.editResponse(ctx, ev, "Successfully joined your voice channel!")
	return nil
}

func (b *Bot) handleDisconnect(ctx context.Context, ev *gateway.InteractionCreateEvent, _ *discord.CommandInteraction) error {
	b.voiceMutex.Lock()
	defer b.voiceMutex.Unlock()

	if err := b.voiceSession.Leave(ctx); err != nil {
		b.editResponse(ctx, ev, "Failed to disconnect or not currently in a voice channel.")
		return err
	}

	b.editResponse(ctx, ev, "Disconnected from the voice channel.")
	return nil
}

func (b *Bot) handleType(ctx context.Context, ev *gateway.InteractionCreateEvent, data *discord.CommandInteraction) error {
	opt := data.Options.Find("text")
	if opt == nil {
		b.editResponse(ctx, ev, "Missing required option: text")
		return nil
	}

	b.editResponse(ctx, ev, opt.Value.String())
	return nil
}

func (b *Bot) editResponse(ctx context.Context, ev *gateway.InteractionCreateEvent, content string) {
	_, err := b.state.EditInteractionResponse(ev.AppID, ev.Token, api.EditInteractionResponseData{
		Content: option.NewNullableString(content),
	})
	if err != nil {
		log.Printf("Failed to edit interaction response: %v", err)
	}
}
