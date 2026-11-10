package printout

import (
	"strconv"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/pdf"
	"github.com/fiskus-online/fiskus/backend/internal/richtext"
)

// Formátovaný text dokladu (text nad položkami a pod nimi): tučné, kurzíva
// (šikmé písmo), podčiarknuté, odrážky a číslovaný zoznam s odsadením.

type word struct {
	text  string
	run   richtext.Run
	space bool // pred slovom je medzera
	brk   bool // zalomenie riadku (z textu)
}

type placed struct {
	x    float64
	w    float64
	text string
	run  richtext.Run
}

func (s *sheet) runFont(r richtext.Run) *pdf.Font {
	if r.Bold {
		return s.f.Bold
	}
	return s.f.Regular
}

// words rozdelí úseky na slová (medzera patrí pred slovo).
func words(runs []richtext.Run) []word {
	var out []word
	space := false
	for _, r := range runs {
		if r.Text == "\n" {
			out = append(out, word{brk: true})
			space = false
			continue
		}
		parts := strings.Split(r.Text, " ")
		for i, p := range parts {
			if i > 0 {
				space = true
			}
			if p == "" {
				continue
			}
			out = append(out, word{text: p, run: r, space: space && len(out) > 0 && !out[len(out)-1].brk})
			space = false
		}
	}
	return out
}

// layout zalomí slová do riadkov danej šírky.
func (s *sheet) layout(runs []richtext.Run, size, width float64) [][]placed {
	var lines [][]placed
	var line []placed
	x := 0.0
	newLine := func() {
		lines = append(lines, line)
		line, x = nil, 0
	}
	for _, w := range words(runs) {
		if w.brk {
			newLine()
			continue
		}
		f := s.runFont(w.run)
		gap := 0.0
		if w.space && len(line) > 0 {
			gap = f.Width(" ", size)
		}
		ww := f.Width(w.text, size)
		if len(line) > 0 && x+gap+ww > width {
			newLine()
			gap = 0
		}
		text := w.text
		for ww > width { // slovo dlhšie ako riadok
			rs := []rune(text)
			n := len(rs) - 1
			for n > 1 && f.Width(string(rs[:n]), size) > width {
				n--
			}
			line = append(line, placed{x: x, w: f.Width(string(rs[:n]), size), text: string(rs[:n]), run: w.run})
			newLine()
			text = string(rs[n:])
			ww = f.Width(text, size)
		}
		// slová s rovnakým formátom v riadku ako jeden text (menšie PDF, súvislý text)
		if k := len(line) - 1; k >= 0 && sameRun(line[k].run, w.run) {
			sep := ""
			if gap > 0 {
				sep = " "
			}
			line[k].text += sep + text
			line[k].w = x + gap + ww - line[k].x
		} else {
			line = append(line, placed{x: x + gap, w: ww, text: text, run: w.run})
		}
		x += gap + ww
	}
	if len(line) > 0 || len(lines) == 0 {
		lines = append(lines, line)
	}
	return lines
}

func sameRun(a, b richtext.Run) bool {
	return a.Bold == b.Bold && a.Italic == b.Italic && a.Underline == b.Underline
}

// richText nakreslí formátovaný (alebo obyčajný) text od okraja left.
func (s *sheet) richText(t string, size float64) {
	blocks := richtext.Blocks(t)
	indent := pdf.MM(5)
	for i, b := range blocks {
		if i > 0 && (b.List == "" || blocks[i-1].List == "") && richtext.IsRich(t) {
			s.y += pdf.MM(1) // medzera medzi odsekmi
		}
		x0, w := float64(left), float64(width)
		marker := ""
		switch b.List {
		case "ul":
			marker = "•"
		case "ol":
			marker = strconv.Itoa(b.Index) + "."
		}
		if marker != "" {
			x0, w = x0+indent, w-indent
		}
		for li, line := range s.layout(b.Runs, size, w) {
			s.ensure(lh(size))
			if li == 0 && marker != "" {
				s.textRight(x0-pdf.MM(1.5), s.y, s.f.Regular, size, ink, marker)
			}
			for k, p := range line {
				f := s.runFont(p.run)
				x := x0 + p.x
				if p.run.Italic {
					s.p.TextOblique(x, baseline(s.y, size), f, size, ink, p.text)
				} else {
					s.text(x, s.y, f, size, ink, p.text)
				}
				if p.run.Underline {
					end := x + p.w
					// podčiarknutie pokračuje cez medzeru k ďalšiemu podčiarknutému slovu
					if k+1 < len(line) && line[k+1].run.Underline {
						end = x0 + line[k+1].x
					}
					uy := baseline(s.y, size) + size*0.12
					s.p.Line(x, uy, end, uy, size*0.06, ink)
				}
			}
			s.y += lh(size)
		}
	}
}
