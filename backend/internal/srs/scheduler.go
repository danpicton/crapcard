// Package srs wraps the spaced repetition algorithm.
//
// The rest of the application talks to Scheduler and CardState, never to the
// underlying library. That boundary matters for one specific reason: go-fsrs
// implements FSRS scheduling but not parameter optimisation — training a
// user's own weights from their review history exists only in the Rust and
// Python ports. Keeping the algorithm behind this interface means an
// optimiser can later run out of process and hand back a Params without the
// review pipeline noticing.
package srs

import (
	"encoding/json"
	"fmt"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// Rating is the answer a user gives to a card.
//
// The values are persisted in the review log and sent by the client;
// renumbering them would silently rewrite study history.
type Rating int

// The four grades, matching FSRS.
const (
	RatingAgain Rating = 1
	RatingHard  Rating = 2
	RatingGood  Rating = 3
	RatingEasy  Rating = 4
)

// String renders a rating for logs and API responses.
func (r Rating) String() string {
	switch r {
	case RatingAgain:
		return "again"
	case RatingHard:
		return "hard"
	case RatingGood:
		return "good"
	case RatingEasy:
		return "easy"
	}
	return "unknown"
}

// ParseRating converts a wire integer to a Rating, rejecting anything outside
// the four grades.
func ParseRating(v int) (Rating, error) {
	r := Rating(v)
	switch r {
	case RatingAgain, RatingHard, RatingGood, RatingEasy:
		return r, nil
	}
	return 0, fmt.Errorf("invalid rating %d: want 1 (again) to 4 (easy)", v)
}

// State is where a card sits in the learning cycle. Values are persisted.
type State int

// Card states, matching FSRS.
const (
	StateNew        State = 0
	StateLearning   State = 1
	StateReview     State = 2
	StateRelearning State = 3
)

// String renders a state for logs and API responses.
func (s State) String() string {
	switch s {
	case StateNew:
		return "new"
	case StateLearning:
		return "learning"
	case StateReview:
		return "review"
	case StateRelearning:
		return "relearning"
	}
	return "unknown"
}

// CardState is the scheduling memory of a single card. It is stored on the
// card row and is the only thing the algorithm needs to schedule the next
// review.
type CardState struct {
	Due           time.Time
	Stability     float64
	Difficulty    float64
	ElapsedDays   uint64
	ScheduledDays uint64
	Reps          uint64
	Lapses        uint64
	State         State
	LastReview    time.Time
}

// ReviewLog records one answer, in enough detail to replay a card's history —
// which is exactly what a parameter optimiser consumes.
type ReviewLog struct {
	Rating        Rating
	State         State
	ElapsedDays   uint64
	ScheduledDays uint64
	Review        time.Time
}

// Result pairs the card's new state with the log entry for the answer.
type Result struct {
	Card CardState
	Log  ReviewLog
}

// Scheduler schedules cards. Review advances a card by one answer; Preview
// shows what each possible answer would do without committing to one.
type Scheduler interface {
	Review(c CardState, now time.Time, rating Rating) Result
	Preview(c CardState, now time.Time) map[Rating]Result
	Retrievability(c CardState, now time.Time) float64
}

// Params are the tunable inputs to the algorithm, stored per user so a future
// optimiser can replace them without a schema change.
type Params struct {
	RequestRetention float64   `json:"request_retention"`
	MaximumInterval  float64   `json:"maximum_interval"`
	Weights          []float64 `json:"weights"`
	EnableShortTerm  bool      `json:"enable_short_term"`
	EnableFuzz       bool      `json:"enable_fuzz"`
}

// DefaultParams returns the library's default parameters — the population
// average FSRS ships with, used until a user has enough history to do better.
func DefaultParams() Params {
	d := fsrs.DefaultParam()
	return Params{
		RequestRetention: d.RequestRetention,
		MaximumInterval:  d.MaximumInterval,
		Weights:          append([]float64(nil), d.W[:]...),
		EnableShortTerm:  d.EnableShortTerm,
		EnableFuzz:       d.EnableFuzz,
	}
}

// MarshalJSON encodes the parameters.
func (p Params) MarshalJSON() ([]byte, error) {
	type alias Params
	return json.Marshal(alias(p))
}

// UnmarshalJSON decodes parameters, falling back to the defaults for any
// field the stored blob predates.
func (p *Params) UnmarshalJSON(b []byte) error {
	type alias Params
	tmp := alias(DefaultParams())
	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}
	*p = Params(tmp)
	if len(p.Weights) != len(fsrs.DefaultWeights()) {
		p.Weights = DefaultParams().Weights
	}
	return nil
}

