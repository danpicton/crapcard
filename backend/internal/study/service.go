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
	"github.com/danpicton/crapcard/internal/decks"
	"github.com/danpicton/crapcard/internal/notes"
	"github.com/danpicton/crapcard/internal/srs"
)

// ErrQueueEmpty is returned when nothing in the deck is due.
var ErrQueueEmpty = errors.New("nothing due in this deck")

// ErrCardSuspended is returned when an answer arrives for a suspended card.
// Suspension means "out of the study loop"; a stale tab must not schedule it.
var ErrCardSuspended = errors.New("card is suspended")

// Preview is what one possible answer would do to the card.
type Preview struct {
	Interval time.Duration
	Due      time.Time
}

// Question is the next card to study, rendered and ready to show.
type Question struct {
	CardID int64
	NoteID int64
	DeckID int64
	// DeckName is filled in so a session the user did not explicitly start —
	// landing on a card straight after signing in — can say which deck it is.
	DeckName string
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
	decks *decks.Repository
	sched srs.Scheduler
}

// NewService creates a study Service scheduling with the given parameters.
func NewService(
	noteRepo *notes.Repository,
	cardRepo *cards.Repository,
	deckRepo *decks.Repository,
	params srs.Params,
) *Service {
	return &Service{
		notes: noteRepo,
		cards: cardRepo,
		decks: deckRepo,
		sched: srs.NewScheduler(params),
	}
}

// NextAnywhere returns the next due card without the caller naming a deck,
// picking up wherever the user last left off. It backs the landing screen:
// signing in should put a card in front of you, not a menu.
func (s *Service) NextAnywhere(ctx context.Context, userID int64, h cards.Horizon) (*Question, error) {
	deckID, err := s.cards.NextDeckToStudy(ctx, userID, h)
	if errors.Is(err, cards.ErrNotFound) {
		return nil, ErrQueueEmpty
	}
	if err != nil {
		return nil, err
	}
	return s.Next(ctx, userID, deckID, h)
}

// Next returns the next due card in the deck, rendered, along with the
// interval each possible answer would produce.
func (s *Service) Next(ctx context.Context, userID, deckID int64, h cards.Horizon) (*Question, error) {
	due, err := s.cards.Due(ctx, userID, deckID, h, 1)
	if err != nil {
		return nil, err
	}
	if len(due) == 0 {
		return nil, ErrQueueEmpty
	}
	return s.question(ctx, userID, due[0], h)
}

// question renders one card as a ready-to-show Question.
func (s *Service) question(ctx context.Context, userID int64, card *cards.Card, h cards.Horizon) (*Question, error) {
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

	counts, err := s.cards.Counts(ctx, userID, card.DeckID, h)
	if err != nil {
		return nil, err
	}

	deck, err := s.decks.Get(ctx, userID, card.DeckID)
	if err != nil {
		return nil, fmt.Errorf("load deck for card %d: %w", card.ID, err)
	}

	return &Question{
		CardID:   card.ID,
		NoteID:   note.ID,
		DeckID:   card.DeckID,
		DeckName: deck.Name,
		Template: card.Template,
		Question: rendered.Question,
		Answer:   rendered.Answer,
		State:    card.State.State,
		Counts:   counts,
		Previews: s.previews(card.State, h.Now),
	}, nil
}

// Undo reverts the user's most recent answer and hands the card back,
// rendered and revealed, so it can simply be graded again.
func (s *Service) Undo(ctx context.Context, userID int64, h cards.Horizon) (*Question, error) {
	cardID, err := s.cards.UndoLastReview(ctx, userID)
	if err != nil {
		return nil, err
	}
	card, err := s.cards.Get(ctx, userID, cardID)
	if err != nil {
		return nil, err
	}
	return s.question(ctx, userID, card, h)
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
func (s *Service) Answer(ctx context.Context, userID, cardID int64, rating srs.Rating, h cards.Horizon) (*AnswerResult, error) {
	card, err := s.cards.Get(ctx, userID, cardID)
	if err != nil {
		return nil, err
	}
	if card.Suspended {
		return nil, ErrCardSuspended
	}

	res := s.sched.Review(card.State, h.Now, rating)
	if err := s.cards.ApplyReview(ctx, userID, cardID, card.State, res.Card, res.Log); err != nil {
		return nil, err
	}

	counts, err := s.cards.Counts(ctx, userID, card.DeckID, h)
	if err != nil {
		return nil, err
	}

	return &AnswerResult{
		CardID:   cardID,
		Interval: res.Card.Due.Sub(h.Now),
		Due:      res.Card.Due,
		State:    res.Card.State,
		Counts:   counts,
	}, nil
}

// DeckCounts summarises a deck's queue without starting a session, for the
// deck list.
func (s *Service) DeckCounts(ctx context.Context, userID, deckID int64, h cards.Horizon) (cards.Counts, error) {
	return s.cards.Counts(ctx, userID, deckID, h)
}
