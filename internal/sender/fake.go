// Package sender отправляет события в MMP (Mobile Measurement Partner).
// Здесь только фейковая реализация: она запоминает отправленное.
package sender

import (
	"errors"
	"sync"
)

// ErrTemporary — временная ошибка MMP (таймаут, 5xx): событие можно отправить позже.
var ErrTemporary = errors.New("mmp temporarily unavailable")

// Payload — то, что уходит в MMP.
type Payload struct {
	EventID   string
	MMPID     string
	EventName string
	Amount    float64
	Currency  string
	Sandbox   bool // true — отправка в тестовое окружение MMP
}

// Sender отправляет событие в MMP.
type Sender interface {
	Send(p Payload) error
}

// Fake запоминает отправленные события. Если задан Err, Send возвращает его.
type Fake struct {
	mu   sync.Mutex
	Err  error
	sent []Payload
}

// Send реализует Sender.
func (f *Fake) Send(p Payload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.sent = append(f.sent, p)
	return nil
}

// Sent возвращает копию отправленных событий.
func (f *Fake) Sent() []Payload {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Payload, len(f.sent))
	copy(out, f.sent)
	return out
}
