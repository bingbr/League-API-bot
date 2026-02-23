package discord

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bwmarrin/discordgo"
)

type interactionRespondCall struct {
	resp *discordgo.InteractionResponse
}

type interactionEditCall struct {
	edit *discordgo.WebhookEdit
}

type interactionDeleteCall struct{}

type interactionFollowupCall struct {
	wait bool
	data *discordgo.WebhookParams
}

type interactionCallRecorder struct {
	respondCalls  []interactionRespondCall
	editCalls     []interactionEditCall
	deleteCalls   []interactionDeleteCall
	followupCalls []interactionFollowupCall
}

func TestRunDeferredEmbedCommand_FastSuccessRespondsImmediately(t *testing.T) {
	recorder := withDeferredCommandTestStubs(t)
	setDeferredCommandTiming(t, 150*time.Millisecond, 50*time.Millisecond)

	err := RunDeferredEmbedCommand(testSession(), testInteraction(), 300*time.Millisecond, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
		return []*discordgo.MessageEmbed{{Title: "fast"}}, nil
	}, nil)
	if err != nil {
		t.Fatalf("RunDeferredEmbedCommand() error = %v", err)
	}

	if len(recorder.respondCalls) != 1 {
		t.Fatalf("len(respondCalls) = %d, want 1", len(recorder.respondCalls))
	}
	if got := recorder.respondCalls[0].resp.Type; got != discordgo.InteractionResponseChannelMessageWithSource {
		t.Fatalf("response type = %v, want %v", got, discordgo.InteractionResponseChannelMessageWithSource)
	}
	if len(recorder.editCalls) != 0 {
		t.Fatalf("len(editCalls) = %d, want 0", len(recorder.editCalls))
	}
	if len(recorder.followupCalls) != 0 {
		t.Fatalf("len(followupCalls) = %d, want 0", len(recorder.followupCalls))
	}
	if len(recorder.deleteCalls) != 0 {
		t.Fatalf("len(deleteCalls) = %d, want 0", len(recorder.deleteCalls))
	}
}

func TestRunDeferredEmbedCommand_SlowSuccessDefersAndEdits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		recorder := withDeferredCommandTestStubs(t)
		setDeferredCommandTiming(t, 90*time.Millisecond, 60*time.Millisecond)

		err := RunDeferredEmbedCommand(testSession(), testInteraction(), 300*time.Millisecond, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(70 * time.Millisecond):
				return []*discordgo.MessageEmbed{{Title: "slow"}}, nil
			}
		}, nil)
		if err != nil {
			t.Fatalf("RunDeferredEmbedCommand() error = %v", err)
		}

		if len(recorder.respondCalls) != 1 {
			t.Fatalf("len(respondCalls) = %d, want 1", len(recorder.respondCalls))
		}
		if got := recorder.respondCalls[0].resp.Type; got != discordgo.InteractionResponseDeferredChannelMessageWithSource {
			t.Fatalf("response type = %v, want %v", got, discordgo.InteractionResponseDeferredChannelMessageWithSource)
		}
		if len(recorder.editCalls) != 1 {
			t.Fatalf("len(editCalls) = %d, want 1", len(recorder.editCalls))
		}
		if recorder.editCalls[0].edit.Embeds == nil || len(*recorder.editCalls[0].edit.Embeds) != 1 {
			t.Fatalf("unexpected edited embeds: %#v", recorder.editCalls[0].edit.Embeds)
		}
		if len(recorder.followupCalls) != 0 {
			t.Fatalf("len(followupCalls) = %d, want 0", len(recorder.followupCalls))
		}
		if len(recorder.deleteCalls) != 0 {
			t.Fatalf("len(deleteCalls) = %d, want 0", len(recorder.deleteCalls))
		}
	})
}

func TestRunDeferredEmbedCommand_FastErrorRespondsImmediately(t *testing.T) {
	recorder := withDeferredCommandTestStubs(t)
	setDeferredCommandTiming(t, 150*time.Millisecond, 50*time.Millisecond)

	err := RunDeferredEmbedCommand(testSession(), testInteraction(), 300*time.Millisecond, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
		return nil, errors.New("boom")
	}, nil)
	if err != nil {
		t.Fatalf("RunDeferredEmbedCommand() error = %v", err)
	}

	if len(recorder.respondCalls) != 1 {
		t.Fatalf("len(respondCalls) = %d, want 1", len(recorder.respondCalls))
	}
	if got := recorder.respondCalls[0].resp.Type; got != discordgo.InteractionResponseChannelMessageWithSource {
		t.Fatalf("response type = %v, want %v", got, discordgo.InteractionResponseChannelMessageWithSource)
	}
	if recorder.respondCalls[0].resp.Data == nil || len(recorder.respondCalls[0].resp.Data.Embeds) != 1 {
		t.Fatalf("unexpected response embeds: %#v", recorder.respondCalls[0].resp.Data)
	}
	if got := recorder.respondCalls[0].resp.Data.Flags; got != discordgo.MessageFlagsEphemeral {
		t.Fatalf("response flags = %v, want %v", got, discordgo.MessageFlagsEphemeral)
	}
	if got := recorder.respondCalls[0].resp.Data.Embeds[0].Description; got != "Could not connect to Riot servers.\nPlease try again later." {
		t.Fatalf("error description = %q", got)
	}
	if len(recorder.editCalls) != 0 {
		t.Fatalf("len(editCalls) = %d, want 0", len(recorder.editCalls))
	}
	if len(recorder.followupCalls) != 0 {
		t.Fatalf("len(followupCalls) = %d, want 0", len(recorder.followupCalls))
	}
	if len(recorder.deleteCalls) != 0 {
		t.Fatalf("len(deleteCalls) = %d, want 0", len(recorder.deleteCalls))
	}
}

