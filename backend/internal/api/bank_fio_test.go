package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestFioAPI(t *testing.T) {
	data, err := os.ReadFile("../bank/testdata/fio.json")
	if err != nil {
		t.Fatal(err)
	}
	bankSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer bankSrv.Close()

	e := setup(t, edition.Desktop)
	e.app.FioURL = bankSrv.URL
	base := fmt.Sprintf("/api/companies/%d/bank/fio", e.cid)
	token := strings.Repeat("Ab3", 20)

	e.do("POST", base, map[string]string{"token": "zlý"}, 422, nil)
	var list []map[string]any
	e.do("POST", base, map[string]string{"label": "Fio", "token": token}, 200, &list)
	if len(list) != 1 || list[0]["tokenSet"] != true || list[0]["token"] != nil {
		t.Fatalf("napojenie: %+v", list)
	}
	id := int64(list[0]["id"].(float64))
	var res struct{ Imported int }
	e.do("POST", fmt.Sprintf("%s/%d/sync", base, id), nil, 200, &res)
	if res.Imported != 3 {
		t.Fatalf("stiahnutie: %+v", res)
	}
	e.do("POST", fmt.Sprintf("%s/%d/sync", base, id), nil, 422, nil) // limit 30 s
	e.do("GET", base, nil, 200, &list)
	if fmt.Sprint(list) == "" || strings.Contains(fmt.Sprint(list), token) {
		t.Fatalf("zoznam: %+v", list)
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, id), nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("zmazanie: %+v", list)
	}
}
