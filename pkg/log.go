package pkg

import (
	"context"
	"log/slog"
	"slices"
	"sync"

	task "github.com/langhuihui/gotask"
)

var _ slog.Handler = (*MultiLogHandler)(nil)

func ParseLevel(level string) slog.Level {
	var lv slog.LevelVar
	if level == "trace" {
		lv.Set(task.TraceLevel)
	} else {
		lv.UnmarshalText([]byte(level))
	}
	return lv.Level()
}

type HandlerInfo struct {
	slog.Handler
	origin slog.Handler
}

type MultiLogHandler struct {
	handlers                    []HandlerInfo
	attrChildren, groupChildren sync.Map
	parentLevel                 *slog.Level
	level                       *slog.Level
	parent                      *MultiLogHandler
}

// Detach removes this handler from the parent's children maps, allowing it to
// be garbage collected. Call this when the slog.Logger derived via With() is
// no longer needed (e.g. when a Publisher or Subscriber is disposed).
func (m *MultiLogHandler) Detach() {
	if m.parent != nil {
		m.parent.attrChildren.Delete(m)
		m.parent.groupChildren.Delete(m)
		m.parent = nil
	}
}

// DetachLogger removes the logger's underlying handler from its parent
// MultiLogHandler's children maps so it can be garbage collected.
func DetachLogger(logger *slog.Logger) {
	if logger != nil {
		if h, ok := logger.Handler().(*MultiLogHandler); ok {
			h.Detach()
		}
	}
}

func (m *MultiLogHandler) Add(h slog.Handler) {
	m.add(h, h)
}

func (m *MultiLogHandler) add(origin slog.Handler, warp slog.Handler) {
	m.handlers = append(m.handlers, HandlerInfo{origin: origin, Handler: warp})
	m.attrChildren.Range(func(key, value any) bool {
		child := key.(*MultiLogHandler)
		child.add(origin, origin.WithAttrs(value.([]slog.Attr)))
		return true
	})
	m.groupChildren.Range(func(key, value any) bool {
		child := key.(*MultiLogHandler)
		child.add(origin, origin.WithGroup(value.(string)))
		return true
	})
}

func (m *MultiLogHandler) Remove(h slog.Handler) {
	if i := slices.IndexFunc(m.handlers, func(info HandlerInfo) bool {
		return info.origin == h
	}); i != -1 {
		m.handlers = slices.Delete(m.handlers, i, i+1)
	}
	m.attrChildren.Range(func(key, value any) bool {
		child := key.(*MultiLogHandler)
		child.Remove(h)
		return true
	})
	m.groupChildren.Range(func(key, value any) bool {
		child := key.(*MultiLogHandler)
		child.Remove(h)
		return true
	})
}

func (m *MultiLogHandler) SetLevel(level slog.Level) {
	if m.level == nil {
		m.level = &level
	} else {
		*m.level = level
	}
}

// Enabled implements slog.Handler.
func (m *MultiLogHandler) Enabled(_ context.Context, l slog.Level) bool {
	if m.level != nil {
		return l >= *m.level
	}
	return l >= *m.parentLevel
}

// Handle implements slog.Handler.
func (m *MultiLogHandler) Handle(ctx context.Context, rec slog.Record) error {
	for _, h := range m.handlers {
		if err := h.Handle(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

// WithAttrs implements slog.Handler.
func (m *MultiLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	result := &MultiLogHandler{
		handlers:    make([]HandlerInfo, len(m.handlers)),
		parentLevel: m.parentLevel,
		parent:      m,
	}
	m.attrChildren.Store(result, attrs)
	if m.level != nil {
		result.parentLevel = m.level
	}
	for i, h := range m.handlers {
		result.handlers[i] = HandlerInfo{origin: h.origin, Handler: h.WithAttrs(attrs)}
	}
	return result
}

// WithGroup implements slog.Handler.
func (m *MultiLogHandler) WithGroup(name string) slog.Handler {
	result := &MultiLogHandler{
		handlers:    make([]HandlerInfo, len(m.handlers)),
		parentLevel: m.parentLevel,
		parent:      m,
	}
	m.groupChildren.Store(result, name)
	if m.level != nil {
		result.parentLevel = m.level
	}
	for i, h := range m.handlers {
		result.handlers[i] = HandlerInfo{origin: h.origin, Handler: h.WithGroup(name)}
	}
	return result
}
