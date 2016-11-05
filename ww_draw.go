package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/skelterjohn/go.wde"

	"github.com/sqweek/sqribe/midi"
	"github.com/sqweek/sqribe/score"
	"github.com/sqweek/sqribe/wave"

	. "github.com/sqweek/sqribe/core/types"
)

type DisplayNote struct {
	delta int
	accidental *int
	col color.NRGBA
	duration float64
	downBeam bool
	pt *image.Point // centre of note head. nil if not visible
}

type WaveLayout struct {
	image.Rectangle	// rect of entire widget
	wave image.Rectangle	// rect of the waveform display
	waveRulers image.Rectangle	// waveform + rulers

	beatAxis, timeAxis, mixRulers, mixer, aboveMixer, belowMixer, newStaffB image.Rectangle
	staff atomic.Value //holds map[*score.Staff]*StaffLayout
}

func (w *WaveLayout) staves() map[*score.Staff]*StaffLayout {
	return w.staff.Load().(map[*score.Staff]*StaffLayout)
}

type StaffLayout struct {
	r image.Rectangle // rect of staff's waveform overlay
	mix MixerLayout
}

func (slayout StaffLayout) Mid() int {
	return slayout.r.Min.Y + slayout.r.Dy() / 2
}

type MixerLayout struct {
	r, sig, minmaxB, muteB, instC, volS image.Rectangle
	Minimised bool
}

func (layout *MixerLayout) calc(yspacing int, r image.Rectangle) *MixerLayout {
	layout.r = r

	sigW := 8*(yspacing/2)
	layout.sig = rightH(centerV(box(sigW, 7*yspacing), r), r)

	button := box(12, 12) // button size
	layout.instC = topV(box(r.Dx() - 2, 18), r).Add(image.Point{1, 1})
	layout.minmaxB = leftH(centerV(button, layout.instC), r).Add(image.Point{1, 0})
	layout.muteB = rightRect(layout.minmaxB, button.Dx())
	layout.instC.Min.X = layout.muteB.Max.X

	layout.volS = leftH(box(12, r.Dy()), r).Add(image.Point{1, 0})
	layout.volS.Min.Y = layout.instC.Max.Y
	layout.volS.Max.Y = r.Max.Y - 2

	return layout
}

func (rect *WaveLayout) layout(r image.Rectangle, sc *score.Score) {
	rect.Rectangle = r
	axish := 20
	infow := 100
	rect.waveRulers = image.Rect(r.Min.X + infow, r.Min.Y, r.Max.X, r.Max.Y)
	rect.wave = image.Rect(r.Min.X + infow, r.Min.Y + axish, r.Max.X, r.Max.Y - axish)
	rect.beatAxis = aboveRect(rect.wave, axish)
	rect.timeAxis = belowRect(rect.wave, axish)
	rect.mixRulers = leftRect(rect.waveRulers, infow)
	rect.mixer = leftRect(rect.wave, infow)
	rect.aboveMixer = aboveRect(rect.mixer, axish)
	rect.belowMixer = belowRect(rect.mixer, axish)
	rect.newStaffB = leftH(centerV(box(axish, axish), rect.belowMixer), rect.belowMixer)
	staffR := make(map[*score.Staff]*StaffLayout, len(sc.Staves()))
	if sc != nil && len(sc.Staves()) > 0 {
		minimisedH := 18
		nstaves := 0 // counts number of non-minimised staves
		spaceY := rect.wave.Dy()
		staves := sc.Staves()
		prev := rect.staff.Load().(map[*score.Staff]*StaffLayout) // previous layouts
		for _, staff := range staves {
			layout := prev[staff]
			if layout != nil && layout.mix.Minimised {
				spaceY -= minimisedH
			} else {
				nstaves++
			}
			if layout != nil {
				staffR[staff] = layout // so we remember Minimised state
			}
		}
		scoreh := 0
		if nstaves > 0 {
			scoreh = spaceY / nstaves
		}
		minh := yspacing * 8
		if scoreh < minh {
			scoreh = minh
		}
		ypos := rect.wave.Min.Y
		for _, staff := range staves {
			var height int
			slayout, ok := staffR[staff]
			if !ok {
				slayout = new(StaffLayout)
				staffR[staff] = slayout
			}
			if slayout.mix.Minimised {
				height = minimisedH
			} else {
				height = scoreh
			}
			slayout.r = image.Rect(rect.wave.Min.X, ypos, rect.wave.Max.X, ypos + height)
			slayout.mix.calc(yspacing, leftRect(slayout.r, infow))
			ypos += height
		}
	}
	rect.staff.Store(staffR)
}

