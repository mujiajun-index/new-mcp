package smart

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/mujkjk/newmcp/internal/mcp/systemone"
	"github.com/mujkjk/newmcp/model"
)

const noneChoice = "__none_of_the_above__"

type SemanticCandidate struct {
	ToolID             string  `json:"tool_id"`
	Service            string  `json:"service"`
	ServiceDescription string  `json:"service_description"`
	Description        string  `json:"description"`
	Probability        float64 `json:"probability"`
}

type SemanticRankResult struct {
	Matches            []SemanticCandidate
	NoMatch            bool
	NoMatchProbability *float64
	ChoiceConfidence   *float64
	FinalistCount      int
}

// SemanticCandidates uses the same first-group dedup and tool overrides as the
// gateway's direct list, but propagates database errors instead of failing open.
func SemanticCandidates(groups []model.McpGroup) ([]SemanticCandidate, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	pairs, err := model.ResolveEnabledServicesForGroups(groups)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]int64, len(groups))
	for i, g := range groups {
		groupIDs[i] = g.ID
	}
	filters, err := model.GetGroupToolsByGroupIDs(groupIDs)
	if err != nil {
		return nil, err
	}
	filterByGroup := map[int64]map[string]model.McpGroupTool{}
	for _, f := range filters {
		if filterByGroup[f.GroupID] == nil {
			filterByGroup[f.GroupID] = map[string]model.McpGroupTool{}
		}
		filterByGroup[f.GroupID][fmt.Sprintf("%d:%s", f.ServiceID, f.ToolName)] = f
	}
	seen := map[int64]bool{}
	var out []SemanticCandidate
	for _, pair := range pairs {
		svc := pair.Service
		if seen[svc.ID] {
			continue
		}
		seen[svc.ID] = true
		var tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if svc.ToolsCache == "" {
			continue
		}
		if err := json.Unmarshal([]byte(svc.ToolsCache), &tools); err != nil {
			return nil, fmt.Errorf("invalid tool cache for %s: %w", svc.Name, err)
		}
		for _, tool := range tools {
			if tool.Name == "" {
				continue
			}
			if f, ok := filterByGroup[pair.Group.ID][fmt.Sprintf("%d:%s", svc.ID, tool.Name)]; ok {
				if !f.Enabled {
					continue
				}
				if f.DescriptionOverride != "" {
					tool.Description = f.DescriptionOverride
				}
			}
			out = append(out, SemanticCandidate{ToolID: svc.Name + "." + tool.Name, Service: strings.TrimSpace(svc.DisplayName),
				ServiceDescription: svc.Description, Description: tool.Description})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ToolID < out[j].ToolID })
	return out, nil
}

type choiceResponse struct {
	Answers map[string]struct {
		Type          string             `json:"type"`
		Choice        string             `json:"choice"`
		Confidence    *float64           `json:"confidence"`
		Probabilities map[string]float64 `json:"probabilities"`
	} `json:"answers"`
}

