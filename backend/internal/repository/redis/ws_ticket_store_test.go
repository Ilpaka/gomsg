package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"goflow/backend/internal/domain"
)

func TestWSTicketStore_issueConsumeOneTime(t *testing.T) {
	t.Parallel()
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := NewWSTicketStore(rdb)
	uid := domain.ID("10000000-0000-4000-8000-000000000001")
	ticket, err := store.Issue(context.Background(), uid, time.Minute)
	if err != nil || ticket == "" {
		t.Fatalf("Issue: %v %q", err, ticket)
	}

	got, err := store.Consume(context.Background(), ticket)
	if err != nil || got != uid {
		t.Fatalf("Consume #1: %v %q want %q", err, got, uid)
	}

	if _, err := store.Consume(context.Background(), ticket); err == nil {
		t.Fatal("expected error on second consume (one-time ticket)")
	}
}

func TestWSTicketStore_consumeExpired(t *testing.T) {
	t.Parallel()
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := NewWSTicketStore(rdb)
	ticket, err := store.Issue(context.Background(), domain.ID("20000000-0000-4000-8000-000000000002"), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	srv.FastForward(time.Hour)

	if _, err := store.Consume(context.Background(), ticket); err == nil {
		t.Fatal("expected error after TTL expiry")
	}
}
