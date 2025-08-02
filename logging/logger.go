package logging

import (
	"os"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Setup() {
	var logContext zerolog.Context
	if config.ConsoleOutput {
		logContext = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).With()
	} else {
		logContext = log.With()
	}
	log.Logger = logContext.Caller().Logger().Hook(contextHook{})
	zerolog.LevelFieldName = "severity"
	zerolog.TimestampFieldName = "timestamp"
	zerolog.TimeFieldFormat = time.RFC3339Nano

	zerolog.SetGlobalLevel(zerolog.TraceLevel)
}