func rankChoice(ctx context.Context, client *systemone.Client, query string, candidates []SemanticCandidate) (SemanticRankResult, error) {
	criteria := map[string]string{noneChoice: "None of these tools can satisfy the user's request."}
	for _, c := range candidates {
		criteria[c.ToolID] = fmt.Sprintf("Service: %s. Service purpose: %s. Tool purpose: %s", c.Service, c.ServiceDescription, c.Description)
	}
	req := map[string]any{
		"state": map[string]string{"user_request": query},
		"questions": map[string]any{"best": map[string]any{
			"type": "choice", "instructions": "Which one tool best fulfills user_request? Match the requested action, location, time frame, and required output to the tool's actual capabilities. Choose none of the above when no tool is suitable. Judge meaning and capabilities, not shared keywords.",
			"criteria": criteria,
		}},
	}
	args, err := json.Marshal(req)
	if err != nil {
		return SemanticRankResult{}, err
	}
	raw, err := systemone.Evaluate(ctx, client, args)
	if err != nil {
		return SemanticRankResult{}, err
	}
	var response choiceResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return SemanticRankResult{}, err
	}
	answer, ok := response.Answers["best"]
	if !ok || answer.Type != "choice" || len(answer.Probabilities) != len(criteria) {
		return SemanticRankResult{}, fmt.Errorf("decision response lacks complete choice distribution")
	}
	if _, ok := criteria[answer.Choice]; !ok {
		return SemanticRankResult{}, fmt.Errorf("decision response selected an unknown choice")
	}
	if answer.Confidence != nil && (math.IsNaN(*answer.Confidence) || math.IsInf(*answer.Confidence, 0) || *answer.Confidence < 0 || *answer.Confidence > 1) {
		return SemanticRankResult{}, fmt.Errorf("decision response has an invalid confidence")
	}
	var sum float64
	for id, p := range answer.Probabilities {
		if _, ok := criteria[id]; !ok || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return SemanticRankResult{}, fmt.Errorf("decision response has an invalid probability")
		}
		sum += p
	}
	if math.Abs(sum-1) > 0.05 {
		return SemanticRankResult{}, fmt.Errorf("decision probabilities do not sum to one")
	}
	ranked := append([]SemanticCandidate(nil), candidates...)
	for i := range ranked {
		ranked[i].Probability = answer.Probabilities[ranked[i].ToolID]
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Probability == ranked[j].Probability {
			return ranked[i].ToolID < ranked[j].ToolID
		}
		return ranked[i].Probability > ranked[j].Probability
	})
	noneProbability := answer.Probabilities[noneChoice]
	return SemanticRankResult{Matches: ranked, NoMatch: answer.Choice == noneChoice,
		NoMatchProbability: &noneProbability, ChoiceConfidence: answer.Confidence, FinalistCount: len(candidates)}, nil
}

// SemanticRank compares every accessible tool. Batch probabilities are never
// compared directly: each wave retains a shortlist for a new common Choice.
func SemanticRank(ctx context.Context, client *systemone.Client, query string, candidates []SemanticCandidate, limit, batchSize, concurrency int) (SemanticRankResult, error) {
	if len(candidates) == 0 {
		return SemanticRankResult{NoMatch: true}, nil
	}
	if limit < 1 || limit > 10 || batchSize < 2 || batchSize > 254 || concurrency < 1 || concurrency > 16 {
		return SemanticRankResult{}, fmt.Errorf("invalid semantic search limits")
	}
	for len(candidates) > batchSize {
		batchCount := (len(candidates) + batchSize - 1) / batchSize
		results := make([][]SemanticCandidate, batchCount)
		var firstErr error
		var mu sync.Mutex
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		waveCtx, cancel := context.WithCancel(ctx)
		for i := 0; i < batchCount; i++ {
			start, end := i*batchSize, min2((i+1)*batchSize, len(candidates))
			wg.Add(1)
			go func(index int, chunk []SemanticCandidate) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-waveCtx.Done():
					return
				}
				defer func() { <-sem }()
				decision, err := rankChoice(waveCtx, client, query, chunk)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					return
				}
				keep := min(len(decision.Matches), max(3, limit), batchSize-1)
				results[index] = decision.Matches[:keep]
			}(i, candidates[start:end])
		}
		wg.Wait()
		cancel()
		if firstErr != nil {
			return SemanticRankResult{}, firstErr
		}
		if err := ctx.Err(); err != nil {
			return SemanticRankResult{}, err
		}
		candidates = candidates[:0]
		for _, shortlist := range results {
			candidates = append(candidates, shortlist...)
		}
	}
	decision, err := rankChoice(ctx, client, query, candidates)
	if err != nil || decision.NoMatch {
		decision.Matches = nil
		return decision, err
	}
	decision.Matches = decision.Matches[:min2(limit, len(decision.Matches))]
	visible := decision.Matches[:0]
	for _, candidate := range decision.Matches {
		if candidate.Probability > 0 {
			candidate.ServiceDescription = singleLineDesc(candidate.ServiceDescription, searchDescMaxRunes)
			candidate.Description = singleLineDesc(candidate.Description, searchDescMaxRunes)
			visible = append(visible, candidate)
		}
	}
	decision.Matches = visible
	decision.NoMatch = len(visible) == 0
	return decision, nil
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
