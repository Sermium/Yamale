package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeDrafter struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeDrafter) draft(_ context.Context, title, _ string) (fields, usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return fields{}, usage{In: 10}, f.err
	}
	return fields{Metal: "18kt gold (" + title + ")", Stones: []stone{{Kind: "Emerald", Carat: "", Count: 3}}}, usage{In: 300, Out: 90}, nil
}

func catalogueServer(t *testing.T, ps []sourceProduct, d drafter, js ...Jeweler) *server {
	s := testServer(t, js...)
	s.readSite = func(context.Context, string) ([]sourceProduct, error) { return ps, nil }
	s.drafter = d
	s.draftModel = "claude-opus-5-5"
	s.draftMonthly = 1000
	return s
}

func signedJSON(t *testing.T, method, path string, body any, k *secp256k1PrivKey, purpose, subject string) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	if subject == "" {
		subject = hashHex(b)
	}
	return sign(t, request(method, path, b), k, purpose, subject, testNow.Unix())
}

func saveRequest(t *testing.T, k *secp256k1PrivKey, signedFor design, urlID string, body map[string]any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	subject := sha256Hex(append([]byte(signedFor.Key+":"), b...))
	return sign(t, request("PUT", "/v1/catalogue/designs/"+urlID, b), k, "design-save", subject, testNow.Unix())
}

func startImport(t *testing.T, s *server, k *secp256k1PrivKey, origin string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"origin": origin})
	return serve(s, sign(t, request("POST", "/v1/catalogue/import", b), k, "catalogue-import", hashHex([]byte(origin)), testNow.Unix()))
}

func waitForJob(t *testing.T, s *server, id string) catalogue {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.catMu.Lock()
		c, err := s.loadCatalogue(id)
		s.catMu.Unlock()
		require.NoError(t, err)
		if c.Job.State == jobDone || c.Job.State == jobFailed {
			return c
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the import did not finish")
	return catalogue{}
}

func readAll(t *testing.T, s *server, k *secp256k1PrivKey, jeweler string) catalogue {
	w := serve(s, sign(t, request("GET", "/v1/catalogue", nil), k, "catalogue-read", jeweler, testNow.Unix()))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var c catalogue
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c))
	return c
}

func twoProducts() []sourceProduct {
	return []sourceProduct{product("JOA-071", "parrot", "Emerald Parrot Pendant"), product("", "emerald-ring-1", "Emerald Ring")}
}

func TestStaffImportTheirSiteAndClaudeDraftsEachDesign(t *testing.T) {
	biz, staff := testKey("biz"), testKey("staff")
	d := &fakeDrafter{}
	s := catalogueServer(t, twoProducts(), d, activeJeweler("1", biz, staff))
	w := startImport(t, s, staff, "https://www.jewelofafrica.com")
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	c := waitForJob(t, s, "1")
	require.Equal(t, jobDone, c.Job.State)
	require.Equal(t, 2, c.Job.Of)
	require.Equal(t, 2, d.calls)

	got := readAll(t, s, biz, "1")
	require.Equal(t, "https://www.jewelofafrica.com", got.Origin)
	require.Len(t, got.Designs, 2)
	for _, x := range got.Designs {
		require.Equal(t, statusDrafted, x.Status)
		require.Equal(t, x.SourceHash, x.DraftFor)
		require.Contains(t, x.Fields.Metal, "18kt gold")
	}
}

func TestStrangersCannotImportOrRead(t *testing.T) {
	biz, stranger := testKey("biz"), testKey("stranger")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	require.Equal(t, http.StatusForbidden, startImport(t, s, stranger, "https://shop.example").Code)
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("GET", "/v1/catalogue", nil), stranger, "catalogue-read", "1", testNow.Unix())).Code)
}

