// Package richtext: formátovaný text dokladov (text nad položkami a pod
// nimi). Ukladá sa ako malá podmnožina HTML: odseky, zalomenie riadku,
// tučné, kurzíva, podčiarknuté, odrážky a číslovaný zoznam. Čokoľvek iné
// (písma, farby, tabuľky z Wordu, skripty) sa pri uložení zahodí a ostane
// iba text. Rovnaké pravidlá má frontend (lib/richtext.js) — spoločné
// vzorky sú v testdata/cases.json.
//
// Starý text bez značiek (obyčajný text s koncami riadkov) ostáva tak, ako
// je; formátovaný text začína značkou „<“.
package richtext

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Run je úsek textu s jedným formátom. Text "\n" je zalomenie riadku.
type Run struct {
	Text      string
	Bold      bool
	Italic    bool
	Underline bool
}

// Block je odsek alebo položka zoznamu.
type Block struct {
	List  string // "" odsek, "ul" odrážka, "ol" číslovaná položka
	Index int    // poradie v číslovanom zozname (od 1)
	Runs  []Run
}

// IsRich: je text formátovaný (HTML), alebo obyčajný?
func IsRich(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "<") }

// Sanitize vráti text v povolenom tvare. Obyčajný text (bez „<“) vráti
// orezaný a nezmenený.
func Sanitize(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "<") {
		return s
	}
	return Render(Parse(s))
}

// Plain prevedie text na obyčajný (e-faktúra, e-mail): odseky na riadky,
// odrážky „• “, číslovanie „1. “.
func Plain(s string) string {
	if !IsRich(s) {
		return strings.TrimSpace(s)
	}
	var lines []string
	for _, b := range Parse(s) {
		var t strings.Builder
		switch b.List {
		case "ul":
			t.WriteString("• ")
		case "ol":
			t.WriteString(strconv.Itoa(b.Index) + ". ")
		}
		for _, r := range b.Runs {
			t.WriteString(r.Text)
		}
		lines = append(lines, t.String())
	}
	return strings.Join(lines, "\n")
}

// Blocks vráti odseky a položky na kreslenie (PDF). Obyčajný text: každý
// riadok je odsek bez formátu.
func Blocks(s string) []Block {
	if !IsRich(s) {
		var out []Block
		for _, l := range strings.Split(strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n"), "\n") {
			out = append(out, Block{Runs: []Run{{Text: l}}})
		}
		if len(out) == 1 && out[0].Runs[0].Text == "" {
			return nil
		}
		return out
	}
	return Parse(s)
}

// Append pridá k textu ďalší odsek (obyčajný text), v rovnakom tvare.
func Append(s, text string) string {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return s
	case strings.TrimSpace(s) == "":
		return text
	case IsRich(s):
		return Sanitize(s + "<p>" + html.EscapeString(text) + "</p>")
	}
	return strings.TrimSpace(s) + " " + text
}

// ---- čítanie HTML ----

var (
	reTag      = regexp.MustCompile(`^<\s*(/?)\s*([a-zA-Z][a-zA-Z0-9:_-]*)([^>]*?)(/?)\s*>`)
	reStyle    = regexp.MustCompile(`(?i)style\s*=\s*("([^"]*)"|'([^']*)')`)
	reSpace    = regexp.MustCompile(`[\s\x{00a0}]+`)
	reNumbered = regexp.MustCompile(`^(\d+|[a-zA-Z]|[ivxlcdmIVXLCDM]+)[.)]$`)
)

// skipped: obsah týchto značiek sa zahodí celý
var skipped = map[string]bool{"script": true, "style": true, "head": true, "title": true, "xml": true, "template": true, "noscript": true, "iframe": true, "object": true, "svg": true, "math": true}

// blocky: začiatok aj koniec značky ukončí odsek
var blocky = map[string]bool{"p": true, "div": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "pre": true, "section": true, "article": true, "header": true, "footer": true, "table": true,
	"tr": true, "dl": true, "dt": true, "dd": true, "figure": true, "address": true, "hr": true}

type format struct{ bold, italic, underline, ignore bool }

