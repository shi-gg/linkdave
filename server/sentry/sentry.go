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
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	if r.Level >= slog.LevelWarn && hub != nil && hub.Client() != nil {
		tags := make(map[string]string)
		extra := make(map[string]interface{})
		var firstErr error
		prefix := h.group
		if prefix != "" {
			prefix += "."
		}

		for _, a := range h.attrs {
			extractAttr(tags, extra, &firstErr, prefix, a)
		}

		r.Attrs(func(a slog.Attr) bool {
			extractAttr(tags, extra, &firstErr, prefix, a)
			return true
		})

		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetTags(tags)
			scope.SetLevel(mapLevel(r.Level))
			if firstErr != nil {
				extra["log_message"] = r.Message
			}
			if len(extra) > 0 {
				scope.SetContext("extra", sentry.Context(extra))
			}
			if firstErr != nil {
				hub.CaptureException(firstErr)
			} else {
				hub.CaptureMessage(r.Message)
			}
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
	if name == "" {
		return h
	}

	group := name
	if h.group != "" {
		group = h.group + "." + name
	}

	return &Handler{
		parent: h.parent.WithGroup(name),
		attrs:  h.attrs,
		group:  group,
	}
}

func extractAttr(tags map[string]string, extra map[string]interface{}, firstErr *error, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		groupPrefix := prefix + a.Key + "."
		for _, ga := range a.Value.Group() {
			extractAttr(tags, extra, firstErr, groupPrefix, ga)
		}
		return
	}

	key := prefix + a.Key
	switch a.Key {
	case "guild_id", "session", "client", "version":
		tags[key] = a.Value.String()
	case "error":
		if err, ok := a.Value.Any().(error); ok {
			if *firstErr == nil {
				*firstErr = err
			}
		} else {
			extra[key] = a.Value.Any()
		}
	default:
		extra[key] = a.Value.Any()
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
