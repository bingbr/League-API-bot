package discord

import (
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestCollectGuildIDs_Empty(t *testing.T) {
	got := collectGuildIDs("", nil)
	if len(got) != 0 {
		t.Fatalf("expected no guild IDs, got %#v", got)
	}
}

func TestCollectGuildIDs_ConfiguredOnly(t *testing.T) {
	got := collectGuildIDs("123", nil)
	want := []string{"123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestCollectGuildIDs_ConfiguredAndStateDeduplicated(t *testing.T) {
	session := &discordgo.Session{
		State: &discordgo.State{
			Ready: discordgo.Ready{
				Guilds: []*discordgo.Guild{
					{ID: "b"},
					nil,
					{ID: "a"},
					{ID: "a"},
				},
			},
		},
	}
	got := collectGuildIDs("b", session)
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestCollectGuildIDs_StateOnlySorted(t *testing.T) {
	session := &discordgo.Session{
		State: &discordgo.State{
			Ready: discordgo.Ready{
				Guilds: []*discordgo.Guild{
					{ID: "9"},
					{ID: "1"},
					{ID: "4"},
				},
			},
		},
	}
	got := collectGuildIDs("", session)
	want := []string{"1", "4", "9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

type overwriteCall struct {
	guildID string
	names   []string
}

func TestRegisterCommands_DevClearsGlobalThenOverwritesGuild(t *testing.T) {
	bot := newTestBot(true, "guild-dev", []string{"guild-a"})
	getCalls := captureBulkOverwriteCalls(bot)

	if err := bot.registerCommands(); err != nil {
		t.Fatalf("registerCommands returned error: %v", err)
	}
	calls := getCalls()

	if len(calls) != 2 {
		t.Fatalf("expected 2 overwrite calls, got %d", len(calls))
	}
	if calls[0].guildID != "" {
		t.Fatalf("expected first call to clear global scope, got guildID=%q", calls[0].guildID)
	}
	if len(calls[0].names) != 0 {
		t.Fatalf("expected first call to send no commands (clear), got %#v", calls[0].names)
	}
	if calls[1].guildID != "guild-dev" {
		t.Fatalf("expected second call on dev guild, got guildID=%q", calls[1].guildID)
	}
	if !reflect.DeepEqual(calls[1].names, []string{"ping"}) {
		t.Fatalf("expected second call to overwrite guild commands with registry command, got %#v", calls[1].names)
	}
}

func TestRegisterCommands_NonDevClearsGuildsThenOverwritesGlobal(t *testing.T) {
	t.Run("debug", func(t *testing.T) {
		assertNonDevCommandRegistration(t)
	})
	t.Run("prod", func(t *testing.T) {
		assertNonDevCommandRegistration(t)
	})
}

func assertNonDevCommandRegistration(t *testing.T) {
	t.Helper()

	bot := newTestBot(false, "guild-b", []string{"guild-c", "guild-a", "guild-b"})
	getCalls := captureBulkOverwriteCalls(bot)

	if err := bot.registerCommands(); err != nil {
		t.Fatalf("registerCommands returned error: %v", err)
	}
	calls := getCalls()

	if len(calls) != 4 {
		t.Fatalf("expected 4 overwrite calls (3 guild clears + 1 global overwrite), got %d", len(calls))
	}
	if calls[0].guildID != "guild-a" || len(calls[0].names) != 0 {
		t.Fatalf("expected first call to clear guild-a, got guildID=%q names=%#v", calls[0].guildID, calls[0].names)
	}
	if calls[1].guildID != "guild-b" || len(calls[1].names) != 0 {
		t.Fatalf("expected second call to clear guild-b, got guildID=%q names=%#v", calls[1].guildID, calls[1].names)
	}
	if calls[2].guildID != "guild-c" || len(calls[2].names) != 0 {
		t.Fatalf("expected third call to clear guild-c, got guildID=%q names=%#v", calls[2].guildID, calls[2].names)
	}
	if calls[3].guildID != "" {
		t.Fatalf("expected final call to target global scope, got guildID=%q", calls[3].guildID)
	}
	if !reflect.DeepEqual(calls[3].names, []string{"ping"}) {
		t.Fatalf("expected final call to overwrite global commands with registry command, got %#v", calls[3].names)
	}
}

func newTestBot(isDev bool, configuredGuildID string, stateGuildIDs []string) *Bot {
	guilds := make([]*discordgo.Guild, 0, len(stateGuildIDs))
	for _, guildID := range stateGuildIDs {
		guilds = append(guilds, &discordgo.Guild{ID: guildID})
	}

	registry := NewRegistry()
	registry.Add(&Command{
		Data: &discordgo.ApplicationCommand{Name: "ping"},
		Handler: func(*discordgo.Session, *discordgo.InteractionCreate) {
		},
	})

	return &Bot{
		session: &discordgo.Session{
			State: &discordgo.State{
				Ready: discordgo.Ready{
					User:   &discordgo.User{ID: "app-id"},
					Guilds: guilds,
				},
			},
		},
		registry: registry,
		guildID:  configuredGuildID,
		isDev:    isDev,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func captureBulkOverwriteCalls(bot *Bot) func() []overwriteCall {
	calls := make([]overwriteCall, 0, 4)
	bot.bulkOverwriteFn = func(_ string, guildID string, commands []*discordgo.ApplicationCommand, _ ...discordgo.RequestOption) ([]*discordgo.ApplicationCommand, error) {
		names := make([]string, 0, len(commands))
		for _, cmd := range commands {
			if cmd == nil {
				continue
			}
			names = append(names, cmd.Name)
		}
		calls = append(calls, overwriteCall{
			guildID: guildID,
			names:   names,
		})
		return commands, nil
	}
	return func() []overwriteCall {
		out := make([]overwriteCall, len(calls))
		copy(out, calls)
		return out
	}
}
