package router

import (
	"errors"
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

func refund(project int, user string) event.Event {
	ev := payment(project, user)
	ev.Type = event.TypeRefund
	return ev
}

func refundWithAmount(project int, user string, amount float64) event.Event {
	ev := refund(project, user)
	ev.Data.Amount = amount
	return ev
}

func storeWithProject(project store.Project, mappings []store.Mapping, consents []store.Consent) *store.Store {
	return store.New([]store.Project{project}, mappings, consents)
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

func TestProcess_Refund(t *testing.T) {
	tests := []struct {
		name      string
		ev        event.Event
		senderErr error
		want      Status
		wantSent  int
	}{
		{
			name:     "sent",
			ev:       refund(1, "alice"),
			want:     StatusSent,
			wantSent: 1,
		},
		{
			name:     "zero amount",
			ev:       refundWithAmount(1, "alice", 0),
			want:     StatusValidationFailed,
			wantSent: 0,
		},
		{
			name:     "negative amount",
			ev:       refundWithAmount(1, "alice", -1),
			want:     StatusValidationFailed,
			wantSent: 0,
		},
		{
			name:     "no mapping",
			ev:       refund(1, "bob"),
			want:     StatusNoMapping,
			wantSent: 0,
		},
		{
			name:     "disabled project",
			ev:       refund(2, "alice"),
			want:     StatusProjectDisabled,
			wantSent: 0,
		},
		{
			name:      "temporary sender error",
			ev:        refund(1, "alice"),
			senderErr: sender.ErrTemporary,
			want:      StatusRetry,
			wantSent:  0,
		},
		{
			name:      "permanent sender error",
			ev:        refund(1, "alice"),
			senderErr: errors.New("boom"),
			want:      StatusFailed,
			wantSent:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &sender.Fake{Err: tt.senderErr}
			if got := New(testStore(), fake).Process(tt.ev); got.Status != tt.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Reason, tt.want)
			}
			if got := len(fake.Sent()); got != tt.wantSent {
				t.Fatalf("sent %d payloads, want %d", got, tt.wantSent)
			}
		})
	}
}

func TestProcess_RefundPayload(t *testing.T) {
	fake := &sender.Fake{}
	New(testStore(), fake).Process(refund(1, "alice"))

	sent := fake.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d payloads, want 1", len(sent))
	}
	if p := sent[0]; p.EventName != "refund" || p.Amount != 9.99 || p.MMPID != "mmp-alice" {
		t.Fatalf("payload = %+v", p)
	}
}

func TestProcess_GDPR(t *testing.T) {
	tests := []struct {
		name     string
		project  store.Project
		mappings []store.Mapping
		consents []store.Consent
		want     Status
		wantSent int
	}{
		{
			name: "not approved",
			project: store.Project{
				ID: 1, AppID: "app.one", Enabled: true, ConsentCheckEnabled: boolPtr(true),
			},
			mappings: []store.Mapping{
				{AppID: "app.one", UserID: "alice", MMPID: "mmp-alice"},
			},
			consents: []store.Consent{
				{ProjectID: 1, UserID: "alice", Approved: false},
			},
			want:     StatusGDPRNotApproved,
			wantSent: 0,
		},
		{
			name: "missing consent",
			project: store.Project{
				ID: 1, AppID: "app.one", Enabled: true, ConsentCheckEnabled: boolPtr(true),
			},
			mappings: []store.Mapping{
				{AppID: "app.one", UserID: "alice", MMPID: "mmp-alice"},
			},
			want:     StatusGDPRNotApproved,
			wantSent: 0,
		},
		{
			name: "consent check runs before mapping lookup",
			project: store.Project{
				ID: 1, AppID: "app.one", Enabled: true, ConsentCheckEnabled: boolPtr(true),
			},
			want:     StatusGDPRNotApproved,
			wantSent: 0,
		},
		{
			name: "disabled check",
			project: store.Project{
				ID: 1, AppID: "app.one", Enabled: true, ConsentCheckEnabled: boolPtr(false),
			},
			mappings: []store.Mapping{
				{AppID: "app.one", UserID: "alice", MMPID: "mmp-alice"},
			},
			consents: []store.Consent{
				{ProjectID: 1, UserID: "alice", Approved: false},
			},
			want:     StatusSent,
			wantSent: 1,
		},
		{
			name: "nil defaults to deny",
			project: store.Project{
				ID: 1, AppID: "app.one", Enabled: true,
			},
			mappings: []store.Mapping{
				{AppID: "app.one", UserID: "alice", MMPID: "mmp-alice"},
			},
			want:     StatusGDPRNotApproved,
			wantSent: 0,
		},
		{
			name: "nil still accepts approved consent",
			project: store.Project{
				ID: 1, AppID: "app.one", Enabled: true,
			},
			mappings: []store.Mapping{
				{AppID: "app.one", UserID: "alice", MMPID: "mmp-alice"},
			},
			consents: []store.Consent{
				{ProjectID: 1, UserID: "alice", Approved: true},
			},
			want:     StatusSent,
			wantSent: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &sender.Fake{}
			st := storeWithProject(
				tt.project,
				tt.mappings,
				tt.consents,
			)

			if got := New(st, fake).Process(refund(1, "alice")); got.Status != tt.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Reason, tt.want)
			}
			if got := len(fake.Sent()); got != tt.wantSent {
				t.Fatalf("sent %d payloads, want %d", got, tt.wantSent)
			}
		})
	}
}

func TestProcess_DryRunUsesSandbox(t *testing.T) {
	ev := payment(1, "alice")
	ev.DryRun = 1

	fake := &sender.Fake{}
	if got := New(testStore(), fake).Process(ev); got.Status != StatusSent {
		t.Fatalf("status = %s, want SENT", got.Status)
	}

	sent := fake.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d payloads, want 1", len(sent))
	}
	if !sent[0].Sandbox {
		t.Fatalf("sandbox = false, want true")
	}
}