func (ww *WaveWidget) Rect() image.Rectangle {
	return ww.rect.Rectangle
}

// screen.Bounds() is the entire window, r is the area this widget is responsible for
func (ww *WaveWidget) Draw(screen wde.Image, r image.Rectangle) {
	change := ww.renderstate.changed
	ww.renderstate.changed = 0
	if !ww.rect.Eq(r) {
		/* our widget size has chaged, relayout & redraw everything */
		change |= EVERYTHING
		ww.renderstate.waveRulers = nil
		ww.renderstate.img = nil
		ww.renderstate.cursor = nil
		ww.Zoom(1.0) // to prevent wider resize blowing wave cache
	}
	if change != 0 {
		pos := ww.pos
		if change & (LAYOUT | RESET) != 0 {
			ww.rect.layout(r, ww.score)
		}
		if ww.renderstate.cursor == nil {
			curcol := color.RGBA{0, 0xdd, 0, 255}
			img := image.NewRGBA(vrect(r, 0))
			draw.Draw(img, img.Rect, &image.Uniform{curcol}, image.ZP, draw.Src)
			ww.renderstate.cursor = img
		}
		if ww.renderstate.img == nil {
			ww.renderstate.img = image.NewRGBA(ww.Rect())
			change |= EVERYTHING
		}
		if ww.renderstate.waveRulers == nil {
			ww.renderstate.waveRulers = image.NewRGBA(ww.rect.waveRulers)
			change |= WAV | BEATS | VIEWPOS
		}
		if change & WAV != 0 {
			ww.drawWave(ww.renderstate.waveRulers, ww.rect.wave, &pos)
		}
		if change & (BEATS | VIEWPOS | SELXN) != 0 {
			ww.drawBeatAxis(ww.renderstate.waveRulers, ww.rect.beatAxis, &pos)
		}
		if change & (VIEWPOS | SELXN) != 0 {
			ww.drawTimeAxis(ww.renderstate.waveRulers, ww.rect.timeAxis, &pos)
		}
		switch {
		case change & (WAV | BEATS | VIEWPOS) != 0:
			change |= SELXN | SCALE | CURSOR
			fallthrough
		case change & (SELXN | SCALE) != 0:
			draw.Draw(ww.renderstate.img, ww.rect.waveRulers, ww.renderstate.waveRulers, ww.rect.waveRulers.Min, draw.Src)
		}
		if change & (SELXN | SCALE | BEATS) != 0 {
			ww.drawSelxn(ww.renderstate.img, ww.rect.waveRulers, &pos)
			ww.drawScale(ww.renderstate.img, ww.rect.wave, ww.rect.mixer.Dx(), &pos)
			img := ww.renderstate.img.SubImage(ww.rect.waveRulers).(*image.RGBA)
			screen.CopyRGBA(img, ww.rect.waveRulers)
			ww.drawCursor(screen, r, ww.cursorX, false)
		} else if change & CURSOR != 0 {
			ww.drawCursor(screen, r, ww.cursorX, true)
		}

		if change & (MIXER | LAYOUT | RESET) != 0 {
			ww.drawMixer(ww.renderstate.img)
			img := ww.renderstate.img.SubImage(ww.rect.mixRulers).(*image.RGBA)
			screen.CopyRGBA(img, ww.rect.mixRulers)
		}
	}
}