func TestTheImportSignatureCoversTheOrigin(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	b, _ := json.Marshal(map[string]string{"origin": "https://shop.example"})
	w := serve(s, sign(t, request("POST", "/v1/catalogue/import", b), biz, "catalogue-import", hashHex([]byte("https://other.example")), testNow.Unix()))
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOnlyAPublicHTTPSOriginIsImported(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	for _, o := range []string{"http://shop.example", "https://shop.example/products"} {
		w := startImport(t, s, biz, o)
		require.Equal(t, http.StatusBadRequest, w.Code, o)
		require.Contains(t, w.Body.String(), `"code":"refused"`)
	}
}

func TestOneImportRunsAtATime(t *testing.T) {
	biz := testKey("biz")
	release := make(chan struct{})
	s := catalogueServer(t, nil, &fakeDrafter{}, activeJeweler("1", biz))
	s.readSite = func(context.Context, string) ([]sourceProduct, error) { <-release; return twoProducts(), nil }
	require.Equal(t, http.StatusAccepted, startImport(t, s, biz, "https://shop.example").Code)
	w := startImport(t, s, biz, "https://shop.example")
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), `"code":"busy"`)
	close(release)
	waitForJob(t, s, "1")
}

func TestAnUnreadableSiteFailsTheJobWithAReason(t *testing.T) {
	biz := testKey("biz")
	for err, code := range map[error]string{errUnreadable: "unreadable", errRobots: "robots", errRateLimited: "rateLimited", errNotPublic: "refused", errors.New("x"): "unreachable"} {
		s := catalogueServer(t, nil, &fakeDrafter{}, activeJeweler("1", biz))
		s.readSite = func(context.Context, string) ([]sourceProduct, error) { return nil, err }
		require.Equal(t, http.StatusAccepted, startImport(t, s, biz, "https://shop.example").Code)
		c := waitForJob(t, s, "1")
		require.Equal(t, jobFailed, c.Job.State)
		require.Equal(t, code, c.Job.Error)
		require.Empty(t, c.Designs, "nothing imported")
	}
}

func TestAFailedDraftLeavesTheDesignImported(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{err: errDraftMalformed}, activeJeweler("1", biz))
	startImport(t, s, biz, "https://shop.example")
	c := waitForJob(t, s, "1")
	require.Equal(t, jobDone, c.Job.State)
	for _, d := range c.Designs {
		require.Equal(t, statusImported, d.Status)
		require.Equal(t, "unavailable", d.DraftNote)
	}
	s2 := catalogueServer(t, twoProducts(), nil, activeJeweler("1", biz))
	startImport(t, s2, biz, "https://shop.example")
	for _, d := range waitForJob(t, s2, "1").Designs {
		require.Equal(t, "unavailable", d.DraftNote, "no key configured: drafting is off, importing is not")
	}
}

func TestTheMonthlyCapPausesDraftingNotImporting(t *testing.T) {
	biz := testKey("biz")
	d := &fakeDrafter{}
	s := catalogueServer(t, append(twoProducts(), product("X3", "x3", "Third")), d, activeJeweler("1", biz))
	s.draftMonthly = 1
	startImport(t, s, biz, "https://shop.example")
	c := waitForJob(t, s, "1")
	require.Equal(t, jobDone, c.Job.State)
	require.Len(t, c.Designs, 3)
	require.Equal(t, 1, d.calls)
	paused := 0
	for _, x := range c.Designs {
		if x.DraftNote == "paused" {
			paused++
			require.Equal(t, statusImported, x.Status)
		}
	}
	require.Equal(t, 2, paused)
}