func TestRunDeferredEmbedCommand_SlowErrorUsesMapperAfterDefer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		recorder := withDeferredCommandTestStubs(t)
		setDeferredCommandTiming(t, 90*time.Millisecond, 60*time.Millisecond)

		err := RunDeferredEmbedCommand(testSession(), testInteraction(), 300*time.Millisecond, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(70 * time.Millisecond):
				return nil, errors.New("boom")
			}
		}, func(err error) string {
			return "mapped error"
		})
		if err != nil {
			t.Fatalf("RunDeferredEmbedCommand() error = %v", err)
		}

		if len(recorder.respondCalls) != 1 {
			t.Fatalf("len(respondCalls) = %d, want 1", len(recorder.respondCalls))
		}
		if got := recorder.respondCalls[0].resp.Type; got != discordgo.InteractionResponseDeferredChannelMessageWithSource {
			t.Fatalf("response type = %v, want %v", got, discordgo.InteractionResponseDeferredChannelMessageWithSource)
		}
		if len(recorder.editCalls) != 0 {
			t.Fatalf("len(editCalls) = %d, want 0", len(recorder.editCalls))
		}
		if len(recorder.followupCalls) != 1 {
			t.Fatalf("len(followupCalls) = %d, want 1", len(recorder.followupCalls))
		}
		if recorder.followupCalls[0].data == nil || len(recorder.followupCalls[0].data.Embeds) != 1 {
			t.Fatalf("unexpected followup data: %#v", recorder.followupCalls[0].data)
		}
		if got := recorder.followupCalls[0].data.Embeds[0].Description; got != "mapped error" {
			t.Fatalf("mapped error description = %q, want %q", got, "mapped error")
		}
		if got := recorder.followupCalls[0].data.Flags; got != discordgo.MessageFlagsEphemeral {
			t.Fatalf("followup flags = %v, want %v", got, discordgo.MessageFlagsEphemeral)
		}
		if len(recorder.deleteCalls) != 1 {
			t.Fatalf("len(deleteCalls) = %d, want 1", len(recorder.deleteCalls))
		}
	})
}

func TestRunDeferredEmbedCommand_DeferFailureSkipsEdit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var editCalls []interactionEditCall
		setDeferredCommandTiming(t, 90*time.Millisecond, 60*time.Millisecond)
		setDeferredCommandStubs(t,
			func(s *discordgo.Session, i *discordgo.Interaction, resp *discordgo.InteractionResponse) error {
				if resp != nil && resp.Type == discordgo.InteractionResponseDeferredChannelMessageWithSource {
					return errors.New("defer failed")
				}
				return nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction, edit *discordgo.WebhookEdit) (*discordgo.Message, error) {
				editCalls = append(editCalls, interactionEditCall{edit: edit})
				return &discordgo.Message{}, nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction) error {
				return nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction, wait bool, data *discordgo.WebhookParams) (*discordgo.Message, error) {
				return &discordgo.Message{}, nil
			},
		)

		err := RunDeferredEmbedCommand(testSession(), testInteraction(), 300*time.Millisecond, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(70 * time.Millisecond):
				return []*discordgo.MessageEmbed{{Title: "slow"}}, nil
			}
		}, nil)
		if err == nil || err.Error() != "defer failed" {
			t.Fatalf("RunDeferredEmbedCommand() error = %v, want defer failed", err)
		}
		if len(editCalls) != 0 {
			t.Fatalf("len(editCalls) = %d, want 0", len(editCalls))
		}
	})
}

