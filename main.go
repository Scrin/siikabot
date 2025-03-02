package main

import (
	"github.com/Scrin/siikabot/bot"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/logging"
	"github.com/rs/zerolog/log"
)

func main() {
	logging.Setup()
	config.LoadEnv()

	err := bot.Init()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize bot")
	}

	err = bot.Run()
	log.Fatal().Err(err).Msg("Bot exited")
}
