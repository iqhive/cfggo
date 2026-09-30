package main

import (
	"fmt"

	"time"

	"github.com/iqhive/banner"
)

// All styling, wordmark layout, opening animation, SVG and playback live in banner.
// This file owns project text, deterministic demonstrations and reading holds.
type frame = banner.Frame
type style = banner.Style

const (
	cols        = banner.Cols
	rows        = banner.Rows
	plain       = banner.Plain
	dim         = banner.Dim
	muted       = banner.Muted
	bright      = banner.Bright
	lit         = banner.Literal
	hot         = banner.Changed
	on          = banner.On
	off         = banner.Off
	bar         = banner.Bar
	prompt      = banner.Prompt
	caret       = banner.Caret
	full        = banner.Full
	shade       = banner.Shade
	projectName = "cfggo"
	tagline     = "Typed configuration that stays live across reloads."
	subline     = "typed live accessors · layered sources · value provenance"
	install     = "go get github.com/iqhive/cfggo"
)

var points = []string{"typed live accessors", "layered sources", "value provenance"}

type film struct {
	cur, card frame
	frames    []frame
}

func (m *film) cut(ms int) {
	f := m.cur
	f.Hold = time.Duration(ms) * time.Millisecond
	m.frames = append(m.frames, f)
}

// build executes the project fixtures. The seed affects decorative digits;
// iqhash also uses it for the real demonstrated digests.
func build(seed uint64) []frame {
	m := &film{}
	opening, card, err := banner.Opening(banner.Card{
		Word: projectName, Tagline: tagline, Points: points, Install: install,
		Wordmark: banner.WordmarkOptions{Seed: seed}, ReadHold: 2500 * time.Millisecond,
		TaglineReveal: banner.TaglineWithReveal, PointInterval: 200 * time.Millisecond,
	})
	if err != nil {
		panic(err)
	}
	m.frames, m.card = opening, card
	m.layers()
	m.reload()
	m.cur = m.card
	m.cut(2500)
	m.cur = frame{HiveLogo: true}
	m.cur.Center(14, "IQ Hive", bright)
	m.cur.Center(16, "iqhive.com", muted)
	m.cut(1000)
	// Keep all scenes and quick motion; distribute extra reading time among
	// settled demonstration results and the final project card.
	extra := 25*time.Second - banner.Duration(m.frames)
	if extra < 0 {
		panic("hero: scene timing exceeds 25 seconds")
	}
	var holds []int
	for i := len(opening); i < len(m.frames)-1; i++ {
		if m.frames[i].Hold >= time.Second {
			holds = append(holds, i)
		}
	}
	for n, i := range holds {
		share := extra / time.Duration(len(holds)-n)
		m.frames[i].Hold += share
		extra -= share
	}
	return m.frames
}

// animation keeps generator failures visible instead of writing guessed output.
func animation(seed uint64) (a banner.Animation, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("hero fixture: %v", p)
		}
	}()
	frames := build(seed)
	a = banner.Animation{Frames: frames, StaticFrame: len(frames) - 2,
		Title:       projectName + " — README demonstration",
		Description: "Real configuration precedence and live reload through typed accessors.",
		Command:     "make hero",
	}
	return a, a.Validate()
}
