package sentry

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

func Init(dsn, version string) {
	if dsn == "" {
		return
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Release:          version,
		Environment:      os.Getenv("SENTRY_ENVIRONMENT"),
		EnableTracing:    false,
		AttachStacktrace: true,
	})
	if err != nil {
		slog.Error("sentry init failed", slog.Any("error", err))
	}
}

func Flush(timeout time.Duration) {
	sentry.Flush(timeout)
}

type Handler struct {
	parent slog.Handler
	attrs  []slog.Attr
	group  string
}

func NewHandler(parent slog.Handler) slog.Handler {
	return &Handler{parent: parent}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.parent.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn && sentry.CurrentHub().Client() != nil {
		event := sentry.NewEvent()
		event.Message = r.Message
		event.Level = mapLevel(r.Level)
		event.Timestamp = r.Time

		tags := make(map[string]string)
		extra := make(map[string]interface{})

		for _, a := range h.attrs {
			extractAttr(tags, extra, event, a)
		}

		r.Attrs(func(a slog.Attr) bool {
			extractAttr(tags, extra, event, a)
			return true
		})

		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTags(tags)
			if len(extra) > 0 {
				scope.SetContext("extra", sentry.Context(extra))
			}
			sentry.CaptureEvent(event)
		})
	}

	return h.parent.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		parent: h.parent.WithAttrs(attrs),
		attrs:  append(append([]slog.Attr{}, h.attrs...), attrs...),
		group:  h.group,
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		parent: h.parent.WithGroup(name),
		attrs:  h.attrs,
		group:  name,
	}
}

func extractAttr(tags map[string]string, extra map[string]interface{}, event *sentry.Event, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		for _, ga := range a.Value.Group() {
			extractAttr(tags, extra, event, ga)
		}
		return
	}

	switch a.Key {
	case "guild_id", "session", "client", "version":
		tags[a.Key] = a.Value.String()
	case "error":
		if err, ok := a.Value.Any().(error); ok {
			event.Exception = append(event.Exception, sentry.Exception{
				Type:       "error",
				Value:      err.Error(),
				Stacktrace: sentry.NewStacktrace(),
			})
		} else {
			extra[a.Key] = a.Value.Any()
		}
	default:
		extra[a.Key] = a.Value.Any()
	}
}

func mapLevel(level slog.Level) sentry.Level {
	switch {
	case level >= slog.LevelError:
		return sentry.LevelError
	case level >= slog.LevelWarn:
		return sentry.LevelWarning
	case level >= slog.LevelInfo:
		return sentry.LevelInfo
	default:
		return sentry.LevelDebug
	}
}
