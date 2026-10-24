package logdash

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"time"
)

// SlogTextHandler is a [slog.Handler] that logs to Logdash.
//
// It mimics [slog.TextHandler] behavior, but logs to Logdash instead of [io.Writer].
//
// [slog.HandlerOptions] are fully supported.
//
// Basic mapping between [slog.Level] and [logdash.logLevel] is:
//   - [slog.LevelDebug] (-4) → [logdash.Debug]
//   - [slog.LevelInfo] (0) → [logdash.Info]
//   - [slog.LevelWarn] (4) → [logdash.Warn]
//   - [slog.LevelError] (8) → [logdash.Error]
//
// Since [slog.Level] is an integer type, the mapping handles any intermediate or custom level values:
//   - Levels < [slog.LevelDebug] (-4) → [logdash.Silly]
//   - Levels ≥ [slog.LevelDebug] (-4) and < [slog.LevelInfo] (0) → [logdash.Debug]
//   - Levels ≥ [slog.LevelInfo] (0) and < [slog.LevelWarn] (4) → [logdash.Info]
//   - Levels ≥ [slog.LevelWarn] (4) and < [slog.LevelError] (8) → [logdash.Warn]
//   - Levels ≥ [slog.LevelError] (8) → [logdash.Error]
//
// If you want to log with a custom level, you can use [slog.Level] directly.
type SlogTextHandler struct {
	opts              slog.HandlerOptions
	preformattedAttrs []string // contains all attrs that are already formatted
	groupPrefix       string   // contains all groups prefix with "."
	groups            []string // all groups started from WithGroup
	logger            *Logger
}

// NewSlogTextHandler creates a new [SlogTextHandler] with the given [Logger] and [slog.HandlerOptions].
func NewSlogTextHandler(logger *Logger, opts slog.HandlerOptions) *SlogTextHandler {
	return &SlogTextHandler{opts: opts, logger: logger}
}

func (h *SlogTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo // the default of slog.HandlerOptions
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *SlogTextHandler) Handle(ctx context.Context, r slog.Record) error {
	// +1 for message
	// +1 for the source
	attrs := make([]string, len(h.preformattedAttrs)+1, len(h.preformattedAttrs)+r.NumAttrs()+2)
	attrs[0] = fmt.Sprintf("%q", r.Message)
	copy(attrs[1:], h.preformattedAttrs)
	r.Attrs(func(a slog.Attr) bool {
		attrs = h.appendAttr(attrs, h.groups, h.groupPrefix, a)
		return true
	})
	// add source
	if h.opts.AddSource && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		attrs = h.appendAttr(attrs, h.groups, h.groupPrefix, slog.String(slog.SourceKey, fmt.Sprintf("%s:%d", f.File, f.Line)))
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
		pre = h.appendAttr(pre, h.groups, h.groupPrefix, a)
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

// appendAttr appends a to dst as key=value, and each attr of a group as group.key=value.
//
// Like [slog.TextHandler], it rewrites every non-group attr with ReplaceAttr,
// inlines a group with an empty key, and leaves out empty attrs and groups.
func (h *SlogTextHandler) appendAttr(dst []string, groups []string, groupPrefix string, a slog.Attr) []string {
	a.Value = a.Value.Resolve()
	if h.opts.ReplaceAttr != nil && a.Value.Kind() != slog.KindGroup {
		a = h.opts.ReplaceAttr(groups, a)
		a.Value = a.Value.Resolve()
	}
	if a.Equal(slog.Attr{}) {
		return dst
	}

	switch a.Value.Kind() {
	case slog.KindGroup:
		if a.Key != "" {
			groups = append(slices.Clip(groups), a.Key)
			groupPrefix = fmt.Sprintf("%s%s.", groupPrefix, a.Key)
		}
		for _, attr := range a.Value.Group() {
			dst = h.appendAttr(dst, groups, groupPrefix, attr)
		}
		return dst
	case slog.KindString:
		return append(dst, fmt.Sprintf("%s%s=%q", groupPrefix, a.Key, a.Value.String()))
	case slog.KindTime:
		return append(dst, fmt.Sprintf("%s%s=%s", groupPrefix, a.Key, a.Value.Time().Format(time.RFC3339Nano)))
	default:
		return append(dst, fmt.Sprintf("%s%s=%s", groupPrefix, a.Key, a.Value))
	}
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
