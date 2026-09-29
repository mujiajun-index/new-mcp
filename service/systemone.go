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
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

type SystemOneService struct{}

func systemOneDetail(c *model.SystemOneConfig) dto.SystemOneConfigDetail {
	u, _ := systemone.ResolveEndpoint(c.Provider, c.EndpointURL)
	return dto.SystemOneConfigDetail{ID: c.ID, Name: c.Name, Description: c.Description,
		Provider: c.Provider, EndpointURL: c.EndpointURL, ResolvedURL: u,
		ModelName: c.ModelName, HasAPIKey: c.APIKey != "", AutoRegister: c.AutoRegister,
		RegisteredServiceID: c.RegisteredServiceID,
		CreatedAt:           c.CreatedAt.Format(time.RFC3339), UpdatedAt: c.UpdatedAt.Format(time.RFC3339)}
}

func (s *SystemOneService) List(userID int64) ([]dto.SystemOneConfigDetail, error) {
	rows, err := model.ListSystemOneConfigs(userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SystemOneConfigDetail, len(rows))
	for i := range rows {
		out[i] = systemOneDetail(&rows[i])
	}
	return out, nil
}

func (s *SystemOneService) Get(userID, id int64) (*dto.SystemOneConfigDetail, error) {
	c, err := model.GetSystemOneConfig(userID, id)
	if err != nil {
		return nil, err
	}
	d := systemOneDetail(c)
	return &d, nil
}

func normalizeSystemOne(req *dto.SystemOneConfigReq, requireKey bool) error {
	req.Name = strings.TrimSpace(req.Name)
	req.EndpointURL = strings.TrimSpace(req.EndpointURL)
	req.ModelName = strings.TrimSpace(req.ModelName)
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if _, err := systemone.ResolveEndpoint(req.Provider, req.EndpointURL); err != nil {
		return err
	}
	if req.ModelName == "" {
		req.ModelName = systemone.DefaultModel(req.Provider)
	}
	if requireKey && strings.TrimSpace(req.APIKey) == "" {
		return fmt.Errorf("api_key is required")
	}
	return nil
}

func (s *SystemOneService) Create(userID int64, req *dto.SystemOneConfigReq) (*dto.SystemOneConfigDetail, error) {
	if err := normalizeSystemOne(req, true); err != nil {
		return nil, err
	}
	key, err := common.Encrypt(req.APIKey)
	if err != nil {
		return nil, err
	}
	c := &model.SystemOneConfig{UserID: userID, Name: req.Name, Description: req.Description,
		Provider: req.Provider, EndpointURL: req.EndpointURL, ModelName: req.ModelName, APIKey: key}
	if err := model.DB.Create(c).Error; err != nil {
		return nil, err
	}
	d := systemOneDetail(c)
	return &d, nil
}

func (s *SystemOneService) Update(userID, id int64, req *dto.SystemOneConfigReq) error {
	if err := normalizeSystemOne(req, false); err != nil {
		return err
	}
	c, err := model.GetSystemOneConfig(userID, id)
	if err != nil {
		return err
	}
	c.Name, c.Description, c.Provider = req.Name, req.Description, req.Provider
	c.EndpointURL, c.ModelName = req.EndpointURL, req.ModelName
	if req.APIKey != "" {
		c.APIKey, err = common.Encrypt(req.APIKey)
		if err != nil {
			return err
		}
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(c).Error; err != nil {
			return err
		}
		if c.RegisteredServiceID != nil {
			return tx.Model(&model.McpService{}).Where("id = ? AND user_id = ?", *c.RegisteredServiceID, userID).
				Updates(map[string]interface{}{"display_name": c.Name, "description": c.Description}).Error
		}
		return nil
	})
}

func systemOneTools() string {
	tool := []map[string]interface{}{{
		"name":        "evaluate",
		"description": "Evaluate observed state using a System One decision model (Jev, CLM or Laya). You MUST provide questions and at least one of: non-null state or a non-empty items object. Both may be provided: state then supplies shared context for each item. Returns typed answers and actual model probabilities, not prose. Ask narrow questions, provide evidence rather than your conclusion. noul is yes/no probability; choice selects among named options; score rates ordered levels. Use items for independent records (up to 500); min_confidence can abstain for noul or choice. Include a no-match choice when appropriate. Confidence measures probability concentration, not guaranteed correctness.",
		"inputSchema": map[string]interface{}{
			"type": "object", "required": []string{"questions"},
			"anyOf": []map[string]interface{}{
				{"required": []string{"state"}, "properties": map[string]interface{}{
					"state": map[string]interface{}{"not": map[string]interface{}{"type": "null"}},
				}},
				{"required": []string{"items"}},
			},
			"properties": map[string]interface{}{
				"state": map[string]interface{}{"description": "Observed evidence and relevant background as text or structured JSON. You MUST provide non-null state unless a non-empty items object is supplied. When both are provided, state is shared context for each item."},
				"questions": map[string]interface{}{"type": "object", "description": "Question ID to typed judgment; IDs are not sent to the model, so instructions must be self-contained.",
					"additionalProperties": map[string]interface{}{"type": "object", "required": []string{"type", "instructions"}, "properties": map[string]interface{}{
						"type":           map[string]interface{}{"type": "string", "enum": []string{"noul", "choice", "score"}},
						"instructions":   map[string]interface{}{"description": "A full, narrow question; text or structured definitions/examples."},
						"criteria":       map[string]interface{}{"description": "noul: optional true/false map; choice: option-to-description map; score: ordered array of levels."},
						"min_confidence": map[string]interface{}{"type": "number", "minimum": 0, "maximum": 1, "description": "For noul/choice, mark low-confidence answers uncertain; choice becomes __uncertain__."},
					}}},
				"items":              map[string]interface{}{"type": "object", "minProperties": 1, "maxProperties": 500, "description": "Item ID to record map; each item is evaluated independently. You MUST provide items unless non-null state is supplied. If provided, items must contain 1 to 500 entries, even when state is also provided. State supplies shared context when both are provided."},
				"model":              map[string]interface{}{"type": "string", "description": "Optional per-call model override."},
				"include_item_usage": map[string]interface{}{"type": "boolean", "description": "Keep model and usage in each item response."},
			},
		},
	}}
	b, _ := json.Marshal(tool)
	return string(b)
}

