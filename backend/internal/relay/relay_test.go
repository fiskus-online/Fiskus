package relay

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func keys(t *testing.T) Keys {
	t.Helper()
	s, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	k, err := Derive(s)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestDerive(t *testing.T) {
	secret := bytes.Repeat([]byte{7}, SecretSize)
	a, _ := Derive(secret)
	b, _ := Derive(secret)
	if a.Channel != b.Channel || a.Auth != b.Auth || len(a.Channel) != 32 {
		t.Fatalf("odvodenie nie je stále: %q %q", a.Channel, b.Channel)
	}
	if strings.Contains(a.Auth, a.Channel) {
		t.Fatal("prístupový kľúč nesmie obsahovať meno kanála")
	}
	if _, err := Derive([]byte("krátke")); err == nil {
		t.Fatal("krátke tajomstvo musí zlyhať")
	}
}

func TestSeal(t *testing.T) {
	k, other := keys(t), keys(t)
	sealed, err := k.Seal(KindItem, []byte("ahoj"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("ahoj")) {
		t.Fatal("obsah nie je zašifrovaný")
	}
	cases := []struct {
		name string
		keys Keys
		kind string
		data []byte
		ok   bool
	}{
		{"správny kľúč", k, KindItem, sealed, true},
		{"iný druh (záznam ako kópia)", k, KindSnapshot, sealed, false},
		{"iný kľúč", other, KindItem, sealed, false},
		{"poškodené", k, KindItem, append(append([]byte{}, sealed[:len(sealed)-1]...), sealed[len(sealed)-1]^1), false},
		{"prikrátke", k, KindItem, sealed[:5], false},
	}
	for _, c := range cases {
		plain, err := c.keys.Open(c.kind, c.data)
		if c.ok != (err == nil) || (c.ok && string(plain) != "ahoj") {
			t.Errorf("%s: %q %v", c.name, plain, err)
		}
		if !c.ok && !errors.Is(err, ErrSealed) {
			t.Errorf("%s: chyba %v", c.name, err)
		}
	}
}

func TestPairLink(t *testing.T) {
	secret := bytes.Repeat([]byte{1}, SecretSize)
	cases := []struct {
		name, link, relay string
		ok                bool
	}{
		{"predvolená schránka", PairLink("https://app.fiskus.online/", secret, "Počítač Jana & syn", ""), "", true},
		{"vlastná schránka", PairLink("https://app.fiskus.online", secret, "PC", "https://moja.example.sk"), "https://moja.example.sk", true},
		{"schránka bez https", PairLink("https://app.fiskus.online", secret, "PC", "http://moja.example.sk"), "", false},
		{"bez kľúča", "https://app.fiskus.online/#pripojit&n=PC", "", false},
	}
	for _, c := range cases {
		_, frag, _ := strings.Cut(c.link, "#")
		if c.ok && !strings.HasPrefix(c.link, "https://app.fiskus.online/#pripojit&") {
			t.Errorf("%s: odkaz %s", c.name, c.link)
		}
		p, err := ParsePairing(frag)
		if c.ok != (err == nil) {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if c.ok && (!bytes.Equal(p.Secret, secret) || p.Relay != c.relay || p.Name == "") {
			t.Errorf("%s: %+v", c.name, p)
		}
	}
}

func TestClientServer(t *testing.T) {
	srv := httptest.NewServer(NewMemServer())
	defer srv.Close()
	ctx := context.Background()
	k := keys(t)
	c := Client{Base: srv.URL, Keys: k}

	if _, _, err := c.GetSnapshot(ctx); !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("prázdna schránka: %v", err)
	}
	st, err := c.State(ctx)
	if err != nil || st.Snapshot != nil || len(st.Inbox) != 0 {
		t.Fatalf("stav: %+v %v", st, err)
	}
	// cudzí kľúč k tej istej schránke
	bad := k
	bad.Auth = keys(t).Auth
	if _, err := (Client{Base: srv.URL, Keys: bad}).State(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cudzí kľúč: %v", err)
	}

	if err := c.Want(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.PutSnapshot(ctx, 42, []byte("kopia")); err != nil {
		t.Fatal(err)
	}
	b, v, err := c.GetSnapshot(ctx)
	if err != nil || string(b) != "kopia" || v != 42 {
		t.Fatalf("kópia: %q %d %v", b, v, err)
	}
	ids := []string{}
	for _, x := range []string{"prvý", "druhý", "tretí"} {
		id, err := c.PostItem(ctx, []byte(x))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	st, _ = c.State(ctx)
	if st.Want != "" || st.Snapshot == nil || st.Snapshot.Version != 42 || len(st.Inbox) != 3 || st.Inbox[0].ID != ids[0] || st.Inbox[2].ID != ids[2] {
		t.Fatalf("stav po zápisoch: %+v", st)
	}
	if got, _ := c.GetItem(ctx, ids[1]); string(got) != "druhý" {
		t.Fatalf("záznam: %q", got)
	}
	for range 2 { // mazanie je opakovateľné
		if err := c.DeleteItem(ctx, ids[1]); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ = c.State(ctx); len(st.Inbox) != 2 {
		t.Fatalf("po zmazaní: %+v", st.Inbox)
	}
	if _, err := c.PostItem(ctx, make([]byte, MaxItem+1)); err == nil {
		t.Fatal("priveľký záznam musí zlyhať")
	}
	if err := c.Wipe(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.GetSnapshot(ctx); !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("po zrušení: %v", err)
	}
}
