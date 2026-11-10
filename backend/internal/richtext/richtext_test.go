package richtext

import (
	"encoding/json"
	"os"
	"testing"
)

type tcase struct {
	Name  string `json:"name"`
	In    string `json:"in"`
	Out   string `json:"out"`
	Plain string `json:"plain"`
}

// spoločné vzorky s frontendom (lib/richtext.test.js)
func TestCases(t *testing.T) {
	b, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []tcase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := Sanitize(tc.In)
			if got != tc.Out {
				t.Fatalf("Sanitize\n got: %s\nwant: %s", got, tc.Out)
			}
			if again := Sanitize(got); again != got {
				t.Fatalf("druhé čistenie zmenilo text: %s", again)
			}
			if p := Plain(got); p != tc.Plain {
				t.Fatalf("Plain\n got: %q\nwant: %q", p, tc.Plain)
			}
		})
	}
}

func TestBlocksAppend(t *testing.T) {
	if bs := Blocks("A\nB"); len(bs) != 2 || bs[1].Runs[0].Text != "B" {
		t.Fatalf("obyčajný: %+v", bs)
	}
	if bs := Blocks(""); bs != nil {
		t.Fatalf("prázdny: %+v", bs)
	}
	bs := Blocks("<ol><li>a</li><li>b</li></ol><p>x</p><ol><li>c</li></ol>")
	if bs[1].Index != 2 || bs[3].Index != 1 {
		t.Fatalf("číslovanie: %+v", bs)
	}
	for _, tc := range []struct{ s, add, want string }{
		{"", "Vyúčtovanie.", "Vyúčtovanie."},
		{"Poznámka", "Vyúčtovanie.", "Poznámka Vyúčtovanie."},
		{"<p>Poznámka</p>", "Vyúčtovanie <b>.", "<p>Poznámka</p><p>Vyúčtovanie &lt;b&gt;.</p>"},
		{"<p>x</p>", " ", "<p>x</p>"},
	} {
		if got := Append(tc.s, tc.add); got != tc.want {
			t.Errorf("Append(%q, %q) = %q", tc.s, tc.add, got)
		}
	}
}
