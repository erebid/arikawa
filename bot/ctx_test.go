package bot

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/discord"
	"github.com/diamondburned/arikawa/gateway"
	"github.com/diamondburned/arikawa/state"
)

type testc struct {
	Ctx     *Context
	Return  chan interface{}
	Counter uint64
	Typed   bool
}

func (t *testc) MーBumpCounter(interface{}) {
	t.Counter++
}

func (t *testc) GetCounter(_ *gateway.MessageCreateEvent) {
	t.Return <- strconv.FormatUint(t.Counter, 10)
}

func (t *testc) Send(_ *gateway.MessageCreateEvent, args ...string) error {
	t.Return <- args
	return errors.New("oh no")
}

func (t *testc) Custom(_ *gateway.MessageCreateEvent, c *customManualParsed) {
	t.Return <- c.args
}

func (t *testc) Variadic(_ *gateway.MessageCreateEvent, c ...*customParsed) {
	t.Return <- c[len(c)-1]
}

func (t *testc) TrailCustom(_ *gateway.MessageCreateEvent, s string, c *customManualParsed) {
	t.Return <- c.args
}

func (t *testc) Content(_ *gateway.MessageCreateEvent, c RawArguments) {
	t.Return <- c
}

func (t *testc) NoArgs(_ *gateway.MessageCreateEvent) error {
	return errors.New("passed")
}

func (t *testc) Noop(_ *gateway.MessageCreateEvent) {
}

func (t *testc) OnTyping(_ *gateway.TypingStartEvent) {
	t.Typed = true
}

func TestNewContext(t *testing.T) {
	var state = &state.State{
		Store: state.NewDefaultStore(nil),
	}

	c, err := New(state, &testc{})
	if err != nil {
		t.Fatal("Failed to create new context:", err)
	}

	if !reflect.DeepEqual(c.Subcommands(), c.subcommands) {
		t.Fatal("Subcommands mismatch.")
	}
}

func TestContext(t *testing.T) {
	var given = &testc{}
	var state = &state.State{
		Store: state.NewDefaultStore(nil),
	}

	s, err := NewSubcommand(given)
	if err != nil {
		t.Fatal("Failed to create subcommand:", err)
	}

	var ctx = &Context{
		Subcommand: s,
		State:      state,
	}

	t.Run("init commands", func(t *testing.T) {
		if err := ctx.Subcommand.InitCommands(ctx); err != nil {
			t.Fatal("Failed to init commands:", err)
		}

		if given.Ctx == nil {
			t.Fatal("given's Context field is nil")
		}

		if given.Ctx.State.Store == nil {
			t.Fatal("given's State is nil")
		}
	})

	t.Run("find commands", func(t *testing.T) {
		cmd := ctx.FindCommand("", "NoArgs")
		if cmd == nil {
			t.Fatal("Failed to find NoArgs")
		}
	})

	t.Run("help", func(t *testing.T) {
		if h := ctx.Help(); h == "" {
			t.Fatal("Empty help?")
		}
		if h := ctx.HelpAdmin(); h == "" {
			t.Fatal("Empty admin help?")
		}
	})

	t.Run("middleware", func(t *testing.T) {
		ctx.HasPrefix = NewPrefix("pls do ")

		// This should trigger the middleware first.
		if err := expect(ctx, given, "1", "pls do getCounter"); err != nil {
			t.Fatal("Unexpected error:", err)
		}
	})

	t.Run("typing event", func(t *testing.T) {
		typing := &gateway.TypingStartEvent{}

		if err := ctx.callCmd(typing); err != nil {
			t.Fatal("Failed to call with TypingStart:", err)
		}

		if !given.Typed {
			t.Fatal("Typed bool is false")
		}
	})

	t.Run("call command", func(t *testing.T) {
		// Set a custom prefix
		ctx.HasPrefix = NewPrefix("~")

		var (
			strings = "hacka doll no. 3"
			expects = []string{"hacka", "doll", "no.", "3"}
		)

		if err := expect(ctx, given, expects, "~send "+strings); err.Error() != "oh no" {
			t.Fatal("Unexpected error:", err)
		}
	})

	t.Run("call command rawarguments", func(t *testing.T) {
		ctx.HasPrefix = NewPrefix("!")
		expects := RawArguments("just things")

		if err := expect(ctx, given, expects, "!content just things"); err != nil {
			t.Fatal("Unexpected call error:", err)
		}
	})

	t.Run("call command custom manual parser", func(t *testing.T) {
		ctx.HasPrefix = NewPrefix("!")
		expects := []string{"arg1", ":)"}

		if err := expect(ctx, given, expects, "!custom arg1 :)"); err != nil {
			t.Fatal("Unexpected call error:", err)
		}
	})

	t.Run("call command custom variadic parser", func(t *testing.T) {
		ctx.HasPrefix = NewPrefix("!")
		expects := &customParsed{true}

		if err := expect(ctx, given, expects, "!variadic bruh moment"); err != nil {
			t.Fatal("Unexpected call error:", err)
		}
	})

	t.Run("call command custom trailing manual parser", func(t *testing.T) {
		ctx.HasPrefix = NewPrefix("!")
		expects := []string{}

		if err := expect(ctx, given, expects, "!trailCustom hime_arikawa"); err != nil {
			t.Fatal("Unexpected call error:", err)
		}
	})

	testMessage := func(content string) error {
		// Mock a messageCreate event
		m := &gateway.MessageCreateEvent{
			Message: discord.Message{
				Content: content,
			},
		}

		return ctx.callCmd(m)
	}

	t.Run("call command without args", func(t *testing.T) {
		ctx.HasPrefix = NewPrefix("")

		if err := testMessage("noArgs"); err.Error() != "passed" {
			t.Fatal("unexpected error:", err)
		}
	})

	// Test error cases

	t.Run("call unknown command", func(t *testing.T) {
		ctx.HasPrefix = NewPrefix("joe pls ")

		err := testMessage("joe pls no")

		if err == nil || !strings.HasPrefix(err.Error(), "Unknown command:") {
			t.Fatal("unexpected error:", err)
		}
	})

	// Test subcommands

	t.Run("register subcommand", func(t *testing.T) {
		ctx.HasPrefix = NewPrefix("run ")

		sub := &testc{}

		_, err := ctx.RegisterSubcommand(sub)
		if err != nil {
			t.Fatal("Failed to register subcommand:", err)
		}

		if err := testMessage("run testc noop"); err != nil {
			t.Fatal("Unexpected error:", err)
		}

		expects := RawArguments("hackadoll no. 3")

		if err := expect(ctx, sub, expects, "run testc content hackadoll no. 3"); err != nil {
			t.Fatal("Unexpected call error:", err)
		}

		if cmd := ctx.FindCommand("testc", "Noop"); cmd == nil {
			t.Fatal("Failed to find subcommand Noop")
		}
	})
}

