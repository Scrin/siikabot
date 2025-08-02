package logging

import (
	"context"

	"github.com/rs/zerolog"
)

type contextHook struct{}

func (h contextHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if v := e.GetCtx().Value(fieldContextKey{}); v != nil {
		fctx := v.(fieldContext)
		for k, v := range fctx.strValues {
			e.Str(k, v)
		}
		for k, v := range fctx.intValues {
			e.Int(k, v)
		}
	}
}

type fieldContextKey struct{}
type fieldContext struct {
	strValues map[string]string
	intValues map[string]int
}

func getFieldContext(ctx context.Context) fieldContext {
	if v := ctx.Value(fieldContextKey{}); v != nil {
		return v.(fieldContext)
	}
	return fieldContext{
		strValues: make(map[string]string),
		intValues: make(map[string]int),
	}
}

// ContextWithStr adds a string to the context such that it will be included in all log lines printed with this context.
func ContextWithStr(ctx context.Context, key, value string) context.Context {
	fctx := getFieldContext(ctx)
	fctx.strValues[key] = value
	return context.WithValue(ctx, fieldContextKey{}, fctx)
}

// ContextWithInt adds an int to the context such that it will be included in all log lines printed with this context.
func ContextWithInt(ctx context.Context, key string, value int) context.Context {
	fctx := getFieldContext(ctx)
	fctx.intValues[key] = value
	return context.WithValue(ctx, fieldContextKey{}, fctx)
}
