package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func putPerson(t *testing.T, s *server, label string, p any) int {
	t.Helper()
	body, _ := json.Marshal(p)
	sum := sha256.Sum256(body)
	k := testKey(label)
	return serve(s, sign(t, request("PUT", "/v1/people/me", body), k, "person-save", addrOf(k)+"."+hex.EncodeToString(sum[:]), testNow.Unix())).Code
}

func lookup(t *testing.T, s *server, reader, target string) (int, map[string]string) {
	t.Helper()
	w := serve(s, sign(t, request("GET", "/v1/people/"+target, nil), testKey(reader), "person-lookup", target, testNow.Unix()))
	var out map[string]string
	_ = json.NewDecoder(w.Body).Decode(&out)
	return w.Code, out
}

func sponsoredLedger(t *testing.T, s *server, labels ...string) {
	var recs []sponsorRecord
	for _, l := range labels {
		recs = append(recs, sponsorRecord{Address: addrOf(testKey(l))})
	}
	raw, _ := json.Marshal(recs)
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "sponsored.json"), raw, 0o600))
}

func TestAPersonWritesItsOwnRecordAndReadsItBack(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusOK, putPerson(t, s, "awa", person{FirstName: " Awa ", LastName: "Diop", BirthDate: "1990-04-12"}))
	k := testKey("awa")
	w := serve(s, sign(t, request("GET", "/v1/people/me", nil), k, "person-read", addrOf(k), testNow.Unix()))
	require.Equal(t, http.StatusOK, w.Code)
	var p person
	require.NoError(t, json.NewDecoder(w.Body).Decode(&p))
	require.Equal(t, person{FirstName: "Awa", LastName: "Diop", BirthDate: "1990-04-12"}, p)
}

func TestAMemberSeesTheNameButNeverTheBirthDate(t *testing.T) {
	s := testServer(t)
	sponsoredLedger(t, s, "reader")
	require.Equal(t, http.StatusOK, putPerson(t, s, "awa", person{FirstName: "Awa", LastName: "Diop", BirthDate: "1990-04-12"}))
	code, out := lookup(t, s, "reader", addrOf(testKey("awa")))
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, map[string]string{"first_name": "Awa", "last_name": "Diop"}, out)
}

func TestAStaffMemberIsAMemberToo(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	require.Equal(t, http.StatusOK, putPerson(t, s, "awa", person{FirstName: "Awa", LastName: "Diop"}))
	code, _ := lookup(t, s, "staff", addrOf(testKey("awa")))
	require.Equal(t, http.StatusOK, code)
}

func TestAKeyOutsideTheAppReadsNoNames(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusOK, putPerson(t, s, "awa", person{FirstName: "Awa", LastName: "Diop"}))
	code, _ := lookup(t, s, "stranger", addrOf(testKey("awa")))
	require.Equal(t, http.StatusForbidden, code)
}

func TestNamesAndDatesAreChecked(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusBadRequest, putPerson(t, s, "a", person{FirstName: "", LastName: "Diop"}))
	require.Equal(t, http.StatusBadRequest, putPerson(t, s, "a", person{FirstName: "<b>", LastName: "Diop"}))
	require.Equal(t, http.StatusBadRequest, putPerson(t, s, "a", person{FirstName: "Awa", LastName: "Diop", BirthDate: "12/04/1990"}))
	require.Equal(t, http.StatusBadRequest, putPerson(t, s, "a", person{FirstName: "Awa", LastName: "Diop", BirthDate: "2099-01-01"}))
	require.Equal(t, http.StatusOK, putPerson(t, s, "a", person{FirstName: "Zoë-Anne", LastName: "N’Diaye", BirthDate: ""}))
	require.Equal(t, http.StatusOK, putPerson(t, s, "b", person{FirstName: "محمد", LastName: "الأمين"}))
}

func TestARecordIsBoundToItsBodyAndItsSigner(t *testing.T) {
	s := testServer(t)
	body, _ := json.Marshal(person{FirstName: "Awa", LastName: "Diop"})
	other, _ := json.Marshal(person{FirstName: "Eve", LastName: "Diop"})
	sum := sha256.Sum256(body)
	k := testKey("awa")
	w := serve(s, sign(t, request("PUT", "/v1/people/me", other), k, "person-save", addrOf(k)+"."+hex.EncodeToString(sum[:]), testNow.Unix()))
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAnAccountWithAProfileReadsNames(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusOK, putPerson(t, s, "awa", person{FirstName: "Awa", LastName: "Diop"}))
	require.Equal(t, http.StatusOK, putPerson(t, s, "pay-user", person{FirstName: "Kofi", LastName: "Mensah"}))
	code, out := lookup(t, s, "pay-user", addrOf(testKey("awa")))
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "Awa", out["first_name"])
}
