package main

import (
	"fmt"
	"hash/fnv"
	"html/template"
	"math"
	"math/rand/v2"
	"strings"
)

// Every bean gets its own generative "bag label": colours come from roast
// level (base) and process (accent); the composition is seeded by the bean
// itself, so it's stable across renders but unique per coffee.

type palette struct{ bg, main, accent, ink string }

func beanPalette(roast, process string) palette {
	p := palette{bg: "#EADFCB", main: "#B08A63", accent: "#2E7D6B", ink: "#1E1612"}
	switch strings.ToLower(roast) {
	case "light":
		p.bg, p.main = "#F6DFB0", "#EFA94A"
	case "medium-light":
		p.bg, p.main = "#F3CF98", "#DE8A3A"
	case "medium":
		p.bg, p.main = "#E9B486", "#C0602F"
	case "medium-dark":
		p.bg, p.main = "#CF8A5E", "#8A4126"
	case "dark":
		p.bg, p.main, p.ink = "#6B3F2C", "#2E1B14", "#F4EBDD"
	}
	switch strings.ToLower(process) {
	case "washed":
		p.accent = "#2F5DA8"
	case "natural":
		p.accent = "#C93756"
	case "honey":
		p.accent = "#F2B33D"
	case "anaerobic":
		p.accent = "#6D45A8"
	}
	return p
}

func seeded(parts ...string) (*rand.Rand, uint64) {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(strings.ToLower(strings.TrimSpace(p))))
		h.Write([]byte{0})
	}
	s := h.Sum64()
	return rand.New(rand.NewPCG(s, s^0x9e3779b97f4a7c15)), s
}

func rf(r *rand.Rand, lo, hi float64) float64 { return lo + r.Float64()*(hi-lo) }

