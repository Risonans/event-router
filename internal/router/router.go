// Package router решает, что делать с биллинговым событием: отправить в MMP или нет и с каким статусом.
package router

import (
	"errors"
	"fmt"

	"eventrouter/internal/event"
	"eventrouter/internal/sender"
	"eventrouter/internal/store"
)

// Status — итог обработки события.
type Status string

const (
	StatusSent             Status = "SENT"              // отправлено в MMP
	StatusNoMapping        Status = "NO_MAPPING"        // нет связки пользователя с MMP, финально
	StatusValidationFailed Status = "VALIDATION_FAILED" // битое или неподдерживаемое событие, финально
	StatusProjectDisabled  Status = "PROJECT_DISABLED"  // интеграция проекта выключена, финально
	StatusRetry            Status = "RETRY"             // повторить позже
	StatusFailed           Status = "FAILED"            // неустранимая ошибка отправки
	StatusGDPRNotApproved  Status = "GDPR_NOT_APPROVED" // согласие не получено, финально
)

// Result — результат обработки события.
type Result struct {
	Status Status
	Reason string
}

// Router обрабатывает события.
type Router struct {
	store  *store.Store
	sender sender.Sender
}

// New создаёт роутер.
func New(st *store.Store, s sender.Sender) *Router {
	return &Router{store: st, sender: s}
}

// Process обрабатывает одно событие.
func (r *Router) Process(ev event.Event) Result {
	if err := validate(ev); err != nil {
		return Result{StatusValidationFailed, err.Error()}
	}

	project, ok := r.store.Project(ev.ProjectID)
	if !ok {
		return Result{StatusValidationFailed, fmt.Sprintf("unknown project %d", ev.ProjectID)}
	}
	if !project.Enabled {
		return Result{StatusProjectDisabled, "integration is disabled"}
	}

	if project.ConsentCheckEnabled == nil || *project.ConsentCheckEnabled {
		consent, ok := r.store.Consent(ev.ProjectID, ev.UserID)
		if !ok || !consent.Approved {
			return Result{StatusGDPRNotApproved, "user consent is not approved"}
		}
	}

	eventName := "purchase"
	if ev.Type == event.TypeRefund {
		eventName = "refund"
	}

	mmpID, ok := r.store.MMPID(project.AppID, ev.UserID)
	if !ok {
		return Result{StatusNoMapping, "no mmp id for user"}
	}

	err := r.sender.Send(sender.Payload{
		EventID:   ev.ID,
		MMPID:     mmpID,
		EventName: eventName,
		Amount:    ev.Data.Amount,
		Currency:  ev.Data.Currency,
		Sandbox:   ev.DryRun == 1,
	})
	switch {
	case err == nil:
		return Result{StatusSent, ""}
	case errors.Is(err, sender.ErrTemporary):
		return Result{StatusRetry, err.Error()}
	default:
		return Result{StatusFailed, err.Error()}
	}
}

func validate(ev event.Event) error {
	switch {
	case ev.ID == "":
		return errors.New("empty event id")
	case ev.UserID == "":
		return errors.New("empty user id")
	case ev.ProjectID <= 0:
		return errors.New("empty project id")
	case ev.Type != event.TypePayment && ev.Type != event.TypeRefund:
		return fmt.Errorf("unsupported event type %q", ev.Type)
	case ev.Data.Amount <= 0:
		return fmt.Errorf("amount must be positive, got %v", ev.Data.Amount)
	case len(ev.Data.Currency) != 3:
		return fmt.Errorf("bad currency %q", ev.Data.Currency)
	}
	return nil
}
