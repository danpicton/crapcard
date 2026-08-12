package auth

import "time"

// SetNowForTest lets handler tests move the clock the login throttle sees.
func (h *Handler) SetNowForTest(now func() time.Time) { h.now = now }