func labelArt(b Bean) template.HTML {
	pal := beanPalette(b.RoastLevel, b.Process)
	r, seed := seeded(b.Name, b.Roaster, b.Origin)
	id := fmt.Sprintf("l%x", seed&0xffffffff)
	const W, H = 300.0, 380.0

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg class="label-art" viewBox="0 0 %g %g" preserveAspectRatio="xMidYMid slice" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">`, W, H)
	fmt.Fprintf(&sb, `<defs><filter id="%s-n" x="0" y="0" width="100%%" height="100%%"><feTurbulence type="fractalNoise" baseFrequency=".85" numOctaves="2" seed="%d" stitchTiles="stitch"/><feColorMatrix values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 .55 -.18"/></filter>`, id, seed%997)
	fmt.Fprintf(&sb, `<clipPath id="%s-c"><rect width="%g" height="%g"/></clipPath></defs>`, id, W, H)
	fmt.Fprintf(&sb, `<g clip-path="url(#%s-c)"><rect width="%g" height="%g" fill="%s"/>`, id, W, H, pal.bg)

	switch r.IntN(5) {
	case 0: // sun & rings
		cx, cy, R := rf(r, 70, 230), rf(r, 120, 250), rf(r, 95, 140)
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, cx, cy, R, pal.main)
		for i := 1; i <= 9; i++ {
			fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="%s" stroke-width="1.3" opacity=".55"/>`, cx, cy, R+float64(i)*13, pal.ink)
		}
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" style="mix-blend-mode:multiply"/>`, rf(r, 40, 260), rf(r, 40, 340), rf(r, 34, 60), pal.accent)

	case 1: // topographic hills
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, rf(r, 60, 240), rf(r, 50, 120), rf(r, 40, 70), pal.accent)
		ph, amp, freq := rf(r, 0, 6.28), rf(r, 14, 30), rf(r, 0.012, 0.022)
		hill := wave(-10, 230, W+10, amp, freq, ph)
		fmt.Fprintf(&sb, `<path d="%s L%g %g L-10 %g Z" fill="%s"/>`, hill, W+10, H+10, H+10, pal.main)
		for i := 0; i < 13; i++ {
			y := 40 + float64(i)*26
			fmt.Fprintf(&sb, `<path d="%s" fill="none" stroke="%s" stroke-width="1.3" opacity=".6"/>`,
				wave(-10, y, W+10, amp*rf(r, .6, 1.2), freq, ph+float64(i)*0.35), pal.ink)
		}

	case 2: // riso circles
		cols := []string{pal.main, pal.accent, pal.ink}
		for i := 0; i < 3; i++ {
			op := "1"
			if i == 2 {
				op = ".18"
			}
			fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="%s" style="mix-blend-mode:multiply"/>`,
				rf(r, 60, 240), rf(r, 90, 300), rf(r, 70, 115), cols[i], op)
		}
		for x := 0; x < 8; x++ {
			for y := 0; y < 10; y++ {
				fmt.Fprintf(&sb, `<circle cx="%d" cy="%d" r="2" fill="%s" opacity=".4"/>`, 24+x*36, 22+y*38, pal.ink)
			}
		}

	case 3: // the bean
		for i := -10; i < 22; i++ {
			x := float64(i) * 22
			fmt.Fprintf(&sb, `<path d="M%.1f 0 L%.1f %g" stroke="%s" stroke-width="7" opacity=".9"/>`, x, x-H*0.6, H, pal.accent)
		}
		rot := rf(r, -40, 40)
		cx, cy := W/2+rf(r, -20, 20), H/2+rf(r, -20, 30)
		fmt.Fprintf(&sb, `<g transform="rotate(%.1f %.1f %.1f)"><ellipse cx="%.1f" cy="%.1f" rx="92" ry="132" fill="%s"/>`, rot, cx, cy, cx, cy, pal.main)
		fmt.Fprintf(&sb, `<path d="M%.1f %.1f C%.1f %.1f %.1f %.1f %.1f %.1f" fill="none" stroke="%s" stroke-width="11" stroke-linecap="round"/></g>`,
			cx-8, cy-118, cx+42, cy-40, cx-42, cy+40, cx+8, cy+118, pal.bg)

	default: // bauhaus quarter-circles
		cols := []string{pal.main, pal.accent, pal.ink, pal.bg}
		cell := 75.0
		for gx := 0; gx < 4; gx++ {
			for gy := 0; gy < 6; gy++ {
				x, y := float64(gx)*cell, float64(gy)*cell-10
				c := cols[r.IntN(len(cols))]
				if c == pal.bg {
					continue
				}
				op := "1"
				if c == pal.ink {
					op = ".85"
				}
				switch r.IntN(4) {
				case 0:
					fmt.Fprintf(&sb, `<path d="M%.0f %.0f h%.0f a%.0f %.0f 0 0 1 -%.0f %.0f z" fill="%s" opacity="%s"/>`, x, y, cell, cell, cell, cell, cell, c, op)
				case 1:
					fmt.Fprintf(&sb, `<path d="M%.0f %.0f v%.0f a%.0f %.0f 0 0 0 %.0f -%.0f z" fill="%s" opacity="%s"/>`, x+cell, y, cell, cell, cell, -cell, cell, c, op)
				case 2:
					fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="%s"/>`, x+cell/2, y+cell/2, cell/2-6, c, op)
				default:
					fmt.Fprintf(&sb, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="%s" opacity="%s"/>`, x, y+cell/2, cell, cell/2, c, op)
				}
			}
		}
	}

	fmt.Fprintf(&sb, `<rect width="%g" height="%g" filter="url(#%s-n)" opacity=".9"/></g></svg>`, W, H, id)
	return template.HTML(sb.String())
}

func wave(x0, y, x1, amp, freq, phase float64) string {
	var sb strings.Builder
	for x := x0; x <= x1; x += 10 {
		yy := y + amp*math.Sin(x*freq+phase) + amp*0.35*math.Sin(x*freq*2.3+phase*1.7)
		if x == x0 {
			fmt.Fprintf(&sb, "M%.1f %.1f", x, yy)
		} else {
			fmt.Fprintf(&sb, " L%.1f %.1f", x, yy)
		}
	}
	return sb.String()
}

// swatch is a tiny roast/process token used in lists.
func swatch(roast, process string) template.HTML {
	p := beanPalette(roast, process)
	return template.HTML(fmt.Sprintf(`<svg class="swatch" viewBox="0 0 24 24" aria-hidden="true"><circle cx="10" cy="12" r="8" fill="%s"/><circle cx="15" cy="12" r="7" fill="%s" style="mix-blend-mode:multiply" opacity=".9"/></svg>`, p.main, p.accent))
}

// radar draws the 7-axis flavour profile. It carries data attributes so the
// bean form can animate it live while sliders move (see app.js).
func radar(b Bean) template.HTML {
	axes := b.Axes()
	n := len(axes)
	const cx, cy, R = 160.0, 160.0, 108.0
	pt := func(i int, v float64) (float64, float64) {
		a := float64(i)*2*math.Pi/float64(n) - math.Pi/2
		return cx + math.Cos(a)*R*v/10, cy + math.Sin(a)*R*v/10
	}
	keys := make([]string, n)
	for i, a := range axes {
		keys[i] = a.Key
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg class="radar" viewBox="-44 -4 408 330" data-radar data-keys="%s" data-cx="%g" data-cy="%g" data-r="%g" role="img" aria-label="Flavour profile">`,
		strings.Join(keys, ","), cx, cy, R)
	for ring := 2; ring <= 10; ring += 2 {
		var pts []string
		for i := 0; i < n; i++ {
			x, y := pt(i, float64(ring))
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", x, y))
		}
		cls := "radar-ring"
		if ring == 10 {
			cls += " outer"
		}
		fmt.Fprintf(&sb, `<polygon class="%s" points="%s"/>`, cls, strings.Join(pts, " "))
	}
	for i := 0; i < n; i++ {
		x, y := pt(i, 10)
		fmt.Fprintf(&sb, `<line class="radar-spoke" x1="%g" y1="%g" x2="%.1f" y2="%.1f"/>`, cx, cy, x, y)
	}
	var pts []string
	var dots strings.Builder
	for i, a := range axes {
		v := 0.0
		if a.Value != nil {
			v = float64(*a.Value)
		}
		x, y := pt(i, v)
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", x, y))
		fmt.Fprintf(&dots, `<circle class="radar-dot" r="4" cx="%.1f" cy="%.1f"/>`, x, y)
	}
	fmt.Fprintf(&sb, `<polygon class="radar-shape" points="%s"/>%s`, strings.Join(pts, " "), dots.String())
	for i, a := range axes {
		lx, ly := pt(i, 12.6)
		anchor := "middle"
		if lx < cx-10 {
			anchor = "end"
		} else if lx > cx+10 {
			anchor = "start"
		}
		val := "–"
		if a.Value != nil {
			val = fmt.Sprint(*a.Value)
		}
		fmt.Fprintf(&sb, `<text class="radar-label" x="%.1f" y="%.1f" text-anchor="%s" dominant-baseline="middle">%s <tspan class="radar-val" data-val="%s">%s</tspan></text>`,
			lx, ly, anchor, a.Label, a.Key, val)
	}
	sb.WriteString(`</svg>`)
	return template.HTML(sb.String())
}

