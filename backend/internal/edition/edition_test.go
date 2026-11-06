package edition

import "testing"

func TestEditions(t *testing.T) {
	tests := []struct {
		name    string
		caps    func() (Capabilities, error)
		mode    Mode
		browser bool
	}{
		{"desktop", func() (Capabilities, error) { return For(Desktop) }, Desktop, false},
		{"server", func() (Capabilities, error) { return For(Server) }, Server, false},
		{"prehliadač", func() (Capabilities, error) { return ForBrowser(), nil }, Desktop, true},
	}
	for _, tt := range tests {
		c, err := tt.caps()
		if err != nil {
			t.Fatal(err)
		}
		// zadarmo a bez obmedzenia počtu firiem vo všetkých režimoch
		if c.Mode != tt.mode || c.Browser != tt.browser || !c.MultiCompany {
			t.Errorf("%s: %+v", tt.name, c)
		}
	}
}

func TestUnknownMode(t *testing.T) {
	if _, err := For("cloud"); err == nil {
		t.Fatal("neznámy režim mal zlyhať")
	}
}