type open struct {
	name string
	fmt  format // formát platný vo vnútri
}

type parser struct {
	blocks []Block
	cur    *Block
	stack  []open
	lists  []string
	marker string // text z Word „mso-list:Ignore“ (odrážka alebo číslo)
}

func (p *parser) top() format {
	if len(p.stack) == 0 {
		return format{}
	}
	return p.stack[len(p.stack)-1].fmt
}

func (p *parser) flush() {
	if p.cur != nil {
		p.blocks = append(p.blocks, *p.cur)
		p.cur = nil
	}
}

func (p *parser) block() *Block {
	if p.cur == nil {
		p.cur = &Block{}
		if p.marker != "" { // odsek zo zoznamu vo Worde
			p.cur.List = "ul"
			if reNumbered.MatchString(p.marker) {
				p.cur.List = "ol"
			}
			p.marker = ""
		}
	}
	return p.cur
}

func (p *parser) text(t string) {
	f := p.top()
	if f.ignore {
		p.marker += strings.TrimSpace(reSpace.ReplaceAllString(html.UnescapeString(t), ""))
		return
	}
	t = reSpace.ReplaceAllString(html.UnescapeString(t), " ")
	if t == "" {
		return
	}
	b := p.block()
	b.Runs = append(b.Runs, Run{Text: t, Bold: f.bold, Italic: f.italic, Underline: f.underline})
}

// Parse prečíta formátovaný text na odseky a položky.
func Parse(s string) []Block {
	p := &parser{}
	for len(s) > 0 {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			p.text(s)
			break
		}
		if i > 0 {
			p.text(s[:i])
			s = s[i:]
		}
		if strings.HasPrefix(s, "<!--") {
			end := strings.Index(s, "-->")
			if end < 0 {
				break
			}
			s = s[end+3:]
			continue
		}
		if strings.HasPrefix(s, "<!") || strings.HasPrefix(s, "<?") {
			end := strings.IndexByte(s, '>')
			if end < 0 {
				break
			}
			s = s[end+1:]
			continue
		}
		m := reTag.FindStringSubmatch(s)
		if m == nil {
			p.text("<") // osamelé „<“ je text
			s = s[1:]
			continue
		}
		s = s[len(m[0]):]
		closing, name, attrs, self := m[1] == "/", strings.ToLower(m[2]), m[3], m[4] == "/"
		if skipped[name] && !closing && !self {
			end := strings.Index(strings.ToLower(s), "</"+name)
			if end < 0 {
				break
			}
			s = s[end:]
			if j := strings.IndexByte(s, '>'); j >= 0 {
				s = s[j+1:]
			}
			continue
		}
		if closing {
			p.close(name)
		} else {
			p.open(name, attrs, self)
		}
	}
	p.flush()
	return finish(p.blocks)
}

func (p *parser) open(name, attrs string, self bool) {
	switch {
	case name == "br":
		b := p.block()
		b.Runs = append(b.Runs, Run{Text: "\n"})
		return
	case name == "ul" || name == "ol":
		p.flush()
		p.lists = append(p.lists, name)
		return
	case name == "li":
		p.flush()
		kind := "ul"
		if len(p.lists) > 0 {
			kind = p.lists[len(p.lists)-1]
		}
		p.cur = &Block{List: kind}
	case blocky[name]:
		p.flush()
	case name == "td" || name == "th":
		if p.cur != nil { // bunky tabuľky v jednom riadku oddelí medzera
			p.cur.Runs = append(p.cur.Runs, Run{Text: " "})
		}
	}
	if self {
		return
	}
	f := p.top()
	switch name {
	case "b", "strong":
		f.bold = true
	case "i", "em", "cite", "dfn", "var":
		f.italic = true
	case "u", "ins":
		f.underline = true
	case "h1", "h2", "h3", "h4", "h5", "h6", "th":
		f.bold = true
	}
	if m := reStyle.FindStringSubmatch(attrs); m != nil {
		st := strings.ToLower(m[2] + m[3])
		st = strings.ReplaceAll(st, " ", "")
		switch {
		case strings.Contains(st, "mso-list:ignore"):
			f.ignore = true
		}
		if strings.Contains(st, "font-weight:bold") || strings.Contains(st, "font-weight:600") || strings.Contains(st, "font-weight:700") ||
			strings.Contains(st, "font-weight:800") || strings.Contains(st, "font-weight:900") {
			f.bold = true
		}
		if strings.Contains(st, "font-weight:normal") || strings.Contains(st, "font-weight:400") {
			f.bold = false
		}
		if strings.Contains(st, "font-style:italic") {
			f.italic = true
		}
		if strings.Contains(st, "text-decoration:underline") || strings.Contains(st, "text-decoration-line:underline") {
			f.underline = true
		}
	}
	p.stack = append(p.stack, open{name: name, fmt: f})
}

