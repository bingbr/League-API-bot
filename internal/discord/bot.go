package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

type Command struct {
	Data    *discordgo.ApplicationCommand
	Handler CommandHandler
}

type CommandHandler func(s *discordgo.Session, i *discordgo.InteractionCreate)

type Registry struct {
	commands []*discordgo.ApplicationCommand
	handlers map[string]CommandHandler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]CommandHandler)}
}

func (r *Registry) Add(cmd *Command) {
	if r == nil || cmd == nil || cmd.Data == nil || cmd.Handler == nil {
		return
	}
	if r.handlers == nil {
		r.handlers = make(map[string]CommandHandler)
	}
	r.commands = append(r.commands, cmd.Data)
	r.handlers[cmd.Data.Name] = cmd.Handler
}

func (r *Registry) Commands() []*discordgo.ApplicationCommand {
	if r == nil {
		return nil
	}
	out := make([]*discordgo.ApplicationCommand, len(r.commands))
	copy(out, r.commands)
	return out
}

func (r *Registry) Handler(name string) (CommandHandler, bool) {
	if r == nil {
		return nil, false
	}
	h, ok := r.handlers[name]
	return h, ok
}

type Bot struct {
	session  *discordgo.Session
	registry *Registry
	guildID  string
	isDev    bool
	logger   *slog.Logger
	// bulkOverwriteFn is used by tests to stub Discord command registration.
	bulkOverwriteFn func(appID string, guildID string, commands []*discordgo.ApplicationCommand, options ...discordgo.RequestOption) (createdCommands []*discordgo.ApplicationCommand, err error)
}

type Option func(*Bot)

func WithLogger(logger *slog.Logger) Option {
	return func(b *Bot) {
		if logger != nil {
			b.logger = logger
		}
	}
}

func WithRegistry(registry *Registry) Option {
	return func(b *Bot) {
		if registry != nil {
			b.registry = registry
		}
	}
}

func NewBot(token, guildID string, isDev bool, opts ...Option) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	bot := &Bot{
		session:         session,
		registry:        NewRegistry(),
		guildID:         guildID,
		isDev:           isDev,
		logger:          slog.Default(),
		bulkOverwriteFn: session.ApplicationCommandBulkOverwrite,
	}
	for _, opt := range opts {
		opt(bot)
	}
	return bot, nil
}

func (b *Bot) Session() *discordgo.Session {
	if b == nil {
		return nil
	}
	return b.session
}

func (b *Bot) Run() error {
	b.session.AddHandler(func(s *discordgo.Session, _ *discordgo.Ready) {
		b.logger.Info("Logged in", "user", s.State.User.Username, "#", s.State.User.Discriminator)
	})

	if err := b.session.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}
	defer func() {
		if err := b.session.Close(); err != nil {
			b.logger.Error("Failed to close Discord session", "error", err)
		}
	}()

	if b.isDev || b.logger.Enabled(context.Background(), slog.LevelDebug) {
		b.logger.Debug("=== DEBUG: Listing all registered commands ===")
		b.logCommands("", "global")
		if b.guildID != "" {
			b.logCommands(b.guildID, "guild")
		}
		b.logger.Debug("=== DEBUG: Listing complete ===")
	}

	if err := b.registerCommands(); err != nil {
		return fmt.Errorf("failed to register commands: %w", err)
	}

	if err := b.session.UpdateGameStatus(0, "League of Legends"); err != nil {
		b.logger.Error("Failed to update listening status", "error", err)
	}

	b.session.AddHandler(b.handleInteraction)
	return b.waitForInterrupt()
}

func (b *Bot) waitForInterrupt() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	b.logger.Info("---> Press Ctrl+C to exit <---")
	<-ctx.Done()
	b.logger.Info("Shutting down...")
	return nil
}

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i == nil || i.Interaction == nil {
		return
	}
	if i.Type != discordgo.InteractionApplicationCommand && i.Type != discordgo.InteractionApplicationCommandAutocomplete {
		return
	}
	b.logInteraction(i)
	cmdName := i.ApplicationCommandData().Name
	if h, ok := b.registry.Handler(cmdName); ok {
		h(s, i)
		return
	}
	if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionApplicationCommandAutocompleteResult,
			Data: &discordgo.InteractionResponseData{Choices: []*discordgo.ApplicationCommandOptionChoice{}},
		})
		return
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "This command is not yet supported.",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func InteractionUserID(i *discordgo.InteractionCreate) (username, userID string) {
	if i == nil {
		return "", ""
	}
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.Username, i.Member.User.ID
	}
	if i.User != nil {
		return i.User.Username, i.User.ID
	}
	return "", ""
}