func (ww *WaveWidget) drawCursor(screen wde.Image, r image.Rectangle, x int, restore bool) {
	if restore {
			prevR := vrect(r, ww.renderstate.cursorPrevX)
			screen.CopyRGBA(ww.renderstate.img.SubImage(prevR).(*image.RGBA), prevR)
	}
	if x < ww.rect.waveRulers.Min.X || x >= ww.rect.waveRulers.Max.X {
		return
	}
	ww.renderstate.cursor.Rect.Min.X = x
	ww.renderstate.cursor.Rect.Max.X = x+1
	screen.CopyRGBA(ww.renderstate.cursor, ww.renderstate.cursor.Rect)
	ww.renderstate.cursorPrevX = x
}

func colourFor(offset *big.Rat, α uint8) color.NRGBA {
	switch (offset.RatString()) {
	case "1": fallthrough
	case "0": fallthrough
	case "1/2": return color.NRGBA{0xff, 0x00, 0x00, α}

	case "1/4": fallthrough
	case "3/4": return color.NRGBA{0x00, 0x00, 0xff, α}

	case "1/8": fallthrough
	case "3/8": fallthrough
	case "5/8": fallthrough
	case "7/8": return color.NRGBA{0xff, 0xff, 0x00, α}

	// this is a bit too close to the blue right next door
	case "1/6": fallthrough
	case "3/6": fallthrough
	case "5/6": return color.NRGBA{0xaa, 0x00, 0x88, α}

	case "1/3": fallthrough
	case "2/3": return color.NRGBA{0xff, 0x00, 0xff, α}
	}
	return color.NRGBA{0x00, 0x00, 0x00, α}
}

func scale(chMin, chMax int16, yscale float64) (int, int) {
	var min, max int
	if chMin < 0 {
		min = int(float64(chMin) / yscale)
	}
	if chMax > 0 {
		max = int(float64(chMax) / yscale)
	}
	return min, max
}

func (ww *WaveWidget) drawWave(dst draw.Image, r image.Rectangle, pos *FramePos) {
	bg := color.RGBA{0xee, 0xee, 0xcc, 255}
	cl := color.RGBA{0x99, 0x99, 0xcc, 255}
	ci := color.RGBA{0xbb, 0x99, 0xbb, 255}
	cr := color.RGBA{0xbb, 0x99, 0x99, 255}
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	if ww.wav == nil {
		return
	}
	f0 := pos.f0
	fpp := FrameN(pos.ppix)
	f0_get, dx0 := f0, 0
	if f0 < 0 {
		f0_get = 0
		dx0 = 1 + int(-f0 / fpp)
	}
	if dx0 >= r.Dx() {
		return
	}
	halfy := r.Dy() / 2
	yorigin := r.Min.Y + halfy
	yscale := (float64(ww.wav.MaxAmp()) / float64(halfy))
	chunks := ww.wav.GetFrames(f0_get, f0 + FrameN(r.Dx()) * fpp)
	for dx := dx0; dx < r.Dx(); dx++ {
		pixS0, pixSN := ww.wav.SampleRange(f0 + fpp * FrameN(dx), f0 + fpp * FrameN(dx+1))
		pixSamples := wave.Extract(chunks, pixS0, pixSN)
		ext := ww.wav.ChannelExtents(pixSamples)
		/* FIXME remove two channel assumption */
		lmin, lmax := scale(ext[0], ext[1], yscale)
		rmin, rmax := scale(ext[2], ext[3], yscale)
		x := r.Min.X + dx
		rl := image.Rect(x, yorigin - lmax, x + 1, yorigin - lmin + 1)
		rr := image.Rect(x, yorigin - rmax, x + 1, yorigin - rmin + 1)
		ri := rl.Intersect(rr)
		draw.Draw(dst, rl, &image.Uniform{cl}, image.ZP, draw.Src)
		draw.Draw(dst, rr, &image.Uniform{cr}, image.ZP, draw.Src)
		if !ri.Empty() {
			draw.Draw(dst, ri, &image.Uniform{ci}, image.ZP, draw.Src)
		}
	}
}

