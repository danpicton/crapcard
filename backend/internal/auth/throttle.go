package auth

import (
	"sync"
	"time"
)

// throttleFreeFailures is how many consecutive failures a username gets
// before delays kick in. A person mistyping a password should never see a
// 429; a dictionary attack should see nothing else.
const throttleFreeFailures = 5

// throttleMaxDelay caps the exponential backoff.
const throttleMaxDelay = 5 * time.Minute

// throttleMaxEntries bounds the tracking map so an attacker spraying made-up
// usernames cannot grow it without limit.
const throttleMaxEntries = 10_000

// loginThrottle applies per-username exponential backoff to failed logins.
//
// It is keyed on username rather than client IP because the resource being
// protected is the account: behind a proxy every request shares an IP, and an
// attacker rotating IPs would sail past an IP limit anyway. The state is
// in-memory — a restart forgives everyone, which is an acceptable trade for a
// single-process app.
type loginThrottle struct {
	mu      sync.Mutex
	entries map[string]*throttleEntry
}

type throttleEntry struct {
	failures    int
	retryAfter  time.Time
	lastFailure time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{entries: make(map[string]*throttleEntry)}
}

// retryIn reports how long the caller must wait before another attempt for
// this username, zero when the attempt is allowed.
func (l *loginThrottle) retryIn(username string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[username]
	if e == nil || now.After(e.retryAfter) {
		return 0
	}
	return e.retryAfter.Sub(now)
}

// fail records a failed attempt, extending the backoff.
func (l *loginThrottle) fail(username string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e := l.entries[username]
	if e == nil {
		if len(l.entries) >= throttleMaxEntries {
			l.prune(now)
		}
		if len(l.entries) >= throttleMaxEntries {
			// The map is at its cap with all-recent failures — an active
			// spray. Not tracking one more made-up name is the lesser evil
			// next to unbounded memory.
			return
		}
		e = &throttleEntry{}
		l.entries[username] = e
	}
	e.failures++
	e.lastFailure = now
	if over := e.failures - throttleFreeFailures; over >= 0 {
		delay := throttleMaxDelay
		// Guard the shift: past 2^9 seconds the cap has long since won.
		if over < 10 {
			delay = min(time.Second<<over, throttleMaxDelay)
		}
		e.retryAfter = now.Add(delay)
	}
}

// success clears the username's slate.
func (l *loginThrottle) success(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, username)
}

// prune drops entries whose backoff has long expired. Called with the lock
// held, only when the map is at its cap.
func (l *loginThrottle) prune(now time.Time) {
	for name, e := range l.entries {
		if now.Sub(e.lastFailure) > time.Hour {
			delete(l.entries, name)
		}
	}
	// A full map of *recent* failures means an active spray; dropping
	// arbitrary entries would let it through, so keep them and let the cap
	// simply stop admitting new names.
}