func TestEveryDraftCallIsLoggedWithoutItsText(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	startImport(t, s, biz, "https://shop.example")
	waitForJob(t, s, "1")
	log, err := os.ReadFile(filepath.Join(s.dir, "catalogue", "drafting.log"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	require.Len(t, lines, 2)
	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	require.Equal(t, "1", entry["jeweler"])
	require.Equal(t, "claude-opus-5-5", entry["model"])
	require.Equal(t, "ok", entry["outcome"])
	require.EqualValues(t, 300, entry["inputTokens"])
	require.EqualValues(t, 90, entry["outputTokens"])
	require.NotContains(t, string(log), "18kt gold", "the log records the call, never the text")
}

func importedDesign(t *testing.T, s *server, k *secp256k1PrivKey) design {
	startImport(t, s, k, "https://shop.example")
	c := waitForJob(t, s, "1")
	for _, d := range c.Designs {
		if d.Key == "JOA-071" {
			return d
		}
	}
	t.Fatal("no JOA-071")
	return design{}
}

func storePhoto(t *testing.T, s *server, k *secp256k1PrivKey) string {
	w := serve(s, sign(t, request("POST", "/v1/files", pngBytes), k, "file", hashHex(pngBytes), testNow.Unix()))
	require.Equal(t, http.StatusCreated, w.Code)
	return hashHex(pngBytes)
}

func confirmRequest(t *testing.T, k *secp256k1PrivKey, d design, body map[string]any) *http.Request {
	b, err := json.Marshal(body)
	require.NoError(t, err)
	subject := sha256Hex(append([]byte(d.Key+":"+body["sourceHash"].(string)+":"), b...))
	return sign(t, request("POST", "/v1/catalogue/designs/"+d.ID+"/confirm", b), k, "design-confirm", subject, testNow.Unix())
}

func goodConfirmation(d design, photo string) map[string]any {
	return map[string]any{"sourceHash": d.SourceHash, "title": "Emerald Parrot Pendant",
		"fields":          fields{Metal: "18kt Yellow Gold", Stones: []stone{{Kind: "Emerald", Carat: "32", Count: 12}}},
		"photos":          []string{photo}, "rightsConfirmed": true}
}

func TestADesignIsConfirmedWithCompleteFieldsPhotosAndTheTick(t *testing.T) {
	biz, staff := testKey("biz"), testKey("staff")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz, staff))
	d := importedDesign(t, s, staff)
	photo := storePhoto(t, s, staff)

	noTick := goodConfirmation(d, photo)
	noTick["rightsConfirmed"] = false
	require.Equal(t, http.StatusBadRequest, serve(s, confirmRequest(t, staff, d, noTick)).Code)
	noCarat := goodConfirmation(d, photo)
	noCarat["fields"] = fields{Metal: "Gold", Stones: []stone{{Kind: "Diamond", Carat: "", Count: 422}}}
	require.Equal(t, http.StatusBadRequest, serve(s, confirmRequest(t, staff, d, noCarat)).Code)
	unknownPhoto := goodConfirmation(d, photo)
	unknownPhoto["photos"] = []string{strings.Repeat("b", 64)}
	require.Equal(t, http.StatusBadRequest, serve(s, confirmRequest(t, staff, d, unknownPhoto)).Code)

	w := serve(s, confirmRequest(t, staff, d, goodConfirmation(d, photo)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got design
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, statusConfirmed, got.Status)
	require.Equal(t, addrOf(staff), got.Confirmed.By)
	require.Equal(t, []string{photo}, got.Confirmed.Photos)
	require.Equal(t, d.SourceHash, got.Confirmed.SourceHash)
}

func TestAStaleConfirmationIsRefused(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	d := importedDesign(t, s, biz)
	photo := storePhoto(t, s, biz)
	body := goodConfirmation(d, photo)
	body["sourceHash"] = strings.Repeat("0", 64)
	w := serve(s, confirmRequest(t, biz, d, body))
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), `"code":"stale"`)
}

func TestSavingAConfirmedDesignNeedsConfirmingAgain(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	d := importedDesign(t, s, biz)
	require.Equal(t, http.StatusOK, serve(s, confirmRequest(t, biz, d, goodConfirmation(d, storePhoto(t, s, biz)))).Code)
	edit := map[string]any{"title": "Parrot", "fields": fields{Metal: "Platinum", Stones: []stone{}}}
	w := serve(s, saveRequest(t, biz, d, d.ID, edit))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got design
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, statusDrafted, got.Status)
	require.Equal(t, "Platinum", got.Fields.Metal)
}

func TestAGoneDesignCannotBeSavedOrConfirmed(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	d := importedDesign(t, s, biz)
	s.readSite = func(context.Context, string) ([]sourceProduct, error) { return twoProducts()[1:], nil }
	startImport(t, s, biz, "https://shop.example")
	waitForJob(t, s, "1")
	edit := map[string]any{"title": "Parrot", "fields": fields{Metal: "Gold", Stones: []stone{}}}
	w := serve(s, saveRequest(t, biz, d, d.ID, edit))
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), `"code":"gone"`)
}