func (ww *WaveWidget) drawSelxn(dst draw.Image, r image.Rectangle, pos *FramePos) {
	csel := color.NRGBA{0xbb, 0xbb, 0xee, 128}
	rng := ww.SelectedTimeRange()
	sel0, selN := rng.MinFrame(), rng.MaxFrame()
	if sel0 < selN {
		selR := image.Rect(r.Min.X + pos.DxAtFrame(sel0), r.Min.Y, r.Min.X + pos.DxAtFrame(selN) + 1, r.Max.Y)
		draw.Draw(dst, selR, &image.Uniform{csel}, image.ZP, draw.Over)
	}
}

func (ww *WaveWidget) drawMixer(dst draw.Image) {
	mixbg := color.RGBA{0x33, 0x22, 0x22, 0xff}
	border := color.RGBA{0x99, 0x88, 0x88, 0xff}
	bg := color.NRGBA{0x55, 0x44, 0x44, 0xff}
	fg := color.RGBA{0xcc, 0xcc, 0xbb, 0xff}
	drawBorders(dst, ww.rect.mixRulers, mixbg, mixbg)
	if ww.score == nil {
		return
	}
	img := image.NewRGBA(ww.rect.mixer) // hack: new image to make clipping easy
	for staff, slayout := range ww.rect.staves() {
		ww.drawStaffCtl(img, staff, slayout)
	}
	draw.Draw(dst, ww.rect.mixer, img, ww.rect.mixer.Min, draw.Over)
	drawBorders(dst, ww.rect.newStaffB, border, bg)
	G.font.luxi.DrawC(dst, fg, ww.rect.newStaffB, "+", centerPt(ww.rect.newStaffB))
}

func (ww *WaveWidget) drawScale(dst draw.Image, r image.Rectangle, infow int, pos *FramePos) {
	if ww.score == nil || !ww.score.HasBeats() {
		return
	}
	black4 := color.RGBA{0x00, 0x00, 0x00, 0x88}
	black1 := color.RGBA{0x00, 0x00, 0x00, 0x22}
	lastFrame := pos.FrameAtDx(r.Dx())
	minX, maxX := -1, -1
	b0 := ww.score.NearestBeat(pos.f0).LPrev()
	i := b0.BeatNum() - 1
	for beat := b0; beat != nil; beat = beat.Next() {
		if beat.Frame() < pos.f0 {
			minX = r.Min.X
			i++
			continue
		}
		if beat.Frame() > lastFrame {
			maxX = r.Max.X
			break
		}
		x := r.Min.X + pos.DxAtFrame(ww.beatFrame(beat))
		if minX == -1 {
			minX = x
		}
		maxX = x
		line := image.Rect(x, r.Min.Y, x+1, r.Max.Y)
		black := black1
		if i % 4 == 0 {
			black = black4
		}
		i++
		draw.Draw(dst, image.Rect(x-3, r.Min.Y, x+4, r.Min.Y+1), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, image.Rect(x-2, r.Min.Y+1, x+3, r.Min.Y+2), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, image.Rect(x-1, r.Min.Y+2, x+2, r.Min.Y+3), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Over)
	}
	if minX >= maxX || minX == -1 || maxX == -1 {
		return
	}
	selRect := ww.getMouseState(ww.mouse.pos).rectSelect
	for staff, slayout := range ww.rect.staves() {
		if slayout.mix.Minimised {
			continue
		}
		mid := slayout.Mid()
		drawStaffLines(dst, black4, minX, maxX, mid)

		ww.drawNotes(dst, r, staff, mid, selRect, pos)

		ww.drawProspectiveNote(dst, r, staff, mid, pos)
	}
	if selRect != nil {
		drawBorders(dst, selRect.Intersect(ww.rect.waveRulers), color.NRGBA{0xff,0xff,0xff,0x88}, color.NRGBA{0xff,0xff,0xff,0x44})
	}
}

