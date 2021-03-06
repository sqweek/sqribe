package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sqweek/sqribe/midi"
	"github.com/sqweek/sqribe/score"
)

type XMLWriter struct {
	stream io.WriteCloser
	level int
	err error
}

func (writer *XMLWriter) Close() error {
	err2 := writer.stream.Close()
	if err2 != nil {
		return err2
	} else if writer.err != nil {
		return writer.err
	}
	return nil
}

func (writer *XMLWriter) Fmt(format string, args... interface{}) {
	_, writer.err = fmt.Fprintf(writer.stream, "%s%s\n", strings.Repeat("  ", writer.level), fmt.Sprintf(format, args...))
}

func (writer *XMLWriter) Tag(name string, attrs... interface{}) string {
	tag := name
	if len(attrs) > 0 {
		for i :=0; i < len(attrs); i += 2 {
			tag = fmt.Sprintf("%s %v=\"%v\"", tag, attrs[i], attrs[i+1])
		}
	}
	writer.Fmt("<%s>", tag)
	writer.level++
	return name
}

func (writer *XMLWriter) CloseTag(name string) {
	writer.level--
	writer.Fmt("</%s>", name)
}

func (writer *XMLWriter) EmptyTag(name string) {
	writer.Fmt("<%s />", name)
}

func (writer *XMLWriter) ContentTag(name string, content interface{}) {
	writer.Fmt("<%s>%v</%s>", name, content, name)
}

func ExportMXML(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	_, err = io.WriteString(file, xml.Header)
	if err != nil {
		return err
	}
	wr := XMLWriter{file, 0, nil}
	score := wr.Tag("score-partwise")
	mxmlIdent(&wr)
	mxmlParts(&wr)
	wr.CloseTag(score)
	err = wr.Close()
	return err
}

func mxmlIdent(wr *XMLWriter) {
	defer wr.CloseTag(wr.Tag("identification"))
	defer wr.CloseTag(wr.Tag("encoding"))
	wr.ContentTag("software", "sqribe")
	wr.ContentTag("encoding-date", time.Now().Format("2006-01-02"))
}

func mxmlParts(wr *XMLWriter) {
	staves := G.score.Staves()
	list := wr.Tag("part-list")
	for i, staff := range staves {
		mix := Mixer.For(staff)
		instName := midi.InstName(mix.Voice)
		id := fmt.Sprintf("P%d", i)
		xpart := wr.Tag("score-part", "id", id)
		wr.ContentTag("part-name", instName)
		xinst := wr.Tag("score-instrument", "id", fmt.Sprintf("%s-I1", id))
		wr.ContentTag("instrument-name", instName)
		wr.CloseTag(xinst)
		wr.CloseTag(xpart)
	}
	wr.CloseTag(list)
	for i, staff := range staves {
		id := fmt.Sprintf("P%d", i)
		mxmlPart(wr, staff, id)
	}
}

func rat(n, d int64) *big.Rat {
	return big.NewRat(n, d)
}

func flt(rat *big.Rat) float64 {
	float, _ := rat.Float64()
	return float
}

type NotePosIter struct {
	Note *score.Note

	notes []*score.Note
	iN int // index of next note
	beat *score.BeatRef
	iB int // index of beat
}

func (iter NotePosIter) done() bool {
	return iter.iN >= len(iter.notes)
}

func (iter *NotePosIter) advance() bool {
	if iter.done() {
		iter.Note = nil
		return false
	}
	iter.Note = iter.notes[iter.iN]
	iter.iN++
	prevb := iter.beat
	iter.beat = iter.Note.Beat
	if prevb == nil {
		iter.iB = iter.beat.BeatNum() - 1
	} else {
		iter.iB += iter.beat.Subtract(prevb)
	}
	return true
}

func (iter NotePosIter) Pos() *big.Rat {
	b := big.NewRat(int64(iter.iB), 1)
	b.Add(b, iter.Note.Offset)
	return b
}