func (p *parser) close(name string) {
	switch {
	case name == "ul" || name == "ol":
		p.flush()
		if n := len(p.lists); n > 0 {
			p.lists = p.lists[:n-1]
		}
		return
	case name == "li" || blocky[name]:
		p.flush()
	}
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].name == name {
			p.stack = p.stack[:i]
			break
		}
	}
}

// finish oreže medzery, zlúči rovnaké úseky, vynechá prázdne odseky a
// očísluje položky zoznamov.
func finish(in []Block) []Block {
	var out []Block
	n := 0
	for _, b := range in {
		var runs []Run
		for _, r := range b.Runs {
			if r.Text == "\n" {
				runs = append(runs, r)
				continue
			}
			// medzera za zalomením alebo na začiatku je zbytočná
			if len(runs) == 0 || runs[len(runs)-1].Text == "\n" || strings.HasSuffix(runs[len(runs)-1].Text, " ") {
				r.Text = strings.TrimLeft(r.Text, " ")
			}
			if r.Text == "" {
				continue
			}
			if k := len(runs) - 1; k >= 0 && runs[k].Text != "\n" && runs[k].Bold == r.Bold && runs[k].Italic == r.Italic && runs[k].Underline == r.Underline {
				runs[k].Text += r.Text
				continue
			}
			runs = append(runs, r)
		}
		// koniec odseku: medzery a zalomenia
		for len(runs) > 0 {
			k := len(runs) - 1
			if runs[k].Text == "\n" {
				runs = runs[:k]
				continue
			}
			runs[k].Text = strings.TrimRight(runs[k].Text, " ")
			if runs[k].Text == "" {
				runs = runs[:k]
				continue
			}
			break
		}
		for k := range runs { // medzera pred zalomením
			if k+1 < len(runs) && runs[k+1].Text == "\n" {
				runs[k].Text = strings.TrimRight(runs[k].Text, " ")
			}
		}
		if len(runs) == 0 {
			continue
		}
		b.Runs = runs
		if b.List == "ol" {
			if len(out) > 0 && out[len(out)-1].List == "ol" {
				n++
			} else {
				n = 1
			}
			b.Index = n
		}
		out = append(out, b)
	}
	return out
}

// Render zapíše odseky v povolenom tvare HTML.
func Render(bs []Block) string {
	var b strings.Builder
	list := ""
	for _, bl := range bs {
		if bl.List != list {
			if list != "" {
				b.WriteString("</" + list + ">")
			}
			if bl.List != "" {
				b.WriteString("<" + bl.List + ">")
			}
			list = bl.List
		}
		tag := "p"
		if bl.List != "" {
			tag = "li"
		}
		b.WriteString("<" + tag + ">")
		for _, r := range bl.Runs {
			if r.Text == "\n" {
				b.WriteString("<br>")
				continue
			}
			t := html.EscapeString(r.Text)
			if r.Underline {
				t = "<u>" + t + "</u>"
			}
			if r.Italic {
				t = "<em>" + t + "</em>"
			}
			if r.Bold {
				t = "<strong>" + t + "</strong>"
			}
			b.WriteString(t)
		}
		b.WriteString("</" + tag + ">")
	}
	if list != "" {
		b.WriteString("</" + list + ">")
	}
	return b.String()
}