func drawStaffLines(dst draw.Image, col color.Color, minX, maxX, mid int) {
	minY, maxY := mid - 2 * yspacing, mid + 2 * yspacing
	for y := minY; y <= maxY; y += yspacing {
		line := image.Rect(minX, y, maxX, y+1)
		draw.Draw(dst, line, &image.Uniform{col}, image.ZP, draw.Over)
	}
}

func drawBorders(dst draw.Image, r image.Rectangle, border color.Color, fill color.Color) {
	top := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y + 1)
	left := image.Rect(r.Min.X, r.Min.Y, r.Min.X + 1, r.Max.Y)
	bot := image.Rect(r.Min.X, r.Max.Y - 1, r.Max.X, r.Max.Y)
	right := image.Rect(r.Max.X - 1, r.Min.Y, r.Max.X, r.Max.Y)
	draw.Draw(dst, r, &image.Uniform{fill}, image.ZP, draw.Over)
	for _, line := range []image.Rectangle{top, left, bot, right} {
		draw.Draw(dst, line, &image.Uniform{border}, image.ZP, draw.Over)
	}
}

func (ww *WaveWidget) drawStaffCtl(dst draw.Image, staff *score.Staff, slayout *StaffLayout) {
	mix := Mixer.For(staff)
	layout := slayout.mix
	r := layout.r
	border := color.RGBA{0x99, 0x88, 0x88, 0xff}
	bg := color.NRGBA{0x55, 0x44, 0x44, 0xff}
	fg := color.NRGBA{0xcc, 0xcc, 0xbb, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	black := color.RGBA{0, 0, 0, 0xff}
	drawBorders(dst, r, border, bg)

	drawBorders(dst, layout.minmaxB, fg, bg)
	if layout.Minimised {
		G.font.luxi.DrawC(dst, fg, layout.minmaxB, "+", centerPt(layout.minmaxB))
	} else {
		G.font.luxi.DrawC(dst, fg, layout.minmaxB, "-", centerPt(layout.minmaxB))
	}
	drawBorders(dst, layout.instC, border, white)
	instName := midi.InstName(mix.Voice)
	G.font.luxi.DrawC(dst, black, layout.instC, instName, centerPt(layout.instC))

	var fill color.NRGBA
	if mix.Muted {
		fill = bg
	} else {
		fill = fg
	}
	drawBorders(dst, layout.muteB, border, fill)
	if layout.Minimised {
		return
	}

	mid := slayout.Mid()
	drawStaffLines(dst, fg, layout.sig.Min.X, layout.sig.Max.X, mid)
	keysig, lines := staff.KeyAccidentalLines()
	for i, delta := range lines {
		cg := CenteredGlyph{
			col: fg,
			p: image.Point{layout.sig.Min.X + (i + 1) * (yspacing/2), mid - delta * yspacing/2},
			r: yspacing/2,
		}
		var glyph image.Image
		if keysig.IsSharps() {
			glyph = &SharpGlyph{cg}
		} else {
			glyph = &FlatGlyph{cg}
		}
		draw.Draw(dst, r, glyph, r.Min, draw.Over)
	}

//	restR := image.Rectangle{r.Min, image.Point{sigR.Min.X, r.Max.Y}}.Inset(1)
//	drawBorders(dst, restR, border, bg)
	drawVertSlider(dst, layout.volS, fg, float64(mix.Velocity) / 127.0)
}

func (ww *WaveWidget) noteX(note *score.Note, pos *FramePos) int {
	return ww.rect.wave.Min.X + pos.DxAtFrame(ww.ToFrame(ww.score.Beatf(note)))
}

func (ww *WaveWidget) noteY(staff *score.Staff, note *score.Note, mid int) int {
	delta, _ := staff.LineForPitch(note.Pitch)
	return mid - delta * (yspacing/2)
}

func (ww *WaveWidget) notePt(staff *score.Staff, note *score.Note, mid int, pos *FramePos) image.Point {
	return image.Point{ww.noteX(note, pos), ww.noteY(staff, note, mid)}
}

func (ww *WaveWidget) dispNote(staff *score.Staff, note *score.Note, mid int, pos *FramePos) *DisplayNote {
	dn := DisplayNote{}
	dn.duration = note.Durf()
	dn.delta, dn.accidental = staff.LineForPitch(note.Pitch)
	dn.downBeam = (dn.delta > 2)
	r := ww.rect.wave
	rng := pos.Range(r.Dx())
	frame := ww.ToFrame(ww.score.Beatf(note))
	if frame >= rng.MinFrame() && frame <= rng.MaxFrame() {
		pt := ww.notePt(staff, note, mid, pos)
		dn.pt = &pt
	}
	return &dn
}

func (ww *WaveWidget) drawNote(dst draw.Image, r image.Rectangle, mid int, n *DisplayNote) {
	if n.pt == nil {
		return
	}
	black := color.NRGBA{0, 0, 0, n.col.A}

	/* ledger lines */
	ydist := int(math.Abs(float64(n.pt.Y - mid)))
	sgn := 1
	if (mid > n.pt.Y) {
		sgn = -1
	}
	for dy := yspacing * 3; dy <= ydist; dy += yspacing {
		width := yspacing / 2 + 1
		line := image.Rect(n.pt.X - width, mid + sgn*dy, n.pt.X + width + 1, mid + sgn*(dy + 1))
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Over)
	}

	var head *NoteHead
	if n.duration < 2 {
		head = newNoteHead(n.col, *n.pt, yspacing/2, 35.0)
	} else {
		head = newHollowNote(n.col, *n.pt, yspacing/2, 35.0)
	}
	draw.Draw(dst, r, head, r.Min, draw.Over)
	if n.duration <= 3 {
		var beamEnd image.Point
		var beam image.Rectangle
		if n.downBeam {
			beamEnd = n.pt.Add(image.Pt(-yspacing/2 + 1, yspacing * 2.5 + 1))
			beam = image.Rectangle{n.pt.Add(image.Pt(-yspacing/2, 0)), beamEnd}
		} else {
			beamEnd = n.pt.Add(image.Pt(yspacing/2 - 1, -yspacing * 2.5 - 1))
			beam = image.Rectangle{beamEnd, n.pt.Add(image.Pt(yspacing/2, 0))}
		}
		draw.Draw(dst, beam, &image.Uniform{n.col}, r.Min, draw.Over)
		i := 0
		for d := 0.5; d >= n.duration; d /= 2 {
			var c image.Point
			if n.downBeam {
				c = image.Pt(beamEnd.X - 1, beamEnd.Y - i * 3 - 1)
			} else {
				c = image.Pt(beamEnd.X, beamEnd.Y + i * 3)
			}
			i++
			draw.Draw(dst, r, &NoteTail{CenteredGlyph{n.col, c, 4*yspacing/(2*5)}, n.downBeam}, r.Min, draw.Over)
		}
	}
	/* TODO triplets */
	dotted := 0
	for d := 2.0; d >= 1./128; d/=2 {
		switch {
		case math.Abs(d * 1.5 - n.duration) < 1e-6:
			dotted = 1
			break
		case  math.Abs(d - n.duration) < 1e-6:
			break
		}
	}
	for i := 0; i < dotted; i++ {
		draw.Draw(dst, r, &DotGlyph{CenteredGlyph{n.col, n.pt.Add(image.Pt(yspacing/2 + 3 + 5*i, 0)), yspacing/2}}, r.Min, draw.Over)
	}
	if n.accidental != nil {
		draw.Draw(dst, r, newAccidental(n.col, n.pt.Sub(image.Pt(yspacing, 0)), yspacing/2, *n.accidental), r.Min, draw.Over)
	}
}


