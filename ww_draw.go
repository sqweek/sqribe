package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/big"
	"time"

	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/score"
	"sqweek.net/sqribe/wave"

	. "sqweek.net/sqribe/core/types"
)

type WaveLayout struct {
	wave image.Rectangle	// rect of the waveform display
	waveRulers image.Rectangle	// waveform + rulers

	beatAxis, timeAxis, mixer, aboveMixer, belowMixer, newStaffB image.Rectangle
	staves map[*score.Staff] image.Rectangle
	mixers map[*score.Staff] *MixerLayout
}

type MixerLayout struct {
	r, sig, minmaxB, muteB, instC, volS image.Rectangle
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

func (rect *WaveLayout) layout(r image.Rectangle, score *score.Score) {
	axish := 20
	infow := 100
	rect.waveRulers = image.Rect(r.Min.X + infow, r.Min.Y, r.Max.X, r.Max.Y)
	rect.wave = image.Rect(r.Min.X + infow, r.Min.Y + axish, r.Max.X, r.Max.Y - axish)
	rect.beatAxis = aboveRect(rect.wave, axish)
	rect.timeAxis = belowRect(rect.wave, axish)
	rect.mixer = leftRect(rect.waveRulers, infow)
	rect.aboveMixer = leftH(topV(box(rect.mixer.Dx(), axish), rect.mixer), rect.mixer)
	rect.belowMixer = leftH(botV(box(rect.mixer.Dx(), axish), rect.mixer), rect.mixer)
	rect.newStaffB = leftH(centerV(box(axish, axish), rect.belowMixer), rect.belowMixer)
	if score != nil && len(score.Staves()) > 0 {
		minimisedH := 18
		nstaves := 0 // counts number of non-minimised staves
		spaceY := rect.wave.Dy()
		staves := score.Staves()
		for staff, _ := range rect.staves {
			// not sure if deletion is ok here, as event loop also references rect.mixers
			//delete(rect.mixers, staff)
			//delete(rect.staves ,staff)
			if !staff.Minimised {
				nstaves++
			} else {
				spaceY -= minimisedH
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
		for i := 0; i < len(staves); i++ {
			var height int
			if staves[i].Minimised {
				height = minimisedH
			} else {
				height = scoreh
			}
			srect := image.Rect(rect.wave.Min.X, ypos, rect.wave.Max.X, ypos + height)
			ypos += height
			rect.staves[staves[i]] = srect
			mixlayout := MixerLayout{}
			rect.mixers[staves[i]] = mixlayout.calc(yspacing, leftRect(srect, infow))
		}
	}
}

// dst.Bounds() is the entire window, r is the area this widget is responsible for
func (ww *WaveWidget) Draw(dst draw.Image, r image.Rectangle) {
	change := ww.renderstate.changed
	ww.renderstate.changed = 0
	if !r.Eq(ww.r) {
		/* our widget size has chaged, relayout & redraw everything */
		change |= EVERYTHING
		ww.renderstate.waveRulers = nil
		ww.r = r
	}
	if change != 0 {
		if change & LAYOUT != 0 {
			ww.rect.layout(r, ww.score)
		}
		ww.renderstate.img = image.NewRGBA(ww.r)
		if ww.renderstate.waveRulers == nil {
			ww.renderstate.waveRulers = image.NewRGBA(ww.rect.waveRulers)
			change |= WAV | BEATS | VIEWPOS
		}
		if change & WAV != 0 {
			ww.drawWave(ww.renderstate.waveRulers, ww.rect.wave)
		}
		if change & BEATS != 0 || change & VIEWPOS != 0 {
			ww.drawBeatAxis(ww.renderstate.waveRulers, ww.rect.beatAxis)
		}
		if change & VIEWPOS != 0 {
			ww.drawTimeAxis(ww.renderstate.waveRulers, ww.rect.timeAxis)
		}
		draw.Draw(ww.renderstate.img, ww.rect.waveRulers, ww.renderstate.waveRulers, ww.rect.waveRulers.Min, draw.Src)
		ww.drawSelxn(ww.renderstate.img, ww.rect.wave)
		ww.drawMixer(ww.renderstate.img, ww.rect.mixer.Dx())
		ww.drawScale(ww.renderstate.img, ww.rect.wave, ww.rect.mixer.Dx())

		curcol := color.RGBA{0, 0xdd, 0, 255}
		draw.Draw(ww.renderstate.img, image.Rect(ww.cursorX, r.Min.Y, ww.cursorX+1, r.Max.Y), &image.Uniform{curcol}, r.Min, draw.Src)
	}
	draw.Draw(dst, r, ww.renderstate.img, r.Min, draw.Src)
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

func (ww *WaveWidget) drawWave(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xee, 0xee, 0xcc, 255}
	cl := color.RGBA{0x99, 0x99, 0xcc, 255}
	ci := color.RGBA{0xbb, 0x99, 0xbb, 255}
	cr := color.RGBA{0xbb, 0x99, 0x99, 255}
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	if ww.wav == nil {
		return
	}
	f0 := ww.first_frame
	fpp := FrameN(ww.frames_per_pixel)
	f0_get, dx0 := f0, 0
	if f0 < 0 {
		f0_get = 0
		dx0 = 1 + int(-f0 / fpp)
	}
	size := r.Size()
	if dx0 >= size.X {
		return
	}
	yorigin := (r.Min.Y + r.Max.Y) / 2
	yscale := (float64(ww.wav.MaxAmp()) / float64(size.Y / 2))
	chunks := ww.wav.GetFrames(f0_get, f0 + FrameN(size.X) * fpp)
	for dx := dx0; dx < size.X; dx++ {
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

func (ww *WaveWidget) drawSelxn(dst draw.Image, r image.Rectangle) {
	csel := color.NRGBA{0xbb, 0xbb, 0xee, 128}
	rng := ww.SelectedTimeRange()
	sel0, selN := rng.MinFrame(), rng.MaxFrame()
	selR := image.Rect(ww.PixelAtFrame(sel0), r.Min.Y, ww.PixelAtFrame(selN), r.Max.Y)
	draw.Draw(dst, selR, &image.Uniform{csel}, image.ZP, draw.Over)
}

func (ww *WaveWidget) drawMixer(dst draw.Image, infow int) {
	if ww.score == nil {
		return
	}
	mixbg := color.RGBA{0x33, 0x22, 0x22, 0xff}
	border := color.RGBA{0x99, 0x88, 0x88, 0xff}
	bg := color.NRGBA{0x55, 0x44, 0x44, 0xff}
	fg := color.RGBA{0xcc, 0xcc, 0xbb, 0xff}
	drawBorders(dst, ww.rect.mixer, mixbg, mixbg)
	for staff, _ := range ww.rect.staves {
		ww.drawStaffCtl(dst, staff)
	}
	drawBorders(dst, ww.rect.newStaffB, border, bg)
	G.font.luxi.DrawC(dst, fg, ww.rect.newStaffB, "+", centerPt(ww.rect.newStaffB))
}

func (ww *WaveWidget) drawScale(dst draw.Image, r image.Rectangle, infow int) {
	if ww.score == nil {
		return
	}
	black4 := color.RGBA{0x00, 0x00, 0x00, 0x88}
	black1 := color.RGBA{0x00, 0x00, 0x00, 0x22}
	lastFrame := ww.VisibleFrameRange().MaxFrame()
	minX, maxX := -1, -1
	/* XXX doesn't need whole beats array, see drawBeatAxis() */
	for i, beat := range(ww.score.Beats()) {
		if beat.Frame() < ww.first_frame {
			minX = r.Min.X
			continue
		}
		if beat.Frame() > lastFrame {
			maxX = r.Max.X
			break
		}
		x := ww.PixelAtFrame(beat.Frame())
		if minX == -1 {
			minX = x
		}
		maxX = x
		line := image.Rect(x, r.Min.Y, x+1, r.Max.Y)
		black := black1
		if i % 4 == 0 {
			black = black4
		}
		draw.Draw(dst, image.Rect(x-3, r.Min.Y, x+4, r.Min.Y+1), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, image.Rect(x-2, r.Min.Y+1, x+3, r.Min.Y+2), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, image.Rect(x-1, r.Min.Y+2, x+2, r.Min.Y+3), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Over)
	}
	if minX >= maxX {
		return
	}
	for _, staff := range ww.score.Staves() {
		if staff.Minimised {
			continue
		}
		rect := ww.rect.staves[staff]
		mid := rect.Min.Y + rect.Dy() / 2
		drawStaffLines(dst, black4, minX, maxX, mid)

		ww.drawNotes(dst, r, staff, mid)

		ww.drawProspectiveNote(dst, r, staff, mid)
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

func (ww *WaveWidget) drawStaffCtl(dst draw.Image, staff *score.Staff) {
	layout := ww.rect.mixers[staff]
	r := layout.r
	border := color.RGBA{0x99, 0x88, 0x88, 0xff}
	bg := color.NRGBA{0x55, 0x44, 0x44, 0xff}
	fg := color.NRGBA{0xcc, 0xcc, 0xbb, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	black := color.RGBA{0, 0, 0, 0xff}
	drawBorders(dst, r, border, bg)

	drawBorders(dst, layout.minmaxB, fg, bg)
	if staff.Minimised {
		G.font.luxi.DrawC(dst, fg, layout.minmaxB, "+", centerPt(layout.minmaxB))
	} else {
		G.font.luxi.DrawC(dst, fg, layout.minmaxB, "-", centerPt(layout.minmaxB))
	}
	drawBorders(dst, layout.instC, border, white)
	instName := midi.InstName(staff.Voice())
	G.font.luxi.DrawC(dst, black, layout.instC, instName, centerPt(layout.instC))

	var fill color.NRGBA
	if staff.Muted {
		fill = bg
	} else {
		fill = fg
	}
	drawBorders(dst, layout.muteB, border, fill)
	if staff.Minimised {
		return
	}

	mid := r.Min.Y + r.Dy() / 2
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
	drawVertSlider(dst, layout.volS, bg, fg, float64(staff.Velocity()) / 127.0)
}

type DisplayNote struct {
	delta int
	accidental *int
	col color.NRGBA
	duration float64
	beatf float64
	downBeam bool
}

func (ww *WaveWidget) dispNote(staff *score.Staff, note *score.Note) *DisplayNote {
	dn := DisplayNote{}
	dn.beatf = ww.score.Beatf(note)
	dn.duration = note.Durf()
	dn.delta, dn.accidental = staff.LineForPitch(note.Pitch)
	dn.downBeam = (dn.delta > 2)
	return &dn
}

func (ww *WaveWidget) drawNote(dst draw.Image, r image.Rectangle, mid int, n *DisplayNote) {
	black := color.NRGBA{0, 0, 0, n.col.A}
	rng:= ww.VisibleFrameRange()
	frame, _ := ww.score.ToFrame(n.beatf)
	if frame < rng.MinFrame() || frame > rng.MaxFrame() {
		return
	}

	x := ww.PixelAtFrame(frame)
	y := mid - (yspacing / 2) * n.delta

	/* ledger lines */
	ydist := int(math.Abs(float64(y - mid)))
	sgn := 1
	if (mid > y) {
		sgn = -1
	}
	for dy := yspacing * 3; dy <= ydist; dy += yspacing {
		width := yspacing / 2 + 1
		line := image.Rect(x - width, mid + sgn*dy, x + width + 1, mid + sgn*(dy + 1))
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Over)
	}

	var head *NoteHead
	if n.duration < 2 {
		head = newNoteHead(n.col, image.Point{x, y}, yspacing/2, 35.0)
	} else {
		head = newHollowNote(n.col, image.Point{x, y}, yspacing/2, 35.0)
	}
	draw.Draw(dst, r, head, r.Min, draw.Over)
	if n.duration <= 3 {
		var beamEnd image.Point
		var beam image.Rectangle
		if n.downBeam {
			beamEnd = image.Pt(x - yspacing/2, y + yspacing * 2.5)
			beam = image.Rectangle{image.Pt(x - yspacing/2, y), beamEnd.Add(image.Pt(1,1))}
		} else {
			beamEnd = image.Pt(x + yspacing/2 - 1, y - yspacing * 2.5 - 1)
			beam = image.Rectangle{beamEnd, image.Pt(x + yspacing/2, y)}
		}
		draw.Draw(dst, beam, &image.Uniform{n.col}, r.Min, draw.Over)
		i := 0
		for d := 0.5; d >= n.duration; d /= 2 {
			var c image.Point
			if n.downBeam {
				c = image.Pt(beamEnd.X, beamEnd.Y - i * 3)
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
		draw.Draw(dst, r, &DotGlyph{CenteredGlyph{n.col, image.Point{x + yspacing/2 + 3 + 5*i, y}, yspacing/2}}, r.Min, draw.Over)
	}
	if n.accidental != nil {
		draw.Draw(dst, r, newAccidental(n.col, image.Point{x - yspacing, y}, yspacing/2, *n.accidental), r.Min, draw.Over)
	}
}


func (ww *WaveWidget) drawNotes(dst draw.Image, r image.Rectangle, staff *score.Staff, mid int) {
	next := score.Chords(ww.score.Iter(ww.VisibleFrameRange(), staff))
	var chord []score.StaffNote
	for next != nil {
		chord, next = next()
		downBeam := true
		notes := make([]*DisplayNote, len(chord))
		for i, sn := range chord {
			notes[i] = ww.dispNote(staff, sn.Note)
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
			} else {
				note.col = color.NRGBA{0, 0, 0, 0xff}
			}
			note.downBeam = downBeam
			ww.drawNote(dst, r, mid, note)
		}
	}
}

func (ww *WaveWidget) drawProspectiveNote(dst draw.Image, r image.Rectangle, staff *score.Staff, mid int) {
	s := ww.getMouseState(ww.mouse.pos)
	if s.ndelta != nil {
		for _, sn := range ww.SelectedNotes() {
			if sn.Staff != staff {
				continue
			}
			n := sn.Note.Dup()
			n.Pitch += uint8(s.ndelta.Δpitch)
			beat, offset := ww.score.Quantize(ww.score.Beatf(sn.Note) + s.ndelta.Δbeat)
			n.Beat = beat
			n.Offset.Set(offset)
			note := ww.dispNote(sn.Staff, n)
			note.col = color.NRGBA{0x88, 0x88, 0x88, 0xaa}
			ww.drawNote(dst, r, mid, note)
		}
	} else if s.note != nil && s.note.staff == staff {
		str, _ := G.noteMenu.options[G.noteMenu.lastSelected].(string)
		var dur big.Rat
		dur.SetString(str)
		n := ww.mkNote(s.note, &dur)
		note := ww.dispNote(staff, n)
		// could avoid transitioning through staff.Note except for NoteAt
		existingNote := staff.NoteAt(n)
		if existingNote == nil {
			note.col = colourFor(n.Offset, 0xbb)
		} else {
			if _, selected := ww.notesel[existingNote]; selected {
				note.col = color.NRGBA{0x88, 0x88, 0x88, 0x88}
			} else {
				note.col = color.NRGBA{0, 0, 0, 0x88}
			}
		}
		ww.drawNote(dst, r, mid, note)
	}
}

func (ww *WaveWidget) drawTicks(dst draw.Image, r image.Rectangle, bottom bool, vals []float64, aToX func(float64)int, label func(float64)string) {
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
	for i, a := range vals {
		x := aToX(a)
		if x >= r.Min.X && x < r.Max.X && x >= lastMajX + targetPixPerTick {
			lastMajX = x
			draw.Draw(dst, tickRect(r, bottom, x, 7), &image.Uniform{fg}, image.ZP, draw.Over)
			if x > lastTextX + textSpacing {
				lastTextX = x
				G.font.luxi.DrawC(dst, fg, r, label(a), image.Point{x, textY})
			}
		}
		if i + 1 == len(vals) || x >= r.Max.X {
			break
		}
		/* minor ticks */
		dx := aToX(vals[i+1]) - x
		if dx > targetPixPerTick {
			da := vals[i+1] - a
			nminor := int((dx) / targetPixPerTick) - 1
			for i := 1; i <= nminor; i++ {
				am := a + float64(i) * (da / float64(nminor + 1))
				x := aToX(am)
				if x >= r.Min.X && x < r.Max.X {
					draw.Draw(dst, tickRect(r, bottom, x, 4), &image.Uniform{fg}, image.ZP, draw.Over)
				}
			}
		}
	}
}

func (ww *WaveWidget) drawBeatAxis(dst draw.Image, r image.Rectangle) {
	score := ww.score
	beatToX := func(beatf float64) int {
		frame, ok := score.ToFrame(beatf)
		if !ok {
			return r.Max.X + r.Dx()
		}
		return ww.PixelAtFrame(frame)
	}
	label := func(beatf float64) string {
		return fmt.Sprintf("b%d", int(beatf) + 1)
	}
	beats := make([]float64, 0)
	if score != nil && len(score.Beats()) > 0 {
		b0 := score.BeatIndex(score.NearestBeat(ww.FrameAtPixel(r.Min.X)))
		bN := score.BeatIndex(score.NearestBeat(ww.FrameAtPixel(r.Max.X)))
		b0 = score.ClipBeat(b0 - 1)
		bN = score.ClipBeat(bN + 1)
		for b := b0; b <= bN; b++ {
			beats = append(beats, float64(b))
		}
	}
	ww.drawTicks(dst, r, true, beats, beatToX, label)
}


func (ww *WaveWidget) drawTimeAxis(dst draw.Image, r image.Rectangle) {
	wav := ww.wav
	tToX := func(t float64) int {
		return ww.PixelAtFrame(wav.FrameAtTime(time.Duration(t * 1000.0) * time.Millisecond))
	}
	label := func(t float64) string {
		dur := time.Duration(t) * time.Second
		return fmt.Sprintf("%02d:%02d", int(dur.Minutes()), int(dur.Seconds()) % 60)
	}
	times := make([]float64, 0)
	if wav != nil {
		t0 := math.Trunc(ww.TimeAtCursor(r.Min.X).Seconds())
		if t0 < 0 {
			t0 = 0.0
		}
		tN := math.Ceil(ww.TimeAtCursor(r.Max.X).Seconds())
		for t := t0; t <= tN; t += 1.0 {
			times = append(times, t)
		}
	}
	ww.drawTicks(dst, r, false, times, tToX, label)
}
