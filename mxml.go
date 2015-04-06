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

	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/score"
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

func (writer *XMLWriter) Tag(name string, attrs... interface{}) func() {
	tag := name
	if len(attrs) > 0 {
		for i :=0; i < len(attrs); i += 2 {
			tag = fmt.Sprintf("%s %v=\"%v\"", tag, attrs[i], attrs[i+1])
		}
	}
	writer.Fmt("<%s>", tag)
	writer.level++
	return func() {writer.CloseTag(name)}
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
	closeTag := wr.Tag("score-partwise")
	mxmlIdent(&wr)
	mxmlParts(&wr)
	closeTag()
	err = wr.Close()
	return err
}

func mxmlIdent(wr *XMLWriter) {
	defer wr.Tag("identification")()
	defer wr.Tag("encoding")()
	wr.ContentTag("software", "sqribe")
	wr.ContentTag("encoding-date", time.Now().Format("2006-01-02"))
}

func mxmlParts(wr *XMLWriter) {
	staves := G.score.SavedStaves()
	closeList := wr.Tag("part-list")
	for _, staff := range staves {
		inst := midi.InstName(staff.Voice)
		closePart := wr.Tag("score-part", "id", staff.Name)
		wr.ContentTag("part-name", inst)
		closeInst := wr.Tag("score-instrument", "id", fmt.Sprintf("%s-I1", staff.Name))
		wr.ContentTag("instrument-name", inst)
		closeInst()
		closePart()
	}
	closeList()
	for _, staff := range staves {
		mxmlPart(wr, staff)
	}
}

func flt(rat *big.Rat) float64 {
	float, _ := rat.Float64()
	return float
}

func mxmlPart(wr *XMLWriter, staff score.SavedStaff) {
	defer wr.Tag("part", "id", staff.Name)()
	ticks := 384
	i0 := 0
	inote := 0
	m := 1
	for {
		closeMeas := wr.Tag("measure", "number", m)
		if m == 1 {
			closeAttr := wr.Tag("attributes")
			closeKey := wr.Tag("key")
			wr.ContentTag("fifths", staff.Nsharps)
			wr.ContentTag("mode", "major")
			closeKey()
			closeTime := wr.Tag("time")
			wr.ContentTag("beats", 4)
			wr.ContentTag("beat-type", 4)
			closeTime()
			wr.ContentTag("divisions", ticks / 4)
			mxmlClef(wr, staff.Origin)
			closeAttr()
		}
		iN := i0 + 4
		var prevOffset *big.Rat
		for inote < len(staff.Notes) && flt(staff.Notes[inote].Offset) < float64(iN) {
			// TODO insert rests
			chord := false
			if prevOffset != nil && staff.Notes[inote].Offset.Cmp(prevOffset) == 0 {
				chord = true
			}
			prevOffset = staff.Notes[inote].Offset
			note := staff.Notes[inote]
			mxmlNote(wr, note, ticks, chord)
			inote++
		}
		closeMeas()

		i0 = iN
		if inote >= len(staff.Notes) {
			break
		}
		m++
	}
}

func mxmlClef(wr *XMLWriter, origin uint8) {
	defer wr.Tag("clef")()
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

func mxmlNote(wr *XMLWriter, note score.SavedNote, mticks int, chord bool) {
	dur := big.NewRat(int64(mticks), 4)
	dur.Mul(dur, note.Duration)
	ticks := int(flt(dur) + 0.5)
	if ticks <= 0 {
		ticks = 1
	}

	defer wr.Tag("note")()
	mxmlPitch(wr, note.Pitch)
	wr.ContentTag("duration", ticks)
	wr.ContentTag("voice", 1)
	ntype, dot := mxmlNoteType(note)
	wr.ContentTag("type", ntype)
	if dot {
		wr.EmptyTag("dot")
	}
	if chord {
		wr.EmptyTag("chord")
	}
}

func mxmlPitch(wr *XMLWriter, pitch uint8) {
	defer wr.Tag("pitch")()
	s := midi.PitchName(pitch)
	wr.ContentTag("step", s[0:1])
	octave, _ := strconv.Atoi(s[len(s) - 1:])
	wr.ContentTag("octave", octave - 1)
	if s[1] == '#' {
		wr.ContentTag("alter", 1)
	} else if s[1] == 'b' {
		wr.ContentTag("alter", -1)
	}
}

func mxmlNoteType(note score.SavedNote) (string, bool) {
	switch note.Duration.RatString() {
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