func (b *Bot) logInteraction(i *discordgo.InteractionCreate) {
	username, userID := InteractionUserID(i)
	interactionType := "command"
	if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
		interactionType = "autocomplete"
	}
	b.logger.Info("Interaction", "command", i.ApplicationCommandData().Name, "type", interactionType, "username", username, "userID", userID, "guildID", i.GuildID)
}

func (b *Bot) registerCommands() error {
	if b.isDev {
		if b.guildID == "" {
			return fmt.Errorf("guild ID is required to register commands in dev mode")
		}
		if err := b.clearGlobalCommands(); err != nil {
			return err
		}
		return b.overwriteCommands(b.guildID, "guild")
	}
	if err := b.clearKnownGuildCommands(); err != nil {
		return err
	}
	return b.overwriteCommands("", "global")
}

func (b *Bot) overwriteCommands(guildID, label string) error {
	created, err := b.bulkOverwrite(guildID, b.registry.Commands())
	if err != nil {
		return fmt.Errorf("cannot overwrite %s commands: %w", label, err)
	}
	b.logger.Info("Registered commands", "scope", label, "count", len(created))
	return nil
}

func (b *Bot) clearGuildCommands(guildID string) error {
	if guildID == "" {
		return nil
	}
	b.logger.Debug("=== Cleaning guild commands ===", "guildID", guildID)
	if _, err := b.bulkOverwrite(guildID, []*discordgo.ApplicationCommand{}); err != nil {
		return fmt.Errorf("cannot clear guild commands: %w", err)
	}
	b.logger.Debug("=== Cleared guild commands ===", "guildID", guildID)
	return nil
}

func (b *Bot) clearKnownGuildCommands() error {
	guildIDs := collectGuildIDs(b.guildID, b.session)
	var errs []error
	for _, guildID := range guildIDs {
		if err := b.clearGuildCommands(guildID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func collectGuildIDs(configuredGuildID string, session *discordgo.Session) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0, 1)
	add := func(id string) {
		if id != "" {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	add(configuredGuildID)
	if session != nil && session.State != nil {
		session.State.RLock()
		for _, guild := range session.State.Guilds {
			if guild != nil {
				add(guild.ID)
			}
		}
		session.State.RUnlock()
	}
	slices.Sort(ids)
	return ids
}

func (b *Bot) clearGlobalCommands() error {
	if _, err := b.bulkOverwrite("", []*discordgo.ApplicationCommand{}); err != nil {
		return fmt.Errorf("cannot clear global commands: %w", err)
	}
	b.logger.Debug("Cleared global commands")
	return nil
}

func (b *Bot) bulkOverwrite(guildID string, commands []*discordgo.ApplicationCommand) ([]*discordgo.ApplicationCommand, error) {
	if b == nil || b.session == nil || b.session.State == nil || b.session.State.User == nil {
		return nil, fmt.Errorf("discord session user is unavailable")
	}
	fn := b.bulkOverwriteFn
	if fn == nil {
		fn = b.session.ApplicationCommandBulkOverwrite
	}
	return fn(b.session.State.User.ID, guildID, commands)
}

func (b *Bot) logCommands(guildID, label string) {
	cmds, err := b.session.ApplicationCommands(b.session.State.User.ID, guildID)
	if err != nil {
		b.logger.Warn("Failed to fetch commands", "scope", label, "error", err)
		return
	}
	logArgs := []any{"scope", label, "count", len(cmds)}
	if guildID != "" {
		logArgs = append(logArgs, "guildID", guildID)
	}
	b.logger.Debug("Commands", logArgs...)
	for _, cmd := range cmds {
		integrationTypes := "nil"
		if cmd.IntegrationTypes != nil {
			integrationTypes = fmt.Sprintf("%v", *cmd.IntegrationTypes)
		}
		b.logger.Debug("  Command", "scope", label, "name", cmd.Name, "id", cmd.ID, "integrationTypes", integrationTypes)
	}
}
