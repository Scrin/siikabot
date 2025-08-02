package main

import (
	"context"

	"github.com/Scrin/siikabot/bot"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/logging"
	"github.com/rs/zerolog/log"
)

func main() {
	config.LoadEnv()
	logging.Setup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := bot.Init(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize bot")
	}

	err = bot.Run()
	log.Fatal().Err(err).Msg("Bot exited")
}
