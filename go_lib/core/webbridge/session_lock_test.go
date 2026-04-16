package webbridge

import (
	"testing"
	"time"
)

func TestUISessionLockClaimConflictAndExpiry(t *testing.T) {
	lock := NewUISessionLock(10 * time.Second)
	t0 := time.Unix(100, 0)

	state, ok := lock.Claim("client-a", "A", t0)
	if !ok {
		t.Fatalf("first claim should succeed")
	}
	if state.OwnerClientID != "client-a" {
		t.Fatalf("unexpected owner after first claim: %s", state.OwnerClientID)
	}

	state, ok = lock.Claim("client-b", "B", t0.Add(2*time.Second))
	if ok {
		t.Fatalf("second claim should fail while lease is active")
	}
	if state.OwnerClientID != "client-a" {
		t.Fatalf("owner should remain client-a, got %s", state.OwnerClientID)
	}

	state, ok = lock.Claim("client-b", "B", t0.Add(11*time.Second))
	if !ok {
		t.Fatalf("claim should succeed after lease expiry")
	}
	if state.OwnerClientID != "client-b" {
		t.Fatalf("owner should switch to client-b, got %s", state.OwnerClientID)
	}
}

func TestUISessionLockHeartbeatAndRelease(t *testing.T) {
	lock := NewUISessionLock(12 * time.Second)
	t0 := time.Unix(200, 0)

	if _, ok := lock.Claim("client-a", "A", t0); !ok {
		t.Fatalf("initial claim should succeed")
	}

	state, ok := lock.Heartbeat("client-a", t0.Add(6*time.Second))
	if !ok {
		t.Fatalf("heartbeat from owner should succeed")
	}
	wantLease := t0.Add(18 * time.Second)
	if !state.LeaseExpiresAt.Equal(wantLease) {
		t.Fatalf("heartbeat should extend lease to %v, got %v", wantLease, state.LeaseExpiresAt)
	}

	if _, ok := lock.Heartbeat("client-b", t0.Add(7*time.Second)); ok {
		t.Fatalf("heartbeat from non-owner should fail")
	}

	state, ok = lock.Release("client-a")
	if !ok {
		t.Fatalf("release from owner should succeed")
	}
	if state.OwnerClientID != "" {
		t.Fatalf("owner should be cleared after release, got %s", state.OwnerClientID)
	}
}
