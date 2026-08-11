package auth

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestDummyHashIsWellFormed(t *testing.T) {
	cost, err := bcrypt.Cost(dummyPasswordHash)
	if err != nil {
		t.Fatalf("dummy hash is not valid bcrypt: %v", err)
	}
	if cost != bcryptCost {
		t.Fatalf("dummy hash cost = %d, want %d", cost, bcryptCost)
	}
	start := time.Now()
	bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte("anything"))
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("compare against dummy hash took %v — too fast to mask a real compare", elapsed)
	}
}
