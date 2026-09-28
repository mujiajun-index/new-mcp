package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/systemone"
	"github.com/mujkjk/newmcp/model"
)

type SmartSearchService struct{}

func smartSearchDetail(c *model.SmartSearchConfig) *dto.SmartSearchConfigDetail {
	groups := []string{}
	_ = json.Unmarshal([]byte(c.GroupsJSON), &groups)
	return &dto.SmartSearchConfigDetail{
		Enabled: c.Enabled, Provider: c.Provider, EndpointURL: c.EndpointURL,
		ModelName: c.ModelName, HasAPIKey: c.APIKey != "", AllGroups: c.AllGroups,
		Groups: groups, BatchSize: c.BatchSize, Concurrency: c.Concurrency,
	}
}

func (s *SmartSearchService) Get() (*dto.SmartSearchConfigDetail, error) {
	c, err := model.GetSmartSearchConfig()
	if err != nil {
		return nil, err
	}
	return smartSearchDetail(c), nil
}

func validateSmartSearchInput(in *dto.SmartSearchConfigInput) error {
	if _, err := systemone.ResolveEndpoint(in.Provider, in.EndpointURL); err != nil {
		return err
	}
	if strings.TrimSpace(in.ModelName) == "" {
		return fmt.Errorf("model_name is required")
	}
	if in.BatchSize < 2 || in.BatchSize > 254 {
		return fmt.Errorf("batch_size must be between 2 and 254")
	}
	if in.Concurrency < 1 || in.Concurrency > 16 {
		return fmt.Errorf("concurrency must be between 1 and 16")
	}
	allowed := map[string]bool{}
	for _, group := range model.GetUserGroupOptions() {
		allowed[group] = true
	}
	if in.AllGroups {
		in.Groups = []string{}
	}
	seen := map[string]bool{}
	for _, group := range in.Groups {
		if !allowed[group] || seen[group] {
			return fmt.Errorf("invalid or duplicate user group: %s", group)
		}
		seen[group] = true
	}
	return nil
}

func (s *SmartSearchService) Update(actor model.Operator, in *dto.SmartSearchConfigInput) (*dto.SmartSearchConfigDetail, error) {
	if err := validateSmartSearchInput(in); err != nil {
		return nil, err
	}
	c, err := model.GetSmartSearchConfig()
	if err != nil {
		return nil, err
	}
	key := c.APIKey
	if in.APIKey != "" {
		key, err = common.Encrypt(in.APIKey)
		if err != nil {
			return nil, err
		}
	}
	if in.Enabled && key == "" {
		return nil, fmt.Errorf("api_key is required to enable smart search")
	}
	groups, _ := json.Marshal(in.Groups)
	c.Enabled, c.Provider, c.EndpointURL, c.ModelName = in.Enabled, in.Provider, strings.TrimSpace(in.EndpointURL), strings.TrimSpace(in.ModelName)
	c.APIKey, c.AllGroups, c.GroupsJSON = key, in.AllGroups, string(groups)
	c.BatchSize, c.Concurrency = in.BatchSize, in.Concurrency
	if err := model.DB.Save(c).Error; err != nil {
		return nil, err
	}
	model.RecordManageLog(actor.ID, actor.Username, "修改智能搜索设置", 0, actor, map[string]any{
		"enabled": in.Enabled, "provider": in.Provider, "model_name": in.ModelName,
		"all_groups": in.AllGroups, "groups": in.Groups, "batch_size": in.BatchSize, "concurrency": in.Concurrency,
	})
	return smartSearchDetail(c), nil
}

// SetEnabled changes only the live availability flag. Other form edits remain
// drafts until explicitly saved by the administrator.
func (s *SmartSearchService) SetEnabled(actor model.Operator, enabled bool) (*dto.SmartSearchConfigDetail, error) {
	c, err := model.GetSmartSearchConfig()
	if err != nil {
		return nil, err
	}
	if enabled {
		if c.APIKey == "" {
			return nil, fmt.Errorf("请先保存决策接口和 API 密钥")
		}
		if _, err := systemone.ResolveEndpoint(c.Provider, c.EndpointURL); err != nil {
			return nil, err
		}
		if strings.TrimSpace(c.ModelName) == "" || c.BatchSize < 2 || c.BatchSize > 254 || c.Concurrency < 1 || c.Concurrency > 16 {
			return nil, fmt.Errorf("请先保存有效的智能搜索配置")
		}
	}
	if c.Enabled == enabled {
		return smartSearchDetail(c), nil
	}
	result := model.DB.Model(&model.SmartSearchConfig{}).Where("id = ?", c.ID).Update("enabled", enabled)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, fmt.Errorf("智能搜索配置不存在，请先保存")
	}
	c.Enabled = enabled
	model.RecordManageLog(actor.ID, actor.Username, "切换智能搜索开关", 0, actor, map[string]any{"enabled": enabled})
	return smartSearchDetail(c), nil
}

func (s *SmartSearchService) Test(in *dto.SmartSearchConfigInput) ([]byte, error) {
	if err := validateSmartSearchInput(in); err != nil {
		return nil, err
	}
	key := in.APIKey
	if key == "" {
		c, err := model.GetSmartSearchConfig()
		if err != nil {
			return nil, err
		}
		if c.APIKey != "" {
			key, err = common.Decrypt(c.APIKey)
			if err != nil {
				return nil, fmt.Errorf("cannot read saved API key")
			}
		}
	}
	client, err := systemone.NewClient(in.Provider, in.EndpointURL, key, in.ModelName)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return systemone.Evaluate(ctx, client, json.RawMessage(`{"state":{"request":"I need a weather forecast"},"questions":{"tool":{"type":"choice","instructions":"Which tool fits the request?","criteria":{"weather":"Forecast the weather","calculator":"Perform arithmetic","none":"Neither tool fits"}}}}`))
}
