package systemone

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testClient(t *testing.T, fn roundTripFunc) *Client {
	t.Helper()
	c, err := NewClient("typesafe", "https://example.test", "secret", "jev-latest")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP.Transport = fn
	return c
}
func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestEvaluatePreservesStateAndAppliesAbstention(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://example.test/v1/systemone" {
			t.Errorf("URL = %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing bearer token")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "9007199254740993") {
			t.Errorf("large JSON number changed: %s", body)
		}
		if strings.Contains(string(body), "min_confidence") {
			t.Errorf("local threshold leaked upstream: %s", body)
		}
		return jsonResponse(200, `{"model":"jev-1","answers":{"urgent":{"type":"noul","noul":0.55},"team":{"type":"choice","choice":"billing","confidence":0.3,"probabilities":{"technical":0.2,"billing":0.8}}}}`), nil
	})
	out, err := Evaluate(context.Background(), c, json.RawMessage(`{"state":{"id":9007199254740993},"questions":{"urgent":{"type":"noul","instructions":"urgent?","min_confidence":0.5},"team":{"type":"choice","instructions":"which?","criteria":{"billing":"Payments","technical":"Bugs"},"min_confidence":0.8}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Answers map[string]map[string]json.RawMessage `json:"answers"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if string(result.Answers["urgent"]["uncertain"]) != "true" || string(result.Answers["team"]["choice"]) != `"__uncertain__"` {
		t.Fatalf("unexpected abstention: %s", out)
	}
	if string(result.Answers["team"]["probabilities"]) != `{"billing":0.8,"technical":0.2}` {
		t.Fatalf("criteria order changed: %s", out)
	}
}
func TestEvaluateItemsIndependentAndPartialFailure(t *testing.T) {
	var mu sync.Mutex
	states := map[string]bool{}
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		var v struct {
			State struct {
				Item    string `json:"item"`
				Context string `json:"context"`
			} `json:"state"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			t.Error(err)
		}
		mu.Lock()
		states[v.State.Item] = v.State.Context == "policy"
		mu.Unlock()
		if v.State.Item == "bad" {
			return jsonResponse(400, `{"error":"bad item"}`), nil
		}
		return jsonResponse(200, `{"model":"jev-1","answers":{"yes":{"type":"noul","noul":0.9}},"usage":{"input_tokens":3,"output_tokens":2}}`), nil
	})
	out, err := Evaluate(context.Background(), c, json.RawMessage(`{"state":"policy","items":{"a":"good","b":"bad"},"questions":{"yes":{"type":"noul","instructions":"Is it good?"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Results map[string]json.RawMessage `json:"results"`
		Errors  map[string]string          `json:"errors"`
		Meta    struct {
			ItemCount   int `json:"item_count"`
			InputTokens int `json:"input_tokens"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || len(result.Errors) != 1 || result.Meta.ItemCount != 2 || result.Meta.InputTokens != 3 || !states["good"] || !states["bad"] {
		t.Fatalf("unexpected batch: %s, states=%v", out, states)
	}
}
func TestInvalidQuestionsRejectedBeforeCall(t *testing.T) {
	c := testClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected upstream request"); return nil, nil })
	for _, args := range []string{
		`{"state":"x","questions":{"q":{"type":"bool","instructions":"?"}}}`,
		`{"state":"x","questions":{"q":{"type":"choice","instructions":"?","criteria":[]}}}`,
		`{"state":"x","questions":{"q":{"type":"noul","instructions":"?","criteria":{"maybe":"x"}}}}`,
	} {
		if _, err := Evaluate(context.Background(), c, json.RawMessage(args)); err == nil {
			t.Errorf("accepted %s", args)
		}
	}
}
func TestEndpointResolution(t *testing.T) {
	url, err := ResolveEndpoint("typesafe", "http://127.0.0.1:8700/")
	if err != nil || url != "http://127.0.0.1:8700/v1/systemone" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	url, err = ResolveEndpoint("openrouter", "")
	if err != nil || url != DefaultRouterURL {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if _, err := ResolveEndpoint("typesafe", "not-a-url"); err == nil {
		t.Fatal("accepted relative URL")
	}
}
