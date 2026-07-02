package bot

import (
	"context"
	"fmt"
	"log"
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

type Bot struct {
	state          *state.State
	voiceSession   *voice.Session
	lastCmdTime    time.Time
	cooldownMutex  sync.Mutex
	cooldownPeriod time.Duration
	voiceMutex     sync.Mutex
}

func NewBot(token string) (*Bot, error) {
	s := state.New("Bot " + token)
	s.AddIntents(gateway.IntentGuilds | gateway.IntentGuildVoiceStates | gateway.IntentGuildMessages)

	v, err := voice.NewSession(s)
	if err != nil {
		return nil, fmt.Errorf("failed to create voice session: %w", err)
	}

	return &Bot{
		state:          s,
		voiceSession:   v,
		cooldownPeriod: 5 * time.Second,
	}, nil
}

func (b *Bot) Start() error {
	b.state.AddHandler(b.interactionHandler)

	if err := b.state.Open(context.Background()); err != nil {
		return fmt.Errorf("error opening connection: %w", err)
	}

	app, err := b.state.CurrentApplication()
	if err != nil {
		return fmt.Errorf("failed to get application info: %w", err)
	}

	commands := []api.CreateCommandData{
		{Name: "join", Description: "Join your voice channel"},
		{Name: "disconnect", Description: "Leave voice channel"},
		{
			Name:        "type",
			Description: "Echo text",
			Options: []discord.CommandOption{
				&discord.StringOption{
					OptionName:  "text",
					Description: "Text to send",
					Required:    true,
				},
			},
		},
	}

	log.Println("Registering commands...")
	_, err = b.state.BulkOverwriteCommands(app.ID, commands)
	if err != nil {
		return fmt.Errorf("command register error: %w", err)
	}

	log.Println("Bot running.")
	return nil
}

func (b *Bot) Stop() {
	if b.state != nil {
		b.voiceMutex.Lock()
		if b.voiceSession != nil {
			_ = b.voiceSession.Leave(context.Background())
		}
		b.voiceMutex.Unlock()

		_ = b.state.Close()
		log.Println("Bot stopped.")
	}
}

func (b *Bot) interactionHandler(ev *gateway.InteractionCreateEvent) {
	data, ok := ev.Data.(*discord.CommandInteraction)
	if !ok {
		return
	}

	ctx := context.Background()

	// Cooldown Check
	b.cooldownMutex.Lock()
	now := time.Now()
	if now.Sub(b.lastCmdTime) < b.cooldownPeriod {
	b.cooldownMutex.Unlock()
	
		// SILENT DROP: We do not call RespondInteraction. 
		// This saves 100% of our upload (egress) bandwidth.
		return 
	}
	b.lastCmdTime = now
	b.cooldownMutex.Unlock()

	// Defer Response (displays "Thinking ...")
	err := b.state.RespondInteraction(ev.ID, ev.Token, api.InteractionResponse{
		Type: api.DeferredMessageInteractionWithSource,
	})
	if err != nil {
		log.Printf("failed to defer response: %v", err)
		return
	}

	switch data.Name {
	case "join":
		b.handleJoin(ctx, ev)
	case "disconnect":
		b.handleDisconnect(ctx, ev)
	case "type":
		var text string
		if len(data.Options) > 0 {
			text = data.Options[0].String()
		}
		b.handleType(ctx, ev, text)
	}
}

func (b *Bot) handleJoin(ctx context.Context, ev *gateway.InteractionCreateEvent) {
	vs, err := b.state.VoiceState(ev.GuildID, ev.SenderID())
	if err != nil || !vs.ChannelID.IsValid() {
		b.editResponse(ev, "❌ You are not in a voice channel")
		return
	}

	b.voiceMutex.Lock()
	defer b.voiceMutex.Unlock()

	// Arikawa infers GuildID internally via channel metadata cache
	err = b.voiceSession.JoinChannel(ctx, vs.ChannelID, false, true)
	if err != nil {
		// Bypass Arikawa's false-positive typed-nil error output
		if strings.Contains(err.Error(), "<nil>") {
			log.Printf("Voice join handshake completed successfully (filtered framework error: %v)", err)
		} else {
			log.Printf("genuine voice join error: %v", err)
			b.editResponse(ev, "❌ Voice join failed: "+err.Error())
			return
		}
	}

	b.editResponse(ev, "🔊 Successfully joined voice channel!")
}

func (b *Bot) handleDisconnect(ctx context.Context, ev *gateway.InteractionCreateEvent) {
	b.voiceMutex.Lock()
	defer b.voiceMutex.Unlock()

	// Tell Arikawa to cleanly step away from the voice channel
	err := b.voiceSession.Leave(ctx)
	if err != nil {
		log.Printf("voice disconnect error: %v", err)
		b.editResponse(ev, "❌ Disconnect failed or bot not in voice channel")
		return
	}

	b.editResponse(ev, "👋 Disconnected")
}

func (b *Bot) handleType(ctx context.Context, ev *gateway.InteractionCreateEvent, text string) {
	b.editResponse(ev, text)
}

func (b *Bot) editResponse(ev *gateway.InteractionCreateEvent, msg string) {
	_, err := b.state.EditInteractionResponse(ev.AppID, ev.Token, api.EditInteractionResponseData{
		Content: option.NewNullableString(msg),
	})
	if err != nil {
		log.Printf("edit error: %v", err)
	}
}