// toLibrary converts Params into the library's own type.
func (p Params) toLibrary() fsrs.Parameters {
	lib := fsrs.DefaultParam()
	lib.RequestRetention = p.RequestRetention
	lib.MaximumInterval = p.MaximumInterval
	lib.EnableShortTerm = p.EnableShortTerm
	lib.EnableFuzz = p.EnableFuzz
	if len(p.Weights) == len(lib.W) {
		copy(lib.W[:], p.Weights)
	}
	return lib
}

// NewCard returns the scheduling state of a card that has never been studied.
// Its due date is the zero time, which keeps it at the front of the queue.
func NewCard() CardState {
	return fromLibrary(fsrs.NewCard())
}

// scheduler is the go-fsrs-backed implementation of Scheduler.
type scheduler struct {
	fsrs *fsrs.FSRS
}

// NewScheduler builds a Scheduler from the given parameters.
func NewScheduler(p Params) Scheduler {
	return &scheduler{fsrs: fsrs.NewFSRS(p.toLibrary())}
}

// Review applies an answer and returns the card's new state plus its log
// entry.
func (s *scheduler) Review(c CardState, now time.Time, rating Rating) Result {
	info := s.fsrs.Next(toLibrary(c), now, fsrs.Rating(rating))
	return Result{
		Card: fromLibrary(info.Card),
		Log:  logFromLibrary(info.ReviewLog),
	}
}

// Preview returns what each of the four answers would do to the card, for
// labelling the answer buttons. The card passed in is not modified.
func (s *scheduler) Preview(c CardState, now time.Time) map[Rating]Result {
	out := make(map[Rating]Result, 4)
	for rating, info := range s.fsrs.Repeat(toLibrary(c), now) {
		out[Rating(rating)] = Result{
			Card: fromLibrary(info.Card),
			Log:  logFromLibrary(info.ReviewLog),
		}
	}
	return out
}

// Retrievability estimates the probability of recalling the card now.
func (s *scheduler) Retrievability(c CardState, now time.Time) float64 {
	return s.fsrs.GetRetrievability(toLibrary(c), now)
}

func toLibrary(c CardState) fsrs.Card {
	return fsrs.Card{
		Due:           c.Due,
		Stability:     c.Stability,
		Difficulty:    c.Difficulty,
		ElapsedDays:   c.ElapsedDays,
		ScheduledDays: c.ScheduledDays,
		Reps:          c.Reps,
		Lapses:        c.Lapses,
		State:         fsrs.State(c.State),
		LastReview:    c.LastReview,
	}
}

func fromLibrary(c fsrs.Card) CardState {
	return CardState{
		Due:           c.Due,
		Stability:     c.Stability,
		Difficulty:    c.Difficulty,
		ElapsedDays:   c.ElapsedDays,
		ScheduledDays: c.ScheduledDays,
		Reps:          c.Reps,
		Lapses:        c.Lapses,
		State:         State(c.State),
		LastReview:    c.LastReview,
	}
}

func logFromLibrary(l fsrs.ReviewLog) ReviewLog {
	return ReviewLog{
		Rating:        Rating(l.Rating),
		State:         State(l.State),
		ElapsedDays:   l.ElapsedDays,
		ScheduledDays: l.ScheduledDays,
		Review:        l.Review,
	}
}
