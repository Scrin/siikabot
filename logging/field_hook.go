package logging

import "github.com/rs/zerolog"

type FieldHook struct {
	Fields map[string]string
}

func (h FieldHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	for k, v := range h.Fields {
		e.Str(k, v)
	}
}
