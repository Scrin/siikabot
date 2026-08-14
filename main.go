package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Scrin/siikabot/bot"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/logging"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog/log"
)

// shutdownGrace bounds how long the tracing flush may take once the bot has stopped. Long enough
// for a batch to reach Tempo, short enough not to delay a container restart noticeably.
const shutdownGrace = 15 * time.Second

func main() {
	config.LoadEnv()
	logging.Setup()

	// All the real work happens in run so that its deferred cleanup runs. Calling log.Fatal here
	// instead would exit through os.Exit, which skips defers entirely — that is precisely how the
	// tracing flush came to be registered but never executed.
	if err := run(); err != nil {
		log.Error().Err(err).Msg("Bot exited")
		os.Exit(1)
	}
	log.Info().Msg("Bot exited cleanly")
}

func run() error {
	// SIGTERM is how a container is stopped, so it is the ordinary shutdown path rather than an
	// exceptional one. Without this the process is killed where it stands and whatever spans are
	// still batched are lost on every single deploy.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := tracing.Init(ctx)
	if err != nil {
		return err
	}
	// Flushes whatever is still batched. Uses its own context because ctx is already cancelled by
	// the time this runs on the signal path, and an exporter handed a dead context flushes nothing.
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()

		if err := shutdownTracing(flushCtx); err != nil {
			log.Error().Err(err).Msg("Failed to shut down tracing cleanly")
		}
	}()

	if err := bot.Init(ctx); err != nil {
		return err
	}

	log.Info().Msg("Bot started")
	if err := bot.Run(ctx); err != nil {
		return err
	}

	// Sync returns without error when the context ends, which is the signal path
	log.Info().Msg("Shutting down")
	return nil
}