func TestRunDeferredEmbedCommand_DeferAlreadyAcknowledgedStopsWithoutEdit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			respondCalls []interactionRespondCall
			editCalls    []interactionEditCall
		)
		setDeferredCommandTiming(t, 90*time.Millisecond, 60*time.Millisecond)
		setDeferredCommandStubs(t,
			func(s *discordgo.Session, i *discordgo.Interaction, resp *discordgo.InteractionResponse) error {
				respondCalls = append(respondCalls, interactionRespondCall{resp: resp})
				if resp != nil && resp.Type == discordgo.InteractionResponseDeferredChannelMessageWithSource {
					return alreadyAcknowledgedRESTError()
				}
				return nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction, edit *discordgo.WebhookEdit) (*discordgo.Message, error) {
				editCalls = append(editCalls, interactionEditCall{edit: edit})
				return &discordgo.Message{}, nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction) error {
				return nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction, wait bool, data *discordgo.WebhookParams) (*discordgo.Message, error) {
				return &discordgo.Message{}, nil
			},
		)

		err := RunDeferredEmbedCommand(testSession(), testInteraction(), 300*time.Millisecond, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(70 * time.Millisecond):
				return []*discordgo.MessageEmbed{{Title: "slow"}}, nil
			}
		}, nil)
		if err != nil {
			t.Fatalf("RunDeferredEmbedCommand() error = %v, want nil", err)
		}

		if len(respondCalls) != 1 {
			t.Fatalf("len(respondCalls) = %d, want 1", len(respondCalls))
		}
		if got := respondCalls[0].resp.Type; got != discordgo.InteractionResponseDeferredChannelMessageWithSource {
			t.Fatalf("response type = %v, want %v", got, discordgo.InteractionResponseDeferredChannelMessageWithSource)
		}
		if len(editCalls) != 0 {
			t.Fatalf("len(editCalls) = %d, want 0", len(editCalls))
		}
	})
}

func TestRunDeferredEmbedCommand_DeferUnknownInteractionStopsWithoutEdit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			respondCalls []interactionRespondCall
			editCalls    []interactionEditCall
		)
		setDeferredCommandTiming(t, 90*time.Millisecond, 60*time.Millisecond)
		setDeferredCommandStubs(t,
			func(s *discordgo.Session, i *discordgo.Interaction, resp *discordgo.InteractionResponse) error {
				respondCalls = append(respondCalls, interactionRespondCall{resp: resp})
				if resp != nil && resp.Type == discordgo.InteractionResponseDeferredChannelMessageWithSource {
					return unknownInteractionRESTError()
				}
				return nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction, edit *discordgo.WebhookEdit) (*discordgo.Message, error) {
				editCalls = append(editCalls, interactionEditCall{edit: edit})
				return &discordgo.Message{}, nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction) error {
				return nil
			},
			func(s *discordgo.Session, i *discordgo.Interaction, wait bool, data *discordgo.WebhookParams) (*discordgo.Message, error) {
				return &discordgo.Message{}, nil
			},
		)

		err := RunDeferredEmbedCommand(testSession(), testInteraction(), 300*time.Millisecond, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(70 * time.Millisecond):
				return []*discordgo.MessageEmbed{{Title: "slow"}}, nil
			}
		}, nil)
		if err != nil {
			t.Fatalf("RunDeferredEmbedCommand() error = %v, want nil", err)
		}

		if len(respondCalls) != 1 {
			t.Fatalf("len(respondCalls) = %d, want 1", len(respondCalls))
		}
		if got := respondCalls[0].resp.Type; got != discordgo.InteractionResponseDeferredChannelMessageWithSource {
			t.Fatalf("response type = %v, want %v", got, discordgo.InteractionResponseDeferredChannelMessageWithSource)
		}
		if len(editCalls) != 0 {
			t.Fatalf("len(editCalls) = %d, want 0", len(editCalls))
		}
	})
}

func TestRunDeferredEmbedCommand_ValidationErrors(t *testing.T) {
	interaction := testInteraction()
	exec := func(context.Context) ([]*discordgo.MessageEmbed, error) {
		return nil, nil
	}

	if err := RunDeferredEmbedCommand(nil, interaction, 0, exec, nil); err == nil || err.Error() != "discord session is required" {
		t.Fatalf("nil session error = %v", err)
	}
	if err := RunDeferredEmbedCommand(testSession(), nil, 0, exec, nil); err == nil || err.Error() != "interaction is required" {
		t.Fatalf("nil interaction error = %v", err)
	}
	if err := RunDeferredEmbedCommand(testSession(), interaction, 0, nil, nil); err == nil || err.Error() != "deferred executor is required" {
		t.Fatalf("nil executor error = %v", err)
	}
}