func (ww *WaveWidget) drawNotes(dst draw.Image, r image.Rectangle, staff *score.Staff, mid int, selRect *image.Rectangle, pos *FramePos) {
	next := score.Chords(ww.score.Iter(pos.Range(r.Dx()), staff))
	var chord []score.StaffNote
	for next != nil {
		chord, next = next()
		downBeam := true
		notes := make([]*DisplayNote, len(chord))
		for i, sn := range chord {
			notes[i] = ww.dispNote(staff, sn.Note, mid, pos)
			downBeam = downBeam && (notes[i].delta > 2)
		}
		for i, note := range notes {
			_, selected := ww.notesel[chord[i].Note]
			if selected {
				α := uint8(0xff)
				state := ww.getMouseState(ww.mouse.pos)
				if state.ndelta != nil {
					α = 0x88
				}
				note.col = color.NRGBA{0x88, 0x88, 0x88, α}
			} else if note.pt != nil && selRect != nil && note.pt.In(*selRect) {
				note.col = color.NRGBA{0x66, 0x66, 0xaa, 0xff}
			} else {
				note.col = color.NRGBA{0, 0, 0, 0xff}
			}
			note.downBeam = downBeam
			ww.drawNote(dst, r, mid, note)
		}
	}
}

func (ww *WaveWidget) drawProspectiveNote(dst draw.Image, r image.Rectangle, staff *score.Staff, mid int, pos *FramePos) {
	s := ww.getMouseState(ww.mouse.pos)
	if s.rectSelect != nil {
		return // dragging to select notes
	}
	if s.ndelta != nil {
		for _, sn := range ww.SelectedNotes() {
			if sn.Staff != staff {
				continue
			}
			note := sn.Note.Dup().Mv(s.ndelta.Δpitch, s.ndelta.Δbeat)
			n := ww.dispNote(sn.Staff, note, mid, pos)
			n.col = color.NRGBA{0x88, 0x88, 0x88, 0xaa}
			ww.drawNote(dst, r, mid, n)
		}
	} else if s.note != nil && ww.pasteMode && len(ww.snarf[staff]) > 0 && len(ww.snarf[s.note.staff]) > 0 {
		sc := ww.score
		anchor := ww.snarf[s.note.staff][0]
		Δpitch := s.note.Δpitch(anchor)
		beat, offset := sc.Quantize(s.note.beatf)
		Δbeat := Δb(beat, offset, anchor.Beat, anchor.Offset)
		for _, note := range ww.snarf[staff] {
			n := ww.dispNote(staff, note.Dup().Mv(Δpitch, Δbeat), mid, pos)
			n.col = color.NRGBA{0x88, 0x88, 0x88, 0xaa}
			ww.drawNote(dst, r, mid, n)
		}
	} else if s.note != nil && s.note.staff == staff {
		menu, _ := G.noteMenu.options[G.noteMenu.lastSelected].(string)
		var dur big.Rat
		dur.SetString(menu)
		note, exists := s.note.mkNote(ww.score, &dur)
		dn := ww.dispNote(staff, note, mid, pos)
		if !exists {
			dn.col = colourFor(note.Offset, 0xbb)
		} else {
			dn.duration, _ = dur.Float64()
			if _, selected := ww.notesel[note]; selected {
				dn.col = color.NRGBA{0x88, 0x88, 0x88, 0x88}
			} else {
				dn.col = color.NRGBA{0, 0, 0, 0x88}
			}
		}
		ww.drawNote(dst, r, mid, dn)
	}
}

