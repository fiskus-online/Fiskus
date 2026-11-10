package invoice

import "testing"

// texty dokladu sa pri prepočte vyčistia na povolené formátovanie
func TestRecalculateSanitizesTexts(t *testing.T) {
	inv := Invoice{Intro: `<p onclick="x()">Úvod<script>alert(1)</script></p>`, Note: "  Obyčajný text\nriadok 2  "}
	if err := inv.Recalculate(); err != nil {
		t.Fatal(err)
	}
	if inv.Intro != "<p>Úvod</p>" || inv.Note != "Obyčajný text\nriadok 2" {
		t.Fatalf("%q %q", inv.Intro, inv.Note)
	}
}