func expect(ctx *Context, given *testc, expects interface{}, content string) (call error) {
	// Return channel for testing
	ret := make(chan interface{})
	given.Return = ret

	// Mock a messageCreate event
	m := &gateway.MessageCreateEvent{
		Message: discord.Message{
			Content: content,
		},
	}

	var callCh = make(chan error)
	go func() {
		callCh <- ctx.callCmd(m)
	}()

	select {
	case arg := <-ret:
		if !reflect.DeepEqual(arg, expects) {
			return fmt.Errorf("returned argument is invalid: %v", arg)
		}
		call = <-callCh
		return

	case call = <-callCh:
		return fmt.Errorf("expected return before error: %w", call)

	case <-time.After(time.Second):
		return errors.New("Timed out while waiting")
	}
}

func BenchmarkConstructor(b *testing.B) {
	var state = &state.State{
		Store: state.NewDefaultStore(nil),
	}

	for i := 0; i < b.N; i++ {
		_, _ = New(state, &testc{})
	}
}

func BenchmarkCall(b *testing.B) {
	var given = &testc{}
	var state = &state.State{
		Store: state.NewDefaultStore(nil),
	}

	s, _ := NewSubcommand(given)

	var ctx = &Context{
		Subcommand: s,
		State:      state,
		HasPrefix:  NewPrefix("~"),
	}

	m := &gateway.MessageCreateEvent{
		Message: discord.Message{
			Content: "~noop",
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx.callCmd(m)
	}
}

func BenchmarkHelp(b *testing.B) {
	var given = &testc{}
	var state = &state.State{
		Store: state.NewDefaultStore(nil),
	}

	s, _ := NewSubcommand(given)

	var ctx = &Context{
		Subcommand: s,
		State:      state,
		HasPrefix:  NewPrefix("~"),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ctx.Help()
	}
}

type hasID struct {
	ChannelID discord.Snowflake
}

type embedsID struct {
	*hasID
	*embedsID
}

type hasChannelInName struct {
	ID discord.Snowflake
}

func TestReflectChannelID(t *testing.T) {
	var s = &hasID{
		ChannelID: 69420,
	}

	t.Run("hasID", func(t *testing.T) {
		if id := reflectChannelID(s); id != 69420 {
			t.Fatal("unexpected channelID:", id)
		}
	})

	t.Run("embedsID", func(t *testing.T) {
		var e = &embedsID{
			hasID: s,
		}

		if id := reflectChannelID(e); id != 69420 {
			t.Fatal("unexpected channelID:", id)
		}
	})

	t.Run("hasChannelInName", func(t *testing.T) {
		var s = &hasChannelInName{
			ID: 69420,
		}

		if id := reflectChannelID(s); id != 69420 {
			t.Fatal("unexpected channelID:", id)
		}
	})
}

func BenchmarkReflectChannelID_1Level(b *testing.B) {
	var s = &hasID{
		ChannelID: 69420,
	}

	for i := 0; i < b.N; i++ {
		_ = reflectChannelID(s)
	}
}

func BenchmarkReflectChannelID_5Level(b *testing.B) {
	var s = &embedsID{
		nil,
		&embedsID{
			nil,
			&embedsID{
				nil,
				&embedsID{
					hasID: &hasID{
						ChannelID: 69420,
					},
				},
			},
		},
	}

	for i := 0; i < b.N; i++ {
		_ = reflectChannelID(s)
	}
}
