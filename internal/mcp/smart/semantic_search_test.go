package smart

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/mcp/systemone"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

func testChoiceClient(t *testing.T, noMatch bool, calls, largest *atomic.Int32) *systemone.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct {
			Questions map[string]struct {
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			http.Error(w, err.Error(), 400)
			return
		}
		criteria := req.Questions["best"].Criteria
		if int32(len(criteria)) > largest.Load() {
			largest.Store(int32(len(criteria)))
		}
		winner := noneChoice
		if !noMatch {
			for id := range criteria {
				if id != noneChoice && (winner == noneChoice || strings.Contains(id, "bazi")) {
					winner = id
				}
			}
		}
		probabilities := map[string]float64{}
		for id := range criteria {
			probabilities[id] = 0.1 / float64(len(criteria)-1)
		}
		probabilities[winner] = 0.9
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"best": map[string]any{
			"type": "choice", "choice": winner, "confidence": 0.9, "probabilities": probabilities,
		}}})
	}))
	t.Cleanup(server.Close)
	return &systemone.Client{URL: server.URL, APIKey: "test", Model: "jev-latest", HTTP: server.Client(), Backoff: time.Millisecond}
}

func TestSemanticRankUsesWholeCatalog(t *testing.T) {
	var calls, largest atomic.Int32
	client := testChoiceClient(t, false, &calls, &largest)
	candidates := make([]SemanticCandidate, 200)
	for i := range candidates {
		candidates[i] = SemanticCandidate{ToolID: fmt.Sprintf("svc.tool_%03d", i), Description: "ordinary tool"}
	}
	candidates[145].ToolID = "svc.bazi_chart"
	candidates[145].Description = "Compute Ba Zi birth chart and fortune"
	result, err := SemanticRank(context.Background(), client, "我要算命", candidates, 3, 200, 4)
	if err != nil || result.NoMatch {
		t.Fatalf("rank failed: %v, none=%v", err, result.NoMatch)
	}
	if calls.Load() != 1 || largest.Load() != 201 {
		t.Fatalf("expected one Choice with 201 options, got calls=%d options=%d", calls.Load(), largest.Load())
	}
	if len(result.Matches) != 3 || result.Matches[0].ToolID != "svc.bazi_chart" || result.Matches[0].Probability != 0.9 || result.FinalistCount != 200 || *result.NoMatchProbability <= 0 {
		t.Fatalf("unexpected top results: %+v", result)
	}
}

func TestSemanticRankReranksBatches(t *testing.T) {
	var calls, largest atomic.Int32
	client := testChoiceClient(t, false, &calls, &largest)
	candidates := make([]SemanticCandidate, 400)
	for i := range candidates {
		candidates[i] = SemanticCandidate{ToolID: fmt.Sprintf("svc.tool_%03d", i)}
	}
	candidates[301].ToolID = "svc.bazi_chart"
	result, err := SemanticRank(context.Background(), client, "我要算命", candidates, 3, 200, 4)
	if err != nil || result.NoMatch {
		t.Fatalf("rank failed: %v, none=%v", err, result.NoMatch)
	}
	if calls.Load() != 3 || largest.Load() != 201 || result.Matches[0].ToolID != "svc.bazi_chart" || result.FinalistCount != 6 {
		t.Fatalf("unexpected rerank: calls=%d options=%d results=%+v", calls.Load(), largest.Load(), result)
	}
}

func TestSemanticRankCanDeclineAllTools(t *testing.T) {
	var calls, largest atomic.Int32
	client := testChoiceClient(t, true, &calls, &largest)
	result, err := SemanticRank(context.Background(), client, "unrelated request", []SemanticCandidate{{ToolID: "svc.weather"}}, 3, 200, 4)
	if err != nil || !result.NoMatch || len(result.Matches) != 0 || *result.NoMatchProbability != 0.9 {
		t.Fatalf("expected no match: %+v, %v", result, err)
	}
}

func TestSemanticRankOmitsZeroProbabilityAndCompactsDescriptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"best": map[string]any{
			"type": "choice", "choice": "wttr.current", "confidence": 0.85,
			"probabilities": map[string]float64{"wttr.current": 0.85, "wttr.forecast": 0.15, "fortune.bazi": 0, noneChoice: 0},
		}}})
	}))
	t.Cleanup(server.Close)
	client := &systemone.Client{URL: server.URL, APIKey: "test", Model: "jev-latest", HTTP: server.Client(), Backoff: time.Millisecond}
	candidates := []SemanticCandidate{
		{ToolID: "wttr.current", Description: "Current weather\n and a 3-day forecast"},
		{ToolID: "wttr.forecast", Description: "Detailed\n forecast"},
		{ToolID: "fortune.bazi", Description: "Fortune telling"},
	}
	result, err := SemanticRank(context.Background(), client, "北京明天会下雨吗", candidates, 3, 200, 4)
	if err != nil {
		t.Fatal(err)
	}
	if result.NoMatch || len(result.Matches) != 2 || result.Matches[0].ToolID != "wttr.current" || result.Matches[0].Description != "Current weather and a 3-day forecast" || result.FinalistCount != 3 {
		t.Fatalf("unexpected filtered results: %+v", result)
	}
}

func TestSemanticCandidatesHonorsGroupToolDisable(t *testing.T) {
	previous := model.DB
	db, err := gorm.Open(sqlite.Open("file:semantic_candidates?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	if err := db.AutoMigrate(&model.McpGroup{}, &model.McpService{}, &model.McpGroupService{}, &model.McpGroupTool{}); err != nil {
		t.Fatal(err)
	}
	group := model.McpGroup{UserID: 1, Name: "allowed", EndpointSlug: "allowed", Status: 1}
	other := model.McpGroup{UserID: 2, Name: "other", EndpointSlug: "other", Status: 1}
	for _, g := range []*model.McpGroup{&group, &other} {
		if err := db.Create(g).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := model.McpService{UserID: 1, Name: "fortune", DisplayName: "Fortune", Status: 1, TransportType: "sse", ToolsCache: `[{"name":"bazi","description":"Ba Zi chart"},{"name":"tarot","description":"Tarot reading"}]`}
	foreign := model.McpService{UserID: 2, Name: "secret", Status: 1, TransportType: "sse", ToolsCache: `[{"name":"hidden"}]`}
	for _, s := range []*model.McpService{&svc, &foreign} {
		if err := db.Create(s).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.McpGroupService{GroupID: group.ID, ServiceID: svc.ID, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.McpGroupService{GroupID: other.ID, ServiceID: foreign.ID, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.McpGroupTool{GroupID: group.ID, ServiceID: svc.ID, ToolName: "tarot", Enabled: false}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := SemanticCandidates([]model.McpGroup{group})
	if err != nil || len(got) != 1 || got[0].ToolID != "fortune.bazi" {
		t.Fatalf("unexpected candidates: %+v, %v", got, err)
	}
}