// Refresh the cached schema after upgrades, including configurations created
// before a tool-description change.
func (s *SystemOneService) SyncAllRegisteredTools() int {
	var configs []model.SystemOneConfig
	if err := model.DB.Where("auto_register = ? AND registered_service_id IS NOT NULL", true).Find(&configs).Error; err != nil {
		return 0
	}
	now := time.Now()
	n := 0
	for _, c := range configs {
		if err := model.DB.Model(&model.McpService{}).Where("id = ? AND user_id = ? AND source = ?", *c.RegisteredServiceID, c.UserID, "systemone").
			Updates(map[string]interface{}{"tools_cache": systemOneTools(), "tools_updated_at": &now}).Error; err == nil {
			n++
		}
	}
	return n
}

func (s *SystemOneService) Enable(userID, id int64) error {
	c, err := model.GetSystemOneConfig(userID, id)
	if err != nil {
		return err
	}
	if c.AutoRegister && c.RegisteredServiceID != nil {
		return nil
	}
	if _, err := systemone.ResolveEndpoint(c.Provider, c.EndpointURL); err != nil {
		return err
	}
	now := time.Now()
	name := fmt.Sprintf("systemone_%d", c.ID)
	svc := &model.McpService{UserID: userID, Name: name, DisplayName: c.Name,
		Description: c.Description, TransportType: "virtual", Source: "systemone",
		Config:     fmt.Sprintf(`{"virtual_type":"systemone","ref_id":%d}`, c.ID),
		ToolsCache: systemOneTools(), ToolsUpdatedAt: &now, HealthStatus: "healthy", Status: common.StatusEnabled}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(svc).Error; err != nil {
			return err
		}
		return tx.Model(c).Updates(map[string]interface{}{"auto_register": true, "registered_service_id": svc.ID}).Error
	})
	if err != nil {
		return err
	}
	if VirtualRegistry != nil {
		VirtualRegistry.Register(svc.ID, userID, name, virtual.ParseConfig(svc.Config), virtual.SystemOneHandler)
	}
	return nil
}

func (s *SystemOneService) Disable(userID, id int64) error {
	c, err := model.GetSystemOneConfig(userID, id)
	if err != nil {
		return err
	}
	if c.RegisteredServiceID == nil {
		return nil
	}
	serviceID := *c.RegisteredServiceID
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("service_id = ?", serviceID).Delete(&model.McpGroupTool{}).Error; err != nil {
			return err
		}
		if err := tx.Where("service_id = ?", serviceID).Delete(&model.McpGroupService{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ? AND source = ?", serviceID, userID, "systemone").Delete(&model.McpService{}).Error; err != nil {
			return err
		}
		return tx.Model(c).Updates(map[string]interface{}{"auto_register": false, "registered_service_id": nil}).Error
	})
	if err != nil {
		return err
	}
	if VirtualRegistry != nil {
		VirtualRegistry.Unregister(serviceID)
	}
	return nil
}

func (s *SystemOneService) Delete(userID, id int64) error {
	if err := s.Disable(userID, id); err != nil {
		return err
	}
	return model.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.SystemOneConfig{}).Error
}

func (s *SystemOneService) Test(userID int64, req *dto.TestSystemOneReq) *dto.TestSystemOneResult {
	if req.ConfigID > 0 {
		c, err := model.GetSystemOneConfig(userID, req.ConfigID)
		if err != nil {
			return &dto.TestSystemOneResult{Error: "配置不存在"}
		}
		// The browser sends all editable fields, including an intentionally empty
		// endpoint (meaning the provider default). Only a config-ID-only request
		// should inherit the saved endpoint; the saved key is always reusable.
		useSavedFields := req.Provider == "" && req.EndpointURL == "" && req.ModelName == ""
		if req.Provider == "" {
			req.Provider = c.Provider
		}
		if useSavedFields {
			req.EndpointURL = c.EndpointURL
		}
		if req.ModelName == "" {
			req.ModelName = c.ModelName
		}
		if req.APIKey == "" {
			req.APIKey, err = common.Decrypt(c.APIKey)
			if err != nil {
				return &dto.TestSystemOneResult{Error: "无法读取 API 密钥"}
			}
		}
	}
	client, err := systemone.NewClient(req.Provider, req.EndpointURL, req.APIKey, req.ModelName)
	if err != nil {
		return &dto.TestSystemOneResult{Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	answer, err := systemone.Evaluate(ctx, client, json.RawMessage(`{"state":"A customer reports a failed payment.","questions":{"mentions_payment":{"type":"noul","instructions":"Does the state mention a payment problem?"}}}`))
	if err != nil {
		return &dto.TestSystemOneResult{Error: err.Error()}
	}
	return &dto.TestSystemOneResult{Success: true, Result: string(answer)}
}
