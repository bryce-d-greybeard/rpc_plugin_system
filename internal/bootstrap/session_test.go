package bootstrap

import (
	"testing"
	"time"
)

func TestSessionManagerConsumeOnce(t *testing.T) {
	mgr := NewManager(time.Minute)
	now := time.Unix(100, 0).UTC()
	s, err := mgr.New("echo", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := mgr.Consume("echo", s.SessionID, s.Token, now.Add(time.Second)); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if _, err := mgr.Consume("echo", s.SessionID, s.Token, now.Add(2*time.Second)); err == nil {
		t.Fatal("second consume unexpectedly succeeded")
	}
}

func TestSessionManagerRejectsExpiry(t *testing.T) {
	mgr := NewManager(time.Second)
	now := time.Unix(100, 0).UTC()
	s, err := mgr.New("echo", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := mgr.Consume("echo", s.SessionID, s.Token, now.Add(2*time.Second)); err == nil {
		t.Fatal("expired consume unexpectedly succeeded")
	}
}
