package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type vaultRead struct {
	Members []vaultMember         `json:"members"`
	Cards   map[string]vaultEntry `json:"cards"`
}

func readVault(t *testing.T, s *server, label, jewelerID string) (int, vaultRead) {
	t.Helper()
	k := testKey(label)
	w := serve(s, sign(t, request("GET", "/v1/cardvault", nil), k, "cardvault-read", jewelerID, testNow.Unix()))
	var out vaultRead
	if w.Code == http.StatusOK {
		require.NoError(t, json.NewDecoder(w.Body).Decode(&out))
	}
	return w.Code, out
}

func putVault(t *testing.T, s *server, label, jewelerID, jewel string, body any) int {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	k := testKey(label)
	subject := sha256Hex([]byte(jewelerID + ":" + jewel + ":" + string(raw)))
	return serve(s, sign(t, request("PUT", "/v1/cardvault/"+jewel, raw), k, "cardvault-put", subject, testNow.Unix())).Code
}

// box is a well-formed 93-byte box whose content is irrelevant to the server.
func box(fill byte) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(rune(fill)), 93)))
}

const cardA = "02" + "11111111111111111111111111111111" + "11111111111111111111111111111111"
const cardB = "03" + "22222222222222222222222222222222" + "22222222222222222222222222222222"

func TestReadingTheVaultRecordsTheReadersKey(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	code, v := readVault(t, s, "staff", "1")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, []vaultMember{{Address: addrOf(testKey("staff")), Key: base64.StdEncoding.EncodeToString(testKey("staff").PubKey().Bytes())}}, v.Members)

	// Once the shop account reads too, both are listed, the business first.
	_, v = readVault(t, s, "shop", "1")
	require.Len(t, v.Members, 2)
	require.Equal(t, addrOf(testKey("shop")), v.Members[0].Address)
}

func TestOnlyMembersUseTheVault(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	code, _ := readVault(t, s, "stranger", "1")
	require.Equal(t, http.StatusForbidden, code)
	require.Equal(t, http.StatusForbidden, putVault(t, s, "stranger", "1", "7", vaultCard{CardKey: cardA, Boxes: map[string]string{}}))
}

func TestAReaderGetsOnlyItsOwnBox(t *testing.T) {
	shop, staff := addrOf(testKey("shop")), addrOf(testKey("staff"))
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	require.Equal(t, http.StatusOK, putVault(t, s, "staff", "1", "7", vaultCard{CardKey: cardA, Boxes: map[string]string{staff: box('s'), shop: box('b')}}))

	_, v := readVault(t, s, "shop", "1")
	require.Equal(t, box('b'), v.Cards["7"].Box)
	require.ElementsMatch(t, []string{shop, staff}, v.Cards["7"].Holders)
	require.Equal(t, cardA, v.Cards["7"].CardKey)
}

func TestBoxesAreAddedNeverOverwritten(t *testing.T) {
	shop, staff := addrOf(testKey("shop")), addrOf(testKey("staff"))
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	require.Equal(t, http.StatusOK, putVault(t, s, "staff", "1", "7", vaultCard{CardKey: cardA, Boxes: map[string]string{staff: box('s')}}))
	// A colleague cannot replace the staff member's box with its own.
	require.Equal(t, http.StatusOK, putVault(t, s, "shop", "1", "7", vaultCard{CardKey: cardA, Boxes: map[string]string{staff: box('x'), shop: box('b')}}))
	_, v := readVault(t, s, "staff", "1")
	require.Equal(t, box('s'), v.Cards["7"].Box)
	require.Len(t, v.Cards["7"].Holders, 2)
}

func TestANewCardReplacesTheOldBoxes(t *testing.T) {
	shop, staff := addrOf(testKey("shop")), addrOf(testKey("staff"))
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	require.Equal(t, http.StatusOK, putVault(t, s, "staff", "1", "7", vaultCard{CardKey: cardA, Boxes: map[string]string{staff: box('s'), shop: box('b')}}))
	require.Equal(t, http.StatusOK, putVault(t, s, "staff", "1", "7", vaultCard{CardKey: cardB, Boxes: map[string]string{staff: box('t')}}))
	_, v := readVault(t, s, "shop", "1")
	require.Equal(t, cardB, v.Cards["7"].CardKey)
	require.Empty(t, v.Cards["7"].Box, "the shop's box opened the old card")
	require.Equal(t, []string{staff}, v.Cards["7"].Holders)
}

func TestBoxesOnlyForMembersAndWellFormed(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	outsider := addrOf(testKey("stranger"))
	require.Equal(t, http.StatusBadRequest, putVault(t, s, "staff", "1", "7", vaultCard{CardKey: cardA, Boxes: map[string]string{outsider: box('o')}}))
	require.Equal(t, http.StatusBadRequest, putVault(t, s, "staff", "1", "7", vaultCard{CardKey: cardA, Boxes: map[string]string{addrOf(testKey("staff")): "c2hvcnQ="}}))
	require.Equal(t, http.StatusBadRequest, putVault(t, s, "staff", "1", "7", vaultCard{CardKey: "04" + cardA[2:], Boxes: map[string]string{}}))
	require.Equal(t, http.StatusBadRequest, putVault(t, s, "staff", "1", "07", vaultCard{CardKey: cardA, Boxes: map[string]string{}}))
}

func TestAPutIsBoundToItsBody(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	raw, _ := json.Marshal(vaultCard{CardKey: cardA, Boxes: map[string]string{}})
	other, _ := json.Marshal(vaultCard{CardKey: cardB, Boxes: map[string]string{}})
	subject := sha256Hex([]byte("1:7:" + string(raw)))
	w := serve(s, sign(t, request("PUT", "/v1/cardvault/7", other), testKey("staff"), "cardvault-put", subject, testNow.Unix()))
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

type fakeKeys map[string]string

func (f fakeKeys) AccountKey(_ context.Context, address string) (string, error) { return f[address], nil }

func TestAMemberKnownToTheChainIsListedBeforeItReads(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("shop"), testKey("staff")))
	shopKey := base64.StdEncoding.EncodeToString(testKey("shop").PubKey().Bytes())
	s.keys = fakeKeys{addrOf(testKey("shop")): shopKey}
	_, v := readVault(t, s, "staff", "1")
	require.Len(t, v.Members, 2)
	require.Equal(t, vaultMember{Address: addrOf(testKey("shop")), Key: shopKey}, v.Members[0])
}
