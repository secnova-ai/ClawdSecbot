package webbridge

import (
	"strings"
	"sync"
	"time"
)

// UISessionState describes current UI session ownership.
type UISessionState struct {
	OwnerClientID    string
	OwnerClientLabel string
	LeaseExpiresAt   time.Time
	RemainingMs      int64
	LeaseDurationMs  int64
}

// UISessionLock enforces single active Web UI owner.
type UISessionLock struct {
	mu sync.Mutex

	ownerClientID    string
	ownerClientLabel string
	leaseExpiresAt   time.Time
	leaseDuration    time.Duration
}

func NewUISessionLock(leaseDuration time.Duration) *UISessionLock {
	if leaseDuration <= 0 {
		leaseDuration = 20 * time.Second
	}
	return &UISessionLock{
		leaseDuration: leaseDuration,
	}
}

func (l *UISessionLock) Claim(clientID, clientLabel string, now time.Time) (UISessionState, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now = normalizeNow(now)
	clientID = strings.TrimSpace(clientID)
	clientLabel = strings.TrimSpace(clientLabel)

	if clientID == "" {
		return l.snapshotLocked(now), false
	}

	if l.ownerClientID == "" || !l.leaseExpiresAt.After(now) || l.ownerClientID == clientID {
		l.ownerClientID = clientID
		if clientLabel != "" {
			l.ownerClientLabel = clientLabel
		}
		l.leaseExpiresAt = now.Add(l.leaseDuration)
		return l.snapshotLocked(now), true
	}

	return l.snapshotLocked(now), false
}

func (l *UISessionLock) Heartbeat(clientID string, now time.Time) (UISessionState, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now = normalizeNow(now)
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return l.snapshotLocked(now), false
	}

	if l.ownerClientID == clientID {
		l.leaseExpiresAt = now.Add(l.leaseDuration)
		return l.snapshotLocked(now), true
	}

	if l.ownerClientID != "" && !l.leaseExpiresAt.After(now) {
		l.clearLocked()
	}

	return l.snapshotLocked(now), false
}

func (l *UISessionLock) Release(clientID string) (UISessionState, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	clientID = strings.TrimSpace(clientID)
	if clientID == "" || l.ownerClientID == "" || l.ownerClientID != clientID {
		return l.snapshotLocked(time.Now()), false
	}

	l.clearLocked()
	return l.snapshotLocked(time.Now()), true
}

func (l *UISessionLock) clearLocked() {
	l.ownerClientID = ""
	l.ownerClientLabel = ""
	l.leaseExpiresAt = time.Time{}
}

func (l *UISessionLock) snapshotLocked(now time.Time) UISessionState {
	now = normalizeNow(now)
	remainingMs := int64(0)
	if l.ownerClientID != "" && l.leaseExpiresAt.After(now) {
		remainingMs = l.leaseExpiresAt.Sub(now).Milliseconds()
	}
	return UISessionState{
		OwnerClientID:    l.ownerClientID,
		OwnerClientLabel: l.ownerClientLabel,
		LeaseExpiresAt:   l.leaseExpiresAt,
		RemainingMs:      remainingMs,
		LeaseDurationMs:  l.leaseDuration.Milliseconds(),
	}
}

func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now()
	}
	return now
}
