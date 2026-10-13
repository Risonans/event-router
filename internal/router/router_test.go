package router

import (
	"testing"

	"eventrouter/internal/event"
	"eventrouter/internal/sender"
	"eventrouter/internal/store"
)

func boolPtr(b bool) *bool { return &b }

func testStore() *store.Store {
	return store.New(
		[]store.Project{
			{ID: 1, AppID: "app.one", Enabled: true, ConsentCheckEnabled: boolPtr(true)},
			{ID: 2, AppID: "app.two", Enabled: false, ConsentCheckEnabled: boolPtr(true)},
		},
		[]store.Mapping{
			{AppID: "app.one", UserID: "alice", MMPID: "mmp-alice"},
		},
		[]store.Consent{
			{ProjectID: 1, UserID: "alice", Approved: true},
			{ProjectID: 1, UserID: "bob", Approved: true},
		},
	)
}

func payment(project int, user string) event.Event {
	return event.Event{
		ID: "ev-1", ProjectID: project, UserID: user, Type: event.TypePayment,
		Data: event.Billing{InvoiceID: 1, Amount: 9.99, Currency: "USD"},
	}
}

func TestProcess(t *testing.T) {
	created := payment(1, "alice")
	created.Type = event.TypeCreated

	tests := []struct {
		name string
		ev   event.Event
		want Status
	}{
		{"payment sent", payment(1, "alice"), StatusSent},
		{"no mapping", payment(1, "bob"), StatusNoMapping},
		{"disabled project", payment(2, "alice"), StatusProjectDisabled},
		{"unknown project", payment(99, "alice"), StatusValidationFailed},
		{"unsupported type", created, StatusValidationFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(testStore(), &sender.Fake{})
			if got := r.Process(tt.ev); got.Status != tt.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Reason, tt.want)
			}
		})
	}
}

func TestProcess_Payload(t *testing.T) {
	fake := &sender.Fake{}
	New(testStore(), fake).Process(payment(1, "alice"))

	sent := fake.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d payloads, want 1", len(sent))
	}
	if p := sent[0]; p.EventName != "purchase" || p.Amount != 9.99 || p.MMPID != "mmp-alice" {
		t.Fatalf("payload = %+v", p)
	}
}

func TestProcess_TemporaryErrorIsRetried(t *testing.T) {
	r := New(testStore(), &sender.Fake{Err: sender.ErrTemporary})
	if got := r.Process(payment(1, "alice")); got.Status != StatusRetry {
		t.Fatalf("status = %s, want RETRY", got.Status)
	}
}