func (ww *WaveWidget) drawTicks(dst draw.Image, r image.Rectangle, bottom bool, vals []float64, frames []FrameN, label func(float64)string, pos *FramePos) {
	targetPixPerTick := 30
	bg := color.RGBA{0x55, 0x44, 0x44, 0xff}
	fg := color.RGBA{0xcc, 0xcc, 0xbb, 0xff}
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Over)
	lastMajX := r.Min.X - targetPixPerTick
	textSpacing := 50
	textY := r.Min.Y + 14
	if bottom {
		textY = r.Max.Y - 14
	}
	lastTextX := r.Min.X - textSpacing
	xs := make([]int, 0, len(frames))
	for _, f := range frames {
		xs = append(xs, r.Min.X + pos.DxAtFrame(f))
	}
	for i, a := range vals {
		last := (i + 1 == len(vals))
		x := xs[i]
		if (x >= r.Min.X && x < r.Max.X) && (x >= lastMajX + targetPixPerTick || last) {
			lastMajX = x
			draw.Draw(dst, tickRect(r, bottom, x, 7), &image.Uniform{fg}, image.ZP, draw.Over)
			if x > lastTextX + textSpacing {
				lastTextX = x
				G.font.luxi.DrawC(dst, fg, r, label(a), image.Point{x, textY})
			}
		}
		if last || x >= r.Max.X {
			break
		}
		/* minor ticks */
		dx := xs[i+1] - x
		if dx > targetPixPerTick {
			nminor := int((dx) / targetPixPerTick) - 1
			for i := 1; i <= nminor; i++ {
				xm := x + int(float64(dx) * (float64(i) / float64(nminor + 1)))
				if xm >= r.Min.X && xm < r.Max.X {
					draw.Draw(dst, tickRect(r, bottom, xm, 4), &image.Uniform{fg}, image.ZP, draw.Over)
				}
			}
		}
	}
}

