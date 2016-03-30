package score

import (
	"math/big"

	"github.com/sqweek/sqribe/plumb"

	. "github.com/sqweek/sqribe/core/types"
)

type BeatPoint interface {
	Beat() *BeatRef
	Offsetf() float64
}

type BeatPt struct {
	beat *BeatRef
	α float64
}

func (pt BeatPt) Beat() *BeatRef {
	return pt.beat
}

func (pt BeatPt) Offsetf() float64 {
	return pt.α
}

type BeatMap interface {
	ToFrame(beat BeatPoint) (FrameN, bool)
	ToBeat(frame FrameN) (BeatPoint, bool)
}

type ScoreOp interface {
	/* Apply returns a change event representing the op, or nil if nothing was changed. */
	apply(score *Score) interface{}
}

type UndoableOp interface {
	ScoreOp
	undo(score *Score)
}

type request struct {
	op ScoreOp
	result chan interface{}
}

type historyItem struct {
	op UndoableOp
	change interface{}
}

type Score struct {
	BeatList
	staves []*Staff
	beatLen *big.Rat
	plumb *plumb.Port

	updates chan request
	history []historyItem
	undone int // counter of number steps currently undone (for redo)

	quantApply chan chan bool
	quantCalc chan chan QuantizeBeats
}

type Measure struct {
	nbeats int /* length of measure */
	notes []Note /* sorted temporally */
}

func MkScore(plumb *plumb.Port) *Score {
	score := Score {
		beatLen: big.NewRat(1, 4),
		plumb: plumb,
		updates: make(chan request),
		history: make([]historyItem, 0, 32),
	}
	go func() {
		for req := range score.updates {
			change := req.op.apply(&score)
			req.result <- change
			if op, ok := req.op.(UndoableOp); ok && change != nil {
				if score.undone != 0 {
					score.history = score.history[0:len(score.history) - score.undone]
					score.undone = 0
				}
				if len(score.history) == cap(score.history) {
					// forget oldest change
					copy(score.history[0:], score.history[1:])
					score.history = score.history[:len(score.history) - 1]
				}
				score.history = append(score.history, historyItem{op, change})
			}
		}
	}()
	return &score
}

func (score *Score) Close() {
	close(score.updates)
}

func (score *Score) IsEmpty() bool {
	return len(score.staves) == 0 && score.Head == nil
}

func (score *Score) Sub(origin interface{}, c chan interface{}) {
	score.plumb.Sub(origin, c)
}

func (score *Score) Unsub(origin interface{}) {
	score.plumb.Unsub(origin)
}

/* returns true if the op actually changed something */
func (score *Score) update(op ScoreOp) bool {
	req := request{op, make(chan interface{})}
	score.updates <- req
	change := <-req.result
	if change != nil {
		score.plumb.C <- change
		return true
	}
	return false
}

func (score *Score) clearHistory() {
	score.history = score.history[0:0]
}

func (score *Score) Undo() bool {
	return score.update(&UndoOp{})
}

type UndoOp struct {}

func (op *UndoOp) apply(score *Score) interface{} {
	if score.undone >= len(score.history) {
		return nil // nothing left to undo
	}
	score.undone++
	item := score.history[len(score.history) - score.undone]
	item.op.undo(score)
	return item.change
}

func (score *Score) Redo() bool {
	return score.update(&RedoOp{})
}

type RedoOp struct{}

func (op *RedoOp) apply(score *Score) interface{} {
	if score.undone == 0 {
		return false
	}
	item := score.history[len(score.history) - score.undone]
	item.op.apply(score)
	score.undone--
	return item.change
}