func mxmlPart(wr *XMLWriter, staff *score.Staff, id string) {
	iter := &NotePosIter{notes: staff.Notes()}
	defer wr.CloseTag(wr.Tag("part", "id", id))
	ticks := 384
	divisions := ticks/4
	i0 := 0 // beat index of measure start
	m := 1 // number of measure
	for !iter.done() {
		meas := wr.Tag("measure", "number", m)
		if m == 1 {
			iter.advance()
			attr := wr.Tag("attributes")
			wr.ContentTag("divisions", divisions)
			key := wr.Tag("key")
			wr.ContentTag("fifths", int(staff.Key()))
			wr.ContentTag("mode", "major")
			wr.CloseTag(key)
			time := wr.Tag("time")
			wr.ContentTag("beats", 4)
			wr.ContentTag("beat-type", 4)
			wr.CloseTag(time)
			mxmlClef(wr, staff.Clef().Origin)
			wr.CloseTag(attr)
		}
		iN := i0 + 4
		curtick := (m - 1) * ticks
		var prevOffset *big.Rat
		for !iter.done() && flt(iter.Pos()) < float64(iN) {
			tick0 := dur2ticks(rat(1,4).Mul(rat(1,4), iter.Pos()), ticks)
			chord := false
			if prevOffset != nil && iter.Pos().Cmp(prevOffset) == 0 {
				chord = true
			} else {
				if tick0 < curtick {
					backup := wr.Tag("backup")
					wr.ContentTag("duration", curtick - tick0)
					wr.CloseTag(backup)
				} else if tick0 > curtick {
					mxmlRest(wr ,tick0 - curtick, divisions)
				}
			}
			prevOffset = iter.Pos()
			note := iter.Note
			durticks := dur2ticks(note.Duration, divisions)
			if durticks <= 0 {
				durticks = 1
			}
			mxmlNote(wr, &note.Pitch, note.Duration, durticks, chord)
			curtick = tick0 + durticks
			iter.advance()
		}
		if ticks > curtick {
			mxmlRest(wr, ticks - curtick, divisions) /* insert rest to finish out the measure */
		}
		wr.CloseTag(meas)

		i0 = iN
		m++
	}
}

func mxmlClef(wr *XMLWriter, origin uint8) {
	defer wr.CloseTag(wr.Tag("clef"))
	if origin == midi.PitchB5 {
		wr.ContentTag("sign", "G")
		wr.ContentTag("line", 2)
	} else if origin == midi.PitchD4 {
		wr.ContentTag("sign", "F")
		wr.ContentTag("line", 4)
	} else {
		wr.ContentTag("sign", midi.PitchName(origin)[0:1])
		wr.ContentTag("line", 3)
	}
}

func dur2ticks(duration *big.Rat, divisions int) int {
	dur := big.NewRat(int64(divisions), 1)
	dur.Mul(dur, duration)
	ticks := int(flt(dur) + 0.5)
	if ticks < 0 {
		ticks = 1
	}
	return ticks
}

func ticks2dur(ticks, divisions int) *big.Rat {
	r := []*big.Rat{rat(1,128), rat(3,256), rat(1,64), rat(3,128), rat(1,32), rat(3,64), rat(1,16), rat(3,32), rat(1,8), rat(3,16), rat(1,4), rat(3,8), rat(1,2), rat(3,4), rat(1,1), rat(3,2), rat(2,1), rat(3,1), rat(4,1)}
	d := float64(ticks) / float64(divisions)
	for i := 0; i + 1 < len(r); i++ {
		if d < (flt(r[i]) + flt(r[i + 1])) / 2 {
			return r[i]
		}
	}
	return r[len(r) - 1]
}

func mxmlRest(wr *XMLWriter, ticks, divisions int) {
	dur := ticks2dur(ticks, divisions)
	mxmlNote(wr, nil, dur, ticks, false)
}

func mxmlNote(wr *XMLWriter, pitch *uint8, duration *big.Rat, ticks int, chord bool) {
	defer wr.CloseTag(wr.Tag("note"))
	if chord {
		wr.EmptyTag("chord")
	}
	if pitch != nil {
		mxmlPitch(wr, *pitch)
	} else {
		wr.EmptyTag("rest")
	}
	wr.ContentTag("duration", ticks)
	wr.ContentTag("voice", 1)
	ntype, dot := mxmlNoteType(duration)
	wr.ContentTag("type", ntype)
	if dot {
		wr.EmptyTag("dot")
	}
}

func mxmlPitch(wr *XMLWriter, pitch uint8) {
	defer wr.CloseTag(wr.Tag("pitch"))
	s := midi.PitchName(pitch)
	wr.ContentTag("step", s[0:1])
	if s[1] == '#' {
		wr.ContentTag("alter", 1)
	} else if s[1] == 'b' {
		wr.ContentTag("alter", -1)
	}
	octave, _ := strconv.Atoi(s[len(s) - 1:])
	wr.ContentTag("octave", octave - 1)
}

func mxmlNoteType(dur *big.Rat) (string, bool) {
	switch dur.RatString() {
	case "1/32": return "128th", false
	case "1/16": return "64th", false
	case "1/8": return "32nd", false
	case "1/4": return "16th", false
	case "1/2": return "eighth", false
	case "1": return "quarter", false
	case "2": return "half", false
	case "3": return "half", true
	case "4": return "whole", false
	}
	return "quarter", false
}
