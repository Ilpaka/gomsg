package service

import (
	"context"
	"testing"
	"time"

	"goflow/backend/internal/domain"
	apperr "goflow/backend/internal/pkg/errors"
)

type fakeTicketIssuer struct {
	issued string
	err    error
}

func (f *fakeTicketIssuer) Issue(ctx context.Context, userID domain.ID, ttl time.Duration) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.issued = string(userID)
	return "test-ticket-hex", nil
}

func TestWSTicketService_Issue_requiresUser(t *testing.T) {
	t.Parallel()
	s := NewWSTicketService(&fakeTicketIssuer{}, time.Minute)
	_, _, err := s.Issue(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Kind != apperr.KindUnauthorized {
		t.Fatalf("want unauthorized, got %v", err)
	}
}

func TestWSTicketService_Issue_success(t *testing.T) {
	t.Parallel()
	f := &fakeTicketIssuer{}
	s := NewWSTicketService(f, 90*time.Second)
	tok, sec, err := s.Issue(context.Background(), domain.ID("10000000-0000-4000-8000-000000000001"))
	if err != nil {
		t.Fatal(err)
	}
	if tok != "test-ticket-hex" || sec != 90 {
		t.Fatalf("unexpected token/exp: %s %d", tok, sec)
	}
	if f.issued != "10000000-0000-4000-8000-000000000001" {
		t.Fatalf("issuer user: %q", f.issued)
	}
}

func TestWSTicketService_Issue_nilStore(t *testing.T) {
	t.Parallel()
	s := NewWSTicketService(nil, time.Minute)
	_, _, err := s.Issue(context.Background(), domain.ID("10000000-0000-4000-8000-000000000001"))
	if err == nil {
		t.Fatal("expected error")
	}
}
