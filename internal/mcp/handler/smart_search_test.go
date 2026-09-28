package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/mcp/smart"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

func TestSmartSearchToolVisibilityAndCachedCallAccess(t *testing.T) {
	previous := model.DB
	db, err := gorm.Open(sqlite.Open("file:smart_search_handler?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	if err := db.AutoMigrate(&model.User{}, &model.SmartSearchConfig{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "member", Password: "test", Group: "vip", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	cfg := model.SmartSearchConfig{ID: 1, Enabled: true, Provider: "typesafe", ModelName: "jev-latest", APIKey: "encrypted", AllGroups: false, GroupsJSON: `["svip"]`, BatchSize: 200, Concurrency: 4}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	h := &GatewayHandler{}
	logCtx := &LogContext{UserID: user.ID, ExposeMode: "smart"}
	hasTool := func() bool {
		response := h.smartToolsResponse(1, logCtx)
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		for _, tool := range response.Result.(map[string]interface{})["tools"].([]smart.MetaTool) {
			if tool.Name == "mcp.smart_search" {
				return true
			}
		}
		return false
	}
	if hasTool() {
		t.Fatal("vip saw svip-only tool")
	}
	if response := h.handleSmartSearch(context.Background(), 1, logCtx, json.RawMessage(`{"query":"fortune"}`)); response.Error == nil {
		t.Fatal("cached call bypassed user-group restriction")
	}
	if err := db.Model(&user).Update("group", "svip").Error; err != nil {
		t.Fatal(err)
	}
	if !hasTool() {
		t.Fatal("svip could not see tool")
	}
	logCtx.ExposeMode = "direct"
	if hasTool() {
		t.Fatal("direct mode exposed smart tool")
	}
	if response := h.handleSmartSearch(context.Background(), 2, logCtx, json.RawMessage(`{"query":"fortune"}`)); response.Error == nil {
		t.Fatal("direct mode accepted smart tool")
	}
	logCtx.ExposeMode = "smart"
	if err := db.Model(&cfg).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if hasTool() {
		t.Fatal("disabled tool remained visible")
	}
	if response := h.handleSmartSearch(context.Background(), 3, logCtx, json.RawMessage(`{"query":"fortune"}`)); response.Error == nil {
		t.Fatal("disabled cached call was accepted")
	}
}

func TestSmartSearchResultFormatsProbabilities(t *testing.T) {
	noneProbability, confidence := 0.0, 0.85
	h := &GatewayHandler{}
	response := h.smartSearchResult(1, smart.SemanticRankResult{
		Matches: []smart.SemanticCandidate{
			{ToolID: "exa.search", Probability: 0.29000000000000004},
			{ToolID: "fortune.bazi", Probability: 0},
		},
		NoMatchProbability: &noneProbability, ChoiceConfidence: &confidence, FinalistCount: 3,
	}, 200)
	content := response.Result.(map[string]any)["content"].([]map[string]any)
	var body struct {
		Matches            []smart.SemanticCandidate `json:"matches"`
		NoMatch            bool                      `json:"no_match"`
		NoMatchProbability *float64                  `json:"no_match_probability"`
		ChoiceConfidence   *float64                  `json:"choice_confidence"`
		EvaluatedToolCount int                       `json:"evaluated_tool_count"`
		FinalistCount      int                       `json:"finalist_count"`
	}
	if err := json.Unmarshal([]byte(content[0]["text"].(string)), &body); err != nil {
		t.Fatal(err)
	}
	if body.NoMatch || len(body.Matches) != 1 || body.Matches[0].Probability != 0.29 || *body.NoMatchProbability != 0 || *body.ChoiceConfidence != 0.85 || body.EvaluatedToolCount != 200 || body.FinalistCount != 3 {
		t.Fatalf("unexpected response: %+v", body)
	}
}
