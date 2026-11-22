// Пакет opsobs пишет структурированный журнал без личных данных (OP-R15, PV-R27).
package opsobs

import (
	"context"
	"io"
	"log/slog"
)

// Ключи, которых в записи быть не должно: имя, Telegram id, дата рождения, токен (OP-R15, PV-R27).
// Внутренний id ученика и псевдоним сюда не входят.
var forbiddenKeys = map[string]struct{}{
	"birth_date":   {},
	"display_name": {},
	"name":         {},
	"telegram_id":  {},
	"token":        {},
}

// New возвращает журнал JSON. Запись с запрещённым ключом не пишется целиком,
// чтобы значение не попало в файл.
func New(w io.Writer) *slog.Logger {
	return slog.New(&handler{next: slog.NewJSONHandler(w, nil)})
}

type handler struct {
	next slog.Handler
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *handler) Handle(ctx context.Context, record slog.Record) error {
	drop := false
	record.Attrs(func(attr slog.Attr) bool {
		if forbiddenAttr(attr) {
			drop = true
			return false
		}
		return true
	})
	if drop {
		return nil
	}
	return h.next.Handle(ctx, record)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	for _, attr := range attrs {
		if forbiddenAttr(attr) {
			return dropHandler{}
		}
	}
	return &handler{next: h.next.WithAttrs(attrs)}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{next: h.next.WithGroup(name)}
}

type dropHandler struct{}

func (dropHandler) Enabled(context.Context, slog.Level) bool { return false }

func (dropHandler) Handle(context.Context, slog.Record) error { return nil }

func (d dropHandler) WithAttrs([]slog.Attr) slog.Handler { return d }

func (d dropHandler) WithGroup(string) slog.Handler { return d }

func forbiddenAttr(attr slog.Attr) bool {
	attr.Value = attr.Value.Resolve()
	if _, ok := forbiddenKeys[attr.Key]; ok {
		return true
	}
	if attr.Value.Kind() != slog.KindGroup {
		return false
	}
	for _, item := range attr.Value.Group() {
		if forbiddenAttr(item) {
			return true
		}
	}
	return false
}