func (ww *WaveWidget) drawBeatAxis(dst draw.Image, r image.Rectangle, pos *FramePos) {
	score := ww.score
	label := func(beatf float64) string {
		return fmt.Sprintf("b%d", int(beatf))
	}
	beats := make([]float64, 0)
	frames := make([]FrameN, 0)
	if score != nil && score.HasBeats() {
		b0 := score.NearestBeat(pos.FrameAtDx(0)).LPrev()
		// XXX should start search from b0
		bN := score.NearestBeat(pos.FrameAtDx(r.Dx())).LNext()
		i := b0.BeatNum()
		for b := b0; b != nil && ww.beatFrame(b) <= ww.beatFrame(bN); b = b.Next() {
			beats = append(beats, float64(i))
			frames = append(frames, ww.beatFrame(b))
			i++
		}
	}
	ww.drawTicks(dst, r, true, beats, frames, label, pos)
}


func (ww *WaveWidget) drawTimeAxis(dst draw.Image, r image.Rectangle, pos *FramePos) {
	wav := ww.wav
	label := func(t float64) string {
		dur := time.Duration(t) * time.Second
		return fmt.Sprintf("%02d:%02d", int(dur.Minutes()), int(dur.Seconds()) % 60)
	}
	times := make([]float64, 0)
	frames := make([]FrameN, 0)
	if wav != nil {
		t0 := math.Trunc(wav.TimeAtFrame(pos.f0).Seconds())
		if t0 < 0 {
			t0 = 0.0
		}
		fRight := wav.Clip(pos.FrameAtDx(r.Max.X), 0)
		tRight := wav.TimeAtFrame(fRight)
		tN := math.Ceil(tRight.Seconds())
		for t := t0; t <= tN; t += 1.0 {
			times = append(times, t)
			frames = append(frames, wav.FrameAtTime(time.Duration(t * 1000.0) * time.Millisecond))
		}
	}
	ww.drawTicks(dst, r, false, times, frames, label, pos)
}