// pulseBars renders the last shots as a rhythm strip: height = rating,
// colour = taste (sour / balanced / bitter).
func pulseBars(shots []Shot) template.HTML {
	if len(shots) == 0 {
		return ""
	}
	if len(shots) > 30 {
		shots = shots[:30]
	}
	const H, bw, gap = 90.0, 14.0, 6.0
	W := float64(len(shots))*(bw+gap) - gap
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg class="pulse" viewBox="0 0 %g %g" preserveAspectRatio="xMinYMax meet" role="img" aria-label="Recent shots">`, W, H+4)
	// oldest → newest, left → right
	for i := range shots {
		s := shots[len(shots)-1-i]
		x := float64(i) * (bw + gap)
		h, cls := 8.0, "p-none"
		if s.Rating != nil {
			h = float64(*s.Rating) / 5 * H
		}
		if s.Taste != nil {
			switch {
			case *s.Taste < 0:
				cls = "p-sour"
			case *s.Taste > 0:
				cls = "p-bitter"
			default:
				cls = "p-good"
			}
		} else if s.Rating != nil {
			cls = "p-rated"
		}
		title := s.BeanName
		if title == "" {
			title = "Shot"
		}
		fmt.Fprintf(&sb, `<a href="/shots/%d/edit"><rect class="%s" x="%.1f" y="%.1f" width="%g" height="%.1f" rx="7"><title>%s · %s</title></rect></a>`,
			s.ID, cls, x, H-h+2, bw, h, template.HTMLEscapeString(title), s.BrewedAt.Local().Format("2 Jan 15:04"))
	}
	sb.WriteString(`</svg>`)
	return template.HTML(sb.String())
}