func TestTheImageProxyServesOnlyADesignsOwnImages(t *testing.T) {
	biz := testKey("biz")
	img := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Write(pngBytes)
	}))
	defer img.Close()
	p := product("JOA-071", "parrot", "Parrot")
	p.Images = []string{img.URL + "/a.png"}
	s := catalogueServer(t, []sourceProduct{p}, &fakeDrafter{}, activeJeweler("1", biz))
	f, _ := trusting(t, img, portOf(t, img))
	s.fetch = f
	d := importedDesign(t, s, biz)
	w := serve(s, sign(t, request("GET", "/v1/catalogue/designs/"+d.ID+"/images/0", nil), biz, "catalogue-read", "1", testNow.Unix()))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
	require.True(t, bytes.Equal(pngBytes, w.Body.Bytes()))
	require.Equal(t, http.StatusNotFound, serve(s, sign(t, request("GET", "/v1/catalogue/designs/"+d.ID+"/images/1", nil), biz, "catalogue-read", "1", testNow.Unix())).Code)
}

func TestASaveSignatureDoesNotReplayOntoAnotherDesign(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	startImport(t, s, biz, "https://shop.example")
	c := waitForJob(t, s, "1")
	require.Len(t, c.Designs, 2)
	a, other := c.Designs[0], c.Designs[1]
	edit := map[string]any{"title": "Parrot", "fields": fields{Metal: "Gold", Stones: []stone{}}}
	require.Equal(t, http.StatusUnauthorized, serve(s, saveRequest(t, biz, a, other.ID, edit)).Code)
	require.Equal(t, http.StatusOK, serve(s, saveRequest(t, biz, other, other.ID, edit)).Code)
	require.Equal(t, http.StatusNotFound, serve(s, saveRequest(t, biz, a, "0123456789abcdef", edit)).Code)
}

func TestAFailedReadNeverShowsAServerPath(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	d := importedDesign(t, s, biz)
	photo := storePhoto(t, s, biz)
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "catalogue", "1.json"), []byte("{not json"), 0o640))
	w := serve(s, confirmRequest(t, biz, d, goodConfirmation(d, photo)))
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), s.dir)
	edit := map[string]any{"title": "Parrot", "fields": fields{Metal: "Gold", Stones: []stone{}}}
	w = serve(s, saveRequest(t, biz, d, d.ID, edit))
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), s.dir)
}

func TestListsAreAlwaysLists(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &nilStonesDrafter{}, activeJeweler("1", biz))
	d := importedDesign(t, s, biz)
	w := serve(s, sign(t, request("GET", "/v1/catalogue", nil), biz, "catalogue-read", "1", testNow.Unix()))
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), `"stones":null`)

	b := []byte(`{"title":"Parrot","fields":{"metal":"Gold"}}`)
	r := sign(t, request("PUT", "/v1/catalogue/designs/"+d.ID, b), biz, "design-save", sha256Hex(append([]byte(d.Key+":"), b...)), testNow.Unix())
	w = serve(s, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"stones":[]`)
	require.NotContains(t, w.Body.String(), `"stones":null`)

	cb := goodConfirmation(d, storePhoto(t, s, biz))
	cb["fields"] = map[string]any{"metal": "Gold"}
	w = serve(s, confirmRequest(t, biz, d, cb))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), `"stones":null`)
	require.Contains(t, w.Body.String(), `"stones":[]`)
}

type nilStonesDrafter struct{}

func (nilStonesDrafter) draft(context.Context, string, string) (fields, usage, error) {
	return fields{Metal: "Gold"}, usage{In: 1, Out: 1}, nil
}

func TestAJobCutShortByARestartIsReportedAsInterrupted(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	s.catMu.Lock()
	require.NoError(t, s.saveCatalogue("1", catalogue{Job: importJob{State: jobDrafting}}))
	s.importing = map[string]bool{}
	s.catMu.Unlock()
	c := readAll(t, s, biz, "1")
	require.Equal(t, jobFailed, c.Job.State)
	require.Equal(t, "interrupted", c.Job.Error)
	require.Equal(t, http.StatusAccepted, startImport(t, s, biz, "https://shop.example").Code)
	waitForJob(t, s, "1")
}

func TestARestartedJobDoesNotBlockANewImport(t *testing.T) {
	biz := testKey("biz")
	s := catalogueServer(t, twoProducts(), &fakeDrafter{}, activeJeweler("1", biz))
	s.catMu.Lock()
	require.NoError(t, s.saveCatalogue("1", catalogue{Job: importJob{State: jobReading}}))
	s.catMu.Unlock()
	require.Equal(t, http.StatusAccepted, startImport(t, s, biz, "https://shop.example").Code)
	waitForJob(t, s, "1")
}
