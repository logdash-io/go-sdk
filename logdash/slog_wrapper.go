package logdash

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"time"
)

type SlogTextHandler struct {
	opts              slog.HandlerOptions
	preformattedAttrs []string // contains all attrs that are already formatted
	groupPrefix       string   // contains all groups prefix with "."
	groups            []string // all groups started from WithGroup
	logger            *Logger
}

func NewSlogTextHandler(logger *Logger, opts slog.HandlerOptions) *SlogTextHandler {
	return &SlogTextHandler{opts: opts, logger: logger}
}

func (h *SlogTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.opts.Level.Level() <= level.Level()
}

func (h *SlogTextHandler) Handle(ctx context.Context, r slog.Record) error {
	// +1 for message
	// +1 for the source
	attrs := make([]string, len(h.preformattedAttrs)+1, len(h.preformattedAttrs)+r.NumAttrs()+2)
	attrs[0] = fmt.Sprintf("%q", r.Message)
	copy(attrs[1:], h.preformattedAttrs)
	r.Attrs(func(a slog.Attr) bool {
		a = h.safeReplaceAttr(h.groups, a)
		if a.Equal(slog.Attr{}) {
			return true
		}
		attrs = append(attrs, h.decorateAttr(a, h.groupPrefix))
		return true
	})
	// add source
	if h.opts.AddSource && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		a := slog.String(slog.SourceKey, fmt.Sprintf("%s:%d", f.File, f.Line))
		a = h.safeReplaceAttr(h.groups, a)
		if !a.Equal(slog.Attr{}) {
			attrs = append(attrs, h.decorateAttr(a, h.groupPrefix))
		}
	}

	// time is not added as text, because we put it into logdash logger as time.Time
	if r.Time.IsZero() {
		r.Time = time.Now()
	}

	h.logger.logWithAttrs(r.Time, convertSlogLevel(r.Level), attrs)
	return nil
}

func (h *SlogTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	pre := make([]string, len(h.preformattedAttrs), len(h.preformattedAttrs)+len(attrs))
	copy(pre, h.preformattedAttrs)
	for _, a := range attrs {
		a = h.safeReplaceAttr(h.groups, a)
		if a.Equal(slog.Attr{}) {
			continue
		}
		pre = append(pre, h.decorateAttr(a, h2.groupPrefix))
	}
	h2.preformattedAttrs = pre
	return &h2
}

func (h *SlogTextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.groupPrefix = fmt.Sprintf("%s%s.", h2.groupPrefix, name)
	h2.preformattedAttrs = slices.Clip(h2.preformattedAttrs)
	h2.groups = make([]string, len(h.groups), len(h.groups)+1)
	copy(h2.groups, h.groups)
	h2.groups = append(h2.groups, name)
	return &h2
}

func (h *SlogTextHandler) decorateAttr(a slog.Attr, groupPrefix string) string {
	a.Value = a.Value.Resolve()
	switch a.Value.Kind() {
	case slog.KindString:
		return fmt.Sprintf("%s%s=%q", groupPrefix, a.Key, a.Value.String())
	case slog.KindTime:
		return fmt.Sprintf("%s%s=%s", groupPrefix, a.Key, a.Value.Time().Format(time.RFC3339Nano))
	case slog.KindGroup:
		attrs := a.Value.Group()
		groupPrefix = fmt.Sprintf("%s%s.", groupPrefix, a.Key)
		for _, attr := range attrs {
			h.decorateAttr(attr, groupPrefix)
		}
	default:
		return fmt.Sprintf("%s%s=%s", groupPrefix, a.Key, a.Value)
	}
	panic("unreachable")
}

func (h *SlogTextHandler) safeReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if h.opts.ReplaceAttr == nil {
		return a
	}
	return h.opts.ReplaceAttr(groups, a)
}

// convertSlogLevel converts slog.Level to logdash.logLevel
func convertSlogLevel(level slog.Level) logLevel {
	// slog.Level is an int, so we can use comparison operators
	// slog.LevelDebug = -4, slog.LevelInfo = 0, slog.LevelWarn = 4, slog.LevelError = 8

	switch {
	case level < slog.LevelDebug:
		return logLevelSilly
	case level < slog.LevelInfo:
		return logLevelDebug
	case level < slog.LevelWarn:
		return logLevelInfo
	case level < slog.LevelError:
		return logLevelWarn
	default:
		return logLevelError
	}
}
