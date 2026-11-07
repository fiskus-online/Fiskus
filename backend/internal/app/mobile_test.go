package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestMobilePairing(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	now := time.Date(2026, 10, 31, 17, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	a.LANAddresses = func() []string { return []string{"192.168.1.20"} }
	var listening []int
	a.OnMobile = func(on bool, port int) error {
		if on {
			listening = append(listening, port)
		}
		return nil
	}
	admin := auth.WithActor(ctx, auth.Local)

	if _, err := a.NewMobilePairing(admin); !fieldErr(err, "enabled") {
		t.Fatalf("vypnutý: %v", err)
	}
	if _, err := a.SetMobile(admin, true, 80); !fieldErr(err, "port") {
		t.Errorf("nízky port: %v", err)
	}
	st, err := a.SetMobile(admin, true, 0)
	if err != nil || !st.Enabled || st.Port != DefaultMobilePort || len(listening) != 1 || st.URLs[0] != "http://192.168.1.20:48081" {
		t.Fatalf("zapnutie %+v %v %v", st, err, listening)
	}
	p, err := a.NewMobilePairing(admin)
	if err != nil || len(p.Code) != 6 || !strings.HasSuffix(p.URLs[0], "/parovanie?kod="+p.Code) {
		t.Fatalf("kód %+v %v", p, err)
	}
	if _, err := a.PairMobile(ctx, "ZLYKOD", "Môj telefón"); err != ErrPairCode {
		t.Errorf("zlý kód: %v", err)
	}
	token, err := a.PairMobile(ctx, strings.ToLower(p.Code), "Pavlov telefón")
	if err != nil || !strings.HasPrefix(token, "fmd_") {
		t.Fatalf("spárovanie %q %v", token, err)
	}
	if _, err := a.PairMobile(ctx, p.Code, "druhý"); err != ErrPairCode {
		t.Errorf("kód je jednorazový: %v", err)
	}
	act, err := a.MobileActor(ctx, token, "192.168.1.30")
	if err != nil || !act.SuperAdmin || !act.Mobile || act.Name != "Telefón: Pavlov telefón" || act.IP != "192.168.1.30" {
		t.Fatalf("telefón %+v %v", act, err)
	}
	if _, err := a.MobileActor(ctx, token+"x", ""); err != ErrNotPaired {
		t.Errorf("zlý token: %v", err)
	}
	// telefón nespravuje párovanie
	if _, err := a.NewMobilePairing(auth.WithActor(ctx, act)); err != ErrForbidden {
		t.Errorf("telefón pároval: %v", err)
	}
	// stav nevracia odtlačky tokenov
	st, _ = a.MobileStatus(admin)
	if len(st.Devices) != 1 || st.Devices[0].Hash != "" || st.Devices[0].Name != "Pavlov telefón" {
		t.Fatalf("zariadenia %+v", st.Devices)
	}
	// vypršaný kód
	p2, _ := a.NewMobilePairing(admin)
	now = now.Add(6 * time.Minute)
	if _, err := a.PairMobile(ctx, p2.Code, "x"); err != ErrPairCode {
		t.Errorf("vypršaný: %v", err)
	}
	// odobratie → token neplatí; vypnutie → nikto
	if _, err := a.RemoveMobileDevice(admin, st.Devices[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.MobileActor(ctx, token, ""); err != ErrNotPaired {
		t.Errorf("odobratý: %v", err)
	}
	// veľa zlých pokusov zablokuje párovanie
	for i := 0; i < maxPairFailures; i++ {
		_, _ = a.PairMobile(ctx, "AAAAAA", "x")
	}
	p3, _ := a.NewMobilePairing(admin)
	if _, err := a.PairMobile(ctx, p3.Code, "x"); err != ErrPairBlocked {
		t.Errorf("zablokovanie: %v", err)
	}
}

func TestMobileServerMode(t *testing.T) {
	a, _ := newApp(t, edition.Server)
	if _, err := a.MobileStatus(auth.WithActor(ctx, auth.Local)); err != ErrDesktopOnly {
		t.Errorf("server: %v", err)
	}
	if _, err := a.PairMobile(ctx, "ABCDEF", "x"); err != ErrDesktopOnly {
		t.Errorf("server párovanie: %v", err)
	}
}

type fakeFirewall struct {
	blocked bool
	allowed []int
	err     error
}

func (f *fakeFirewall) Check(_ context.Context, port int) FirewallState {
	return FirewallState{Supported: true, Blocked: f.blocked, Allowed: len(f.allowed) > 0 && f.allowed[len(f.allowed)-1] == port}
}

func (f *fakeFirewall) Allow(_ context.Context, port int) error {
	if f.err != nil {
		return f.err
	}
	f.blocked = false
	f.allowed = append(f.allowed, port)
	return nil
}

func TestMobileDiagnostics(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	now := time.Date(2026, 11, 6, 19, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	a.OnMobile = func(bool, int) error { return nil }
	admin := auth.WithActor(ctx, auth.Local)
	if _, err := a.SetMobile(admin, true, 48090); err != nil {
		t.Fatal(err)
	}
	st, _ := a.MobileStatus(admin)
	if st.LastRequest != nil || st.FirewallManaged {
		t.Fatalf("na začiatku: %+v", st)
	}
	if fw, err := a.MobileFirewallState(admin); err != nil || fw.Supported {
		t.Fatalf("bez správy firewallu: %+v %v", fw, err)
	}
	if _, err := a.AllowMobileFirewall(admin); !fieldErr(err, "firewall") {
		t.Fatalf("povolenie bez Windows: %v", err)
	}

	a.NoteLANRequest("192.168.1.35", "/parovanie", false)
	st, _ = a.MobileStatus(admin)
	if st.LastRequest == nil || st.LastRequest.IP != "192.168.1.35" || !st.LastRequest.At.Equal(now) {
		t.Fatalf("posledná požiadavka: %+v", st.LastRequest)
	}

	fw := &fakeFirewall{blocked: true}
	a.Firewall = fw
	if s, _ := a.MobileFirewallState(admin); !s.Blocked || s.Allowed || s.Public == nil {
		t.Fatalf("zablokovaný: %+v", s)
	}
	s, err := a.AllowMobileFirewall(admin)
	if err != nil || s.Blocked || !s.Allowed || fw.allowed[0] != 48090 {
		t.Fatalf("povolený port: %+v %v %v", s, err, fw.allowed)
	}
	fw.err = errors.New("povolenie sa nepotvrdilo")
	if _, err := a.AllowMobileFirewall(admin); !fieldErr(err, "firewall") {
		t.Fatalf("zrušené UAC: %v", err)
	}
	if _, err := a.MobileFirewallState(ctx); err == nil {
		t.Error("bez správcu")
	}
}
