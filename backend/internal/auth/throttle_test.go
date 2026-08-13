package auth

import (
	"testing"
	"time"
)

func TestThrottleAllowsHonestMistakes(t *testing.T) {
	l := newLoginThrottle()
	now := time.Now()

	for range throttleFreeFailures {
		if wait := l.retryIn("dan", now); wait != 0 {
			t.Fatalf("throttled after only honest mistakes (wait %v)", wait)
		}
		l.fail("dan", now)
	}
}

func TestThrottleBacksOffExponentially(t *testing.T) {
	l := newLoginThrottle()
	now := time.Now()

	for range throttleFreeFailures + 1 {
		l.fail("dan", now)
	}
	first := l.retryIn("dan", now)
	if first <= 0 {
		t.Fatalf("no backoff after %d failures", throttleFreeFailures+1)
	}

	l.fail("dan", now)
	if second := l.retryIn("dan", now); second <= first {
		t.Fatalf("backoff did not grow: %v then %v", first, second)
	}

	// Far in the future the account is usable again, and the delay is capped.
	if wait := l.retryIn("dan", now.Add(throttleMaxDelay+time.Second)); wait != 0 {
		t.Fatalf("still throttled after the max delay: %v", wait)
	}
}

func TestThrottleIsPerUsernameAndClearsOnSuccess(t *testing.T) {
	l := newLoginThrottle()
	now := time.Now()

	for range throttleFreeFailures + 3 {
		l.fail("dan", now)
	}
	if l.retryIn("someone-else", now) != 0 {
		t.Fatalf("an unrelated username was throttled")
	}

	l.success("dan")
	if wait := l.retryIn("dan", now); wait != 0 {
		t.Fatalf("still throttled after a successful login: %v", wait)
	}
}
