// Package study drives the review loop: hand out the next due card, take the
// user's answer, and hand the card back to the scheduler.
//
// It is the join between notes (which know how to render a card) and cards
// (which hold scheduling state). It never learns what a note type is — it
// asks the note's generator to render whatever template the card names, so
// cloze cards will flow through unchanged.
package study

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/danpicton/crapcard/internal/cards"
	"github.com/danpicton/crapcard/internal/notes"
	"github.com/danpicton/crapcard/internal/srs"
)

// ErrQueueEmpty is returned when nothing in the deck is due.
var ErrQueueEmpty = errors.New("nothing due in this deck")

// Preview is what one possible answer would do to the card.
type Preview struct {
	Interval time.Duration
	Due      time.Time
}

// Question is the next card to study, rendered and ready to show.
type Question struct {
	CardID   int64
	NoteID   int64
	DeckID   int64
	Template string
	Question string
	Answer   string
	State    srs.State
	Counts   cards.Counts
	Previews map[srs.Rating]Preview
}

// AnswerResult is where the card landed after an answer.
type AnswerResult struct {
	CardID   int64
	Interval time.Duration
	Due      time.Time
	State    srs.State
	Counts   cards.Counts
}

// Service runs the study loop.
type Service struct {
	notes *notes.Repository
	cards *cards.Repository
	sched srs.Scheduler
}

// NewService creates a study Service scheduling with the given parameters.
func NewService(noteRepo *notes.Repository, cardRepo *cards.Repository, params srs.Params) *Service {
	return &Service{notes: noteRepo, cards: cardRepo, sched: srs.NewScheduler(params)}
}

// Next returns the next due card in the deck, rendered, along with the
// interval each possible answer would produce.
func (s *Service) Next(ctx context.Context, userID, deckID int64, now time.Time) (*Question, error) {
	due, err := s.cards.Due(ctx, userID, deckID, now, 1)
	if err != nil {
		return nil, err
	}
	if len(due) == 0 {
		return nil, ErrQueueEmpty
	}
	card := due[0]

	note, err := s.notes.Get(ctx, userID, card.NoteID)
	if err != nil {
		return nil, fmt.Errorf("load note for card %d: %w", card.ID, err)
	}
	gen, err := notes.GeneratorFor(note.Type)
	if err != nil {
		return nil, err
	}
	rendered, err := gen.Render(note.Fields, note.Config, card.Template)
	if err != nil {
		return nil, fmt.Errorf("render card %d: %w", card.ID, err)
	}

	counts, err := s.cards.Counts(ctx, userID, deckID, now)
	if err != nil {
		return nil, err
	}

	return &Question{
		CardID:   card.ID,
		NoteID:   note.ID,
		DeckID:   card.DeckID,
		Template: card.Template,
		Question: rendered.Question,
		Answer:   rendered.Answer,
		State:    card.State.State,
		Counts:   counts,
		Previews: s.previews(card.State, now),
	}, nil
}

// previews turns the scheduler's per-rating outcomes into intervals for the
// answer buttons.
func (s *Service) previews(state srs.CardState, now time.Time) map[srs.Rating]Preview {
	out := make(map[srs.Rating]Preview, 4)
	for rating, res := range s.sched.Preview(state, now) {
		out[rating] = Preview{
			Interval: res.Card.Due.Sub(now),
			Due:      res.Card.Due,
		}
	}
	return out
}

// Answer applies a rating to a card and persists the result.
func (s *Service) Answer(ctx context.Context, userID, cardID int64, rating srs.Rating, now time.Time) (*AnswerResult, error) {
	card, err := s.cards.Get(ctx, userID, cardID)
	if err != nil {
		return nil, err
	}

	res := s.sched.Review(card.State, now, rating)
	if err := s.cards.ApplyReview(ctx, userID, cardID, res.Card, res.Log); err != nil {
		return nil, err
	}

	counts, err := s.cards.Counts(ctx, userID, card.DeckID, now)
	if err != nil {
		return nil, err
	}

	return &AnswerResult{
		CardID:   cardID,
		Interval: res.Card.Due.Sub(now),
		Due:      res.Card.Due,
		State:    res.Card.State,
		Counts:   counts,
	}, nil
}

// DeckCounts summarises a deck's queue without starting a session, for the
// deck list.
func (s *Service) DeckCounts(ctx context.Context, userID, deckID int64, now time.Time) (cards.Counts, error) {
	return s.cards.Counts(ctx, userID, deckID, now)
}
