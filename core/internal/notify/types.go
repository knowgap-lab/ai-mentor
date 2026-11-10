package notify

import (
	"context"
	"time"
)

// Error — отказ записи. Error возвращает Code.
type Error struct {
	Code string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

// Failure — ответ транспорта. Status 0 означает обрыв сети.
// Status 429 несёт RetryAfter. Status 400 и 403 — получатель недостижим.
type Failure struct {
	Status     int
	RetryAfter time.Duration
}

func (f *Failure) Error() string {
	if f == nil {
		return ""
	}
	if f.Status == 0 {
		return "network"
	}
	return "status"
}

// Message — строка, которую Enqueue кладёт в outbox.
// Kind пишется в колонку kind. Нулевые CreatedAt и NextAttemptAt
// заменяются временем базы; нулевой срок означает, что строка уже доступна отправителю.
type Message struct {
	ID            string
	Kind          string
	DedupKey      string
	RecipientKind string
	RecipientID   string
	Template      string
	Params        map[string]any
	CreatedAt     time.Time
	NextAttemptAt time.Time
}

// Delivery — одно сообщение транспорту. ParseMode пустой: текст уходит как есть.
type Delivery struct {
	RecipientKind string
	RecipientID   string
	ChatID        string
	Text          string
	ParseMode     string
}

// Transport отправляет одно сообщение. Реализация в процессе не открывает сеть.
type Transport interface {
	Send(ctx context.Context, d Delivery) error
}
