package api

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	t0 := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		fails int
		after time.Duration
		allow bool
	}{
		{"bez pokusov", 0, 0, true},
		{"štyri chyby", 4, time.Minute, true},
		{"päť chýb", 5, time.Minute, false},
		{"päť chýb, po okne", 5, loginWindow, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var l limiter
			for i := 0; i < tt.fails; i++ {
				l.fail("ip|a@b.sk", t0)
			}
			if got := l.allow("ip|a@b.sk", t0.Add(tt.after)); got != tt.allow {
				t.Fatalf("allow %v", got)
			}
		})
	}
}

func TestLimiterSweep(t *testing.T) {
	var l limiter
	t0 := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		l.fail(fmt.Sprintf("ip|%d@x.sk", i), t0)
	}
	// po okne jeden ďalší pokus zmaže staré kľúče
	l.fail("ip|novy@x.sk", t0.Add(loginWindow+time.Second))
	if len(l.hits) != 1 {
		t.Fatalf("kľúče po čistení: %d", len(l.hits))
	}
	// allow bez pokusov v okne kľúč nenechá
	l.allow("ip|iny@x.sk", t0)
	if _, ok := l.hits["ip|iny@x.sk"]; ok {
		t.Fatal("prázdny kľúč ostal v mape")
	}
}
