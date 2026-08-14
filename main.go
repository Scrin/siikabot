package main

import (
	"context"

	"github.com/Scrin/siikabot/bot"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/logging"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog/log"
)

func main() {
	config.LoadEnv()
	logging.Setup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracing, err := tracing.Init(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize tracing")
	}
	// Flushes whatever is still batched, so the spans of a shutting-down process are not lost
	defer func() {
		if err := shutdownTracing(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to shut down tracing cleanly")
		}
	}()

	err = bot.Init(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize bot")
	}

	err = bot.Run()
	log.Fatal().Err(err).Msg("Bot exited")
}
