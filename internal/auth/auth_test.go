package auth

import (
	"testing"
	"time"
)

func TestHashComparePassword(t *testing.T) {
	hash, err := HashPassword("supersecret")
	if err != nil {
		t.Fatal(err)
	}
	if !ComparePassword(hash, "supersecret") {
		t.Fatal("expected password to match")
	}
	if ComparePassword(hash, "wrong") {
		t.Fatal("expected password mismatch")
	}
}

func TestHashPasswordRejectsShortPassword(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
}

func TestLoginProtector(t *testing.T) {
	protector := NewLoginProtector()
	now := time.Now()
	ip := "127.0.0.1"
	for i := 0; i < 5; i++ {
		protector.Fail(ip, now)
	}
	if err := protector.Check(ip, now.Add(time.Minute)); err == nil {
		t.Fatal("expected lockout error")
	}
	protector.Success(ip)
	if err := protector.Check(ip, now.Add(time.Minute)); err != nil {
		t.Fatalf("unexpected error after success: %v", err)
	}
}
