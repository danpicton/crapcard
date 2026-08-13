package srs_test

import (
	"testing"
	"time"

	"github.com/danpicton/crapcard/internal/srs"
)

func TestNewCardStartsInStateNew(t *testing.T) {
	c := srs.NewCard()

	if c.State != srs.StateNew {
		t.Fatalf("state = %v, want New", c.State)
	}
	if c.Reps != 0 || c.Lapses != 0 {
		t.Fatalf("new card has history: %+v", c)
	}
	if c.Stability != 0 || c.Difficulty != 0 {
		t.Fatalf("new card has memory state: %+v", c)
	}
}

func TestNewCardIsDueImmediately(t *testing.T) {
	// A never-studied card must appear in the queue straight away, so its due
	// date cannot be in the future.
	c := srs.NewCard()
	if c.Due.After(time.Now()) {
		t.Fatalf("new card due at %v, which is in the future", c.Due)
	}
}

func TestReviewAdvancesStateAndSchedulesForward(t *testing.T) {
	sched := srs.NewScheduler(srs.DefaultParams())
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	res := sched.Review(srs.NewCard(), now, srs.RatingGood)

	if res.Card.State == srs.StateNew {
		t.Fatalf("card is still New after a review")
	}
	if !res.Card.Due.After(now) {
		t.Fatalf("due = %v, want after the review time %v", res.Card.Due, now)
	}
	if res.Card.Reps != 1 {
		t.Fatalf("reps = %d, want 1", res.Card.Reps)
	}
	if !res.Card.LastReview.Equal(now) {
		t.Fatalf("last review = %v, want %v", res.Card.LastReview, now)
	}
}

func TestBetterRatingsScheduleFurtherOut(t *testing.T) {
	// The central property of the algorithm: the easier you found it, the
	// longer until you see it again.
	sched := srs.NewScheduler(srs.DefaultParams())
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	// Take a card through a couple of reviews so it reaches the Review state,
	// where intervals are measured in days rather than learning-step minutes.
	card := srs.NewCard()
	for range 3 {
		res := sched.Review(card, now, srs.RatingGood)
		card = res.Card
		now = card.Due
	}

	var last time.Duration
	for _, rating := range []srs.Rating{srs.RatingAgain, srs.RatingHard, srs.RatingGood, srs.RatingEasy} {
		res := sched.Review(card, now, rating)
		interval := res.Card.Due.Sub(now)
		if interval <= last {
			t.Fatalf("interval for %v (%v) is not longer than the previous rating's (%v)",
				rating, interval, last)
		}
		last = interval
	}
}

func TestAgainOnAReviewCardCountsALapse(t *testing.T) {
	sched := srs.NewScheduler(srs.DefaultParams())
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	card := srs.NewCard()
	for range 3 {
		res := sched.Review(card, now, srs.RatingGood)
		card = res.Card
		now = card.Due
	}
	if card.State != srs.StateReview {
		t.Fatalf("setup: card state = %v, want Review", card.State)
	}

	res := sched.Review(card, now, srs.RatingAgain)
	if res.Card.Lapses != card.Lapses+1 {
		t.Fatalf("lapses = %d, want %d", res.Card.Lapses, card.Lapses+1)
	}
	if res.Card.State != srs.StateRelearning {
		t.Fatalf("state after a lapse = %v, want Relearning", res.Card.State)
	}
}

func TestReviewProducesALogEntryForTheAnswerGiven(t *testing.T) {
	sched := srs.NewScheduler(srs.DefaultParams())
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	res := sched.Review(srs.NewCard(), now, srs.RatingHard)

	if res.Log.Rating != srs.RatingHard {
		t.Fatalf("log rating = %v, want Hard", res.Log.Rating)
	}
	if !res.Log.Review.Equal(now) {
		t.Fatalf("log review time = %v, want %v", res.Log.Review, now)
	}
	if res.Log.State != srs.StateNew {
		t.Fatalf("log state = %v, want the state the card was in before the answer (New)", res.Log.State)
	}
}

func TestPreviewOffersEveryRatingWithoutMutating(t *testing.T) {
	// The UI labels its four buttons with the interval each would produce, so
	// preview must cover all of them and must not advance the card.
	sched := srs.NewScheduler(srs.DefaultParams())
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	card := srs.NewCard()

	preview := sched.Preview(card, now)

	for _, rating := range []srs.Rating{srs.RatingAgain, srs.RatingHard, srs.RatingGood, srs.RatingEasy} {
		res, ok := preview[rating]
		if !ok {
			t.Fatalf("preview is missing rating %v", rating)
		}
		if !res.Card.Due.After(now) {
			t.Fatalf("preview for %v is due at %v, not after %v", rating, res.Card.Due, now)
		}
	}

	if card.Reps != 0 || card.State != srs.StateNew {
		t.Fatalf("Preview mutated the card it was given: %+v", card)
	}
}

func TestRetrievabilityFallsOffOverTime(t *testing.T) {
	sched := srs.NewScheduler(srs.DefaultParams())
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	card := srs.NewCard()
	for range 3 {
		res := sched.Review(card, now, srs.RatingGood)
		card = res.Card
		now = card.Due
	}

	soon := sched.Retrievability(card, now)
	later := sched.Retrievability(card, now.AddDate(0, 0, 60))
	if !(later < soon) {
		t.Fatalf("retrievability did not decay: %v at review time, %v 60 days later", soon, later)
	}
}

func TestParamsRoundTripThroughJSON(t *testing.T) {
	// Parameters are stored per user so a future optimiser can replace them
	// without a schema change.
	p := srs.DefaultParams()
	p.RequestRetention = 0.87

	encoded, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded srs.Params
	if err := decoded.UnmarshalJSON(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.RequestRetention != 0.87 {
		t.Fatalf("request retention = %v, want 0.87", decoded.RequestRetention)
	}
	if len(decoded.Weights) != len(p.Weights) {
		t.Fatalf("weights length = %d, want %d", len(decoded.Weights), len(p.Weights))
	}
}

func TestRatingsMapToStableWireValues(t *testing.T) {
	// These integers are persisted in the review log and sent by the client;
	// renumbering them would silently rewrite study history.
	if srs.RatingAgain != 1 || srs.RatingHard != 2 || srs.RatingGood != 3 || srs.RatingEasy != 4 {
		t.Fatalf("rating values changed: again=%d hard=%d good=%d easy=%d",
			srs.RatingAgain, srs.RatingHard, srs.RatingGood, srs.RatingEasy)
	}
	if srs.StateNew != 0 || srs.StateLearning != 1 || srs.StateReview != 2 || srs.StateRelearning != 3 {
		t.Fatalf("state values changed")
	}
}

func TestParseRatingRejectsOutOfRange(t *testing.T) {
	for _, v := range []int{0, 5, -1, 99} {
		if _, err := srs.ParseRating(v); err == nil {
			t.Fatalf("ParseRating(%d) succeeded, want an error", v)
		}
	}
	for _, v := range []int{1, 2, 3, 4} {
		if _, err := srs.ParseRating(v); err != nil {
			t.Fatalf("ParseRating(%d): %v", v, err)
		}
	}
}