func TestRunDeferredEmbedCommand_CompletesBeforeTriggerDoesNotDefer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		recorder := withDeferredCommandTestStubs(t)
		setDeferredCommandTiming(t, 300*time.Millisecond, 100*time.Millisecond)

		done := make(chan struct{})
		time.AfterFunc(180*time.Millisecond, func() {
			close(done)
		})

		err := RunDeferredEmbedCommand(testSession(), testInteraction(), 500*time.Millisecond, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-done:
				return []*discordgo.MessageEmbed{{Title: "boundary"}}, nil
			}
		}, nil)
		if err != nil {
			t.Fatalf("RunDeferredEmbedCommand() error = %v", err)
		}

		if len(recorder.respondCalls) != 1 {
			t.Fatalf("len(respondCalls) = %d, want 1", len(recorder.respondCalls))
		}
		if got := recorder.respondCalls[0].resp.Type; got != discordgo.InteractionResponseChannelMessageWithSource {
			t.Fatalf("response type = %v, want %v", got, discordgo.InteractionResponseChannelMessageWithSource)
		}
		if len(recorder.editCalls) != 0 {
			t.Fatalf("len(editCalls) = %d, want 0", len(recorder.editCalls))
		}
		if len(recorder.followupCalls) != 0 {
			t.Fatalf("len(followupCalls) = %d, want 0", len(recorder.followupCalls))
		}
		if len(recorder.deleteCalls) != 0 {
			t.Fatalf("len(deleteCalls) = %d, want 0", len(recorder.deleteCalls))
		}
	})
}

func testSession() *discordgo.Session {
	return &discordgo.Session{}
}

func testInteraction() *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type:   discordgo.InteractionApplicationCommand,
			Locale: discordgo.Locale("en-US"),
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "test",
			},
		},
	}
}

func withDeferredCommandTestStubs(t *testing.T) *interactionCallRecorder {
	t.Helper()

	recorder := &interactionCallRecorder{}
	setDeferredCommandStubs(t,
		func(s *discordgo.Session, i *discordgo.Interaction, resp *discordgo.InteractionResponse) error {
			recorder.respondCalls = append(recorder.respondCalls, interactionRespondCall{resp: resp})
			return nil
		},
		func(s *discordgo.Session, i *discordgo.Interaction, edit *discordgo.WebhookEdit) (*discordgo.Message, error) {
			recorder.editCalls = append(recorder.editCalls, interactionEditCall{edit: edit})
			return &discordgo.Message{}, nil
		},
		func(s *discordgo.Session, i *discordgo.Interaction) error {
			recorder.deleteCalls = append(recorder.deleteCalls, interactionDeleteCall{})
			return nil
		},
		func(s *discordgo.Session, i *discordgo.Interaction, wait bool, data *discordgo.WebhookParams) (*discordgo.Message, error) {
			recorder.followupCalls = append(recorder.followupCalls, interactionFollowupCall{wait: wait, data: data})
			return &discordgo.Message{}, nil
		},
	)
	return recorder
}

func setDeferredCommandTiming(t *testing.T, ackWindow, margin time.Duration) {
	t.Helper()
	oldAck := interactionAckWindow
	oldMargin := deferSafetyMargin
	interactionAckWindow = ackWindow
	deferSafetyMargin = margin
	t.Cleanup(func() {
		interactionAckWindow = oldAck
		deferSafetyMargin = oldMargin
	})
}

func setDeferredCommandStubs(
	t *testing.T,
	respondFn func(s *discordgo.Session, i *discordgo.Interaction, resp *discordgo.InteractionResponse) error,
	editFn func(s *discordgo.Session, i *discordgo.Interaction, edit *discordgo.WebhookEdit) (*discordgo.Message, error),
	deleteFn func(s *discordgo.Session, i *discordgo.Interaction) error,
	followupFn func(s *discordgo.Session, i *discordgo.Interaction, wait bool, data *discordgo.WebhookParams) (*discordgo.Message, error),
) {
	t.Helper()
	oldRespond := interactionRespond
	oldEdit := interactionResponseEdit
	oldDelete := interactionResponseDelete
	oldFollowup := followupMessageCreate
	interactionRespond = respondFn
	interactionResponseEdit = editFn
	interactionResponseDelete = deleteFn
	followupMessageCreate = followupFn
	t.Cleanup(func() {
		interactionRespond = oldRespond
		interactionResponseEdit = oldEdit
		interactionResponseDelete = oldDelete
		followupMessageCreate = oldFollowup
	})
}

func alreadyAcknowledgedRESTError() error {
	return &discordgo.RESTError{
		Response: &http.Response{Status: "400 Bad Request"},
		Message: &discordgo.APIErrorMessage{
			Code:    40060,
			Message: "Interaction has already been acknowledged.",
		},
		ResponseBody: []byte(`{"message":"Interaction has already been acknowledged.","code":40060}`),
	}
}

func unknownInteractionRESTError() error {
	return &discordgo.RESTError{
		Response: &http.Response{Status: "404 Not Found"},
		Message: &discordgo.APIErrorMessage{
			Code:    10062,
			Message: "Unknown interaction",
		},
		ResponseBody: []byte(`{"message":"Unknown interaction","code":10062}`),
	}
}
