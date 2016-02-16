package main

import (
	"image/color"
	"image/draw"
	"image"

	"github.com/skelterjohn/go.wde"

	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/score"
)

type MixVolume struct {
	Gain float64
	Muted bool
}

type MixConfig struct {
	Master, Midi, Wave MixVolume
	MuteMetronome bool
	Staff map[*score.Staff]*StaffMix
	preSolo map[*score.Staff]bool // records Muted status of staves before entering solo
}

type StaffMix struct {
	Voice int
	Velocity int
	Muted bool
}

var Mixer MixConfig

func init() {
	Mixer.Staff = make(map[*score.Staff]*StaffMix)
	Mixer.Master.Gain = 1.0
	Mixer.Midi.Gain = 1.0
	Mixer.Wave.Gain = 1.0
}

func (m *MixConfig) LoadStaff(staff *score.Staff, saved SavedStaff) {
	stm := m.For(staff)
	stm.Voice = saved.Voice
	stm.Velocity = saved.Velocity + 100
	stm.Muted = saved.Muted
}

func (m *MixConfig) For(staff *score.Staff) *StaffMix {
	if sm, ok := m.Staff[staff]; ok {
		return sm
	}
	m.Staff[staff] = &StaffMix{midi.InstPiano, 100, false}
	return m.Staff[staff]
}

func (m *MixConfig) IsSolo(s *score.Staff) bool {
	if m.For(s).Muted {
		return false
	}
	for staff, mix := range m.Staff {
		if staff != s && !mix.Muted {
			return false
		}
	}
	return true
}

func (m *MixConfig) ToggleSolo(s *score.Staff) {
	if m.IsSolo(s) {
		for staff, mix := range m.Staff {
			old := false
			if m.preSolo != nil {
				old, _ = m.preSolo[staff] // old remains false if staff not present in map
			}
			mix.Muted = old
		}
		m.preSolo = nil
	} else {
		m.preSolo = make(map[*score.Staff]bool)
		for staff, mix := range m.Staff {
			m.preSolo[staff] = mix.Muted
			mix.Muted = (staff != s)
		}
	}
}

type VolLayout struct {
	r, icon, slide image.Rectangle
}

func (v *VolLayout) layout(r image.Rectangle) {
	v.r = r
	v.icon = centerV(leftH(box(16, 16), r), r)
	v.slide = image.Rectangle{image.Pt(v.icon.Max.X + 1, r.Min.Y), r.Max}
}

type MixWidget struct {
	ImageWidget
	mLevel, wLevel float64
	layout struct {
		master, midi, wave VolLayout
	}
}

func NewMixWidget(refresh chan Widget) *MixWidget {
	var mw MixWidget
	mw.refresh = refresh
	return &mw
}

func (m *MixWidget) Levels(mid, wav float64) {
	m.mLevel = mid
	m.wLevel = wav
	m.refresh <- m
}

func (m *MixWidget) Toggle(mute *bool) {
	toggle(mute)
	m.refresh <- m
}

func (m *MixWidget) AdjustGain(gain *float64, δ float64) {
	(*gain) += δ
	m.refresh <- m
}


func (m *MixWidget) LeftClick(mouse image.Point) {
	m.click(mouse, -0.1)
}

func (m *MixWidget) RightClick(mouse image.Point) {
	m.click(mouse, 0.1)
}

func (m *MixWidget) click(mouse image.Point, δ float64) {
	if mouse.In(m.layout.master.r) {
		m.AdjustGain(&Mixer.Master.Gain, δ)
	} else if mouse.In(m.layout.midi.r) {
		m.AdjustGain(&Mixer.Midi.Gain, δ)
	} else if mouse.In(m.layout.wave.r) {
		m.AdjustGain(&Mixer.Wave.Gain, δ)
	}
}


func (m *MixWidget) Draw(screen wde.Image, r image.Rectangle) {
	dst, changed := m.Img(r)
	if changed {
		hbox := leftH(box(r.Dx(), (r.Dy() - 2) / 3), r)
		m.layout.master.layout(topV(hbox, r))
		m.layout.midi.layout(centerV(hbox, r))
		m.layout.wave.layout(botV(hbox, r))
	}
	drawvol(dst, m.layout.master, Mixer.Master, IconVol, 0)
	drawvol(dst, m.layout.midi, Mixer.Midi, IconMidi, m.mLevel)
	drawvol(dst, m.layout.wave, Mixer.Wave, IconWave, m.wLevel)
	screen.CopyRGBA(dst, r)
}

func drawvol(dst draw.Image, layout VolLayout, vol MixVolume, icon *image.Alpha, level float64) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	fg := color.RGBA{0x00, 0x00, 0x00, 0xff}
	if vol.Muted {
		fg = color.RGBA{0x88, 0x88, 0x88, 0xff}
	}
	p := vol.Gain / 4.0
	if p > 0.97 {
		p = 0.97
	}
	draw.Draw(dst, layout.r, &image.Uniform{bg}, image.ZP, draw.Src)
	if level != 0 {
		lev := layout.r.Inset(1)
		lev.Max.X = lev.Min.X + int(float64(lev.Dx())*level)
		draw.Draw(dst, lev, &image.Uniform{levelCB.At(level)}, image.ZP, draw.Src)
	}
	draw.DrawMask(dst, layout.icon, &image.Uniform{fg}, image.ZP, icon, image.ZP, draw.Over)
	drawHorzSlider(dst, layout.slide, fg, p)
}

var levelCB ColourBar = ColourBar{[]ColourPoint{
	{0.50, color.NRGBA{0x00, 0xff, 0x00, 0xff}},
	{0.75, color.NRGBA{0xff, 0xff, 0x00, 0xff}},
	{1.00, color.NRGBA{0xff, 0x00, 0x00, 0xff}},
}}
