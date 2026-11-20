package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// messagesStub answers POST /v1/messages with one assistant message and keeps
// the request it was sent.
func messagesStub(t *testing.T, text, stop string, delay time.Duration) (*httptest.Server, *map[string]any) {
	seen := &map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, seen)
		time.Sleep(delay)
		resp := map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"content":     []map[string]any{{"type": "text", "text": text}},
			"stop_reason": stop, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 312, "output_tokens": 95},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

const parrotText = "Twelve significant emeralds totaling 32 carats and 422 hand-set diamonds in 18kt Yellow Gold."

func TestTheDraftComesFromStructuredOutput(t *testing.T) {
	srv, seen := messagesStub(t, `{"metal":"18kt Yellow Gold","weightGrams":"","stones":[{"kind":"Emerald","carat":"32","count":12},{"kind":"Diamond","carat":"","count":422}]}`, "end_turn", 0)
	d := newClaudeDrafter("test-key", "claude-opus-5-5", srv.URL, 5*time.Second, 0)
	f, use, err := d.draft(context.Background(), "Emerald Parrot Pendant", parrotText)
	require.NoError(t, err)
	require.Equal(t, "18kt Yellow Gold", f.Metal)
	require.Equal(t, []stone{{Kind: "Emerald", Carat: "32", Count: 12}, {Kind: "Diamond", Carat: "", Count: 422}}, f.Stones)
	require.Equal(t, usage{In: 312, Out: 95}, use)

	require.Equal(t, "claude-opus-5-5", (*seen)["model"])
	oc := (*seen)["output_config"].(map[string]any)
	require.Equal(t, "low", oc["effort"])
	require.Equal(t, "json_schema", oc["format"].(map[string]any)["type"])
	msgs := (*seen)["messages"].([]any)
	require.Len(t, msgs, 1)
	content := msgs[0].(map[string]any)["content"].([]any)
	require.Len(t, content, 1, "text only: no images, prices, links or jeweler details")
	require.Equal(t, "Title: Emerald Parrot Pendant\n\nDescription:\n"+parrotText, content[0].(map[string]any)["text"])
}

func TestAnAnswerThatBreaksTheRulesIsNotADraft(t *testing.T) {
	for _, text := range []string{`not json`, `{"metal":"Gold","weightGrams":"heavy","stones":[]}`,
		`{"metal":"Gold","weightGrams":"","stones":[{"kind":"","carat":"1","count":1}]}`,
		`{"metal":"Gold","weightGrams":"","stones":[{"kind":"Ruby","carat":"1","count":10000}]}`,
		`{"metal":"Gold","weightGrams":"","stones":[],"owner":"x"}`} {
		srv, _ := messagesStub(t, text, "end_turn", 0)
		_, _, err := newClaudeDrafter("k", "claude-opus-5-5", srv.URL, 5*time.Second, 0).draft(context.Background(), "T", "D")
		require.ErrorIs(t, err, errDraftMalformed, text)
	}
	srv, _ := messagesStub(t, `{"metal":"Gold"`, "max_tokens", 0)
	_, _, err := newClaudeDrafter("k", "claude-opus-5-5", srv.URL, 5*time.Second, 0).draft(context.Background(), "T", "D")
	require.ErrorIs(t, err, errDraftMalformed)
}

func TestARefusalIsNotADraft(t *testing.T) {
	srv, _ := messagesStub(t, "", "refusal", 0)
	_, _, err := newClaudeDrafter("k", "claude-opus-5-5", srv.URL, 5*time.Second, 0).draft(context.Background(), "T", "D")
	require.ErrorIs(t, err, errDraftRefused)
}

func TestASlowAnswerTimesOut(t *testing.T) {
	srv, _ := messagesStub(t, `{}`, "end_turn", 2*time.Second)
	_, _, err := newClaudeDrafter("k", "claude-opus-5-5", srv.URL, 200*time.Millisecond, 0).draft(context.Background(), "T", "D")
	require.Error(t, err)
}

func TestADraftReadsCommasAsDecimalPoints(t *testing.T) {
	f, err := parseDraft([]byte(`{"metal":"Or 18 carats","weightGrams":"4,2","stones":[{"kind":"Saphir","carat":"0,75","count":3}]}`))
	require.NoError(t, err)
	require.Equal(t, "4.2", f.WeightGrams)
	require.Equal(t, "0.75", f.Stones[0].Carat)
}
