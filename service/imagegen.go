package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ImageGenService struct{}

var imageGenConfigMu sync.Mutex

type ImageGenDetail struct {
	model.ImageGenConfig
	DefaultParams            json.RawMessage `json:"default_params"`
	HasAPIKey                bool            `json:"has_api_key"`
	GenerateImageDescDefault string          `json:"generate_image_desc_default"`
	EditImageDescDefault     string          `json:"edit_image_desc_default"`
	UploadImageDesc          string          `json:"upload_image_desc"`
}

// Recognize only the previous built-in descriptions; user-authored text stays intact.
const (
	legacyGenerateImageDesc = "Generate images from a text prompt with the configured image model. Returns downloadable image URLs and metadata. Use return_mode=inline to also return native MCP image content when it fits the response limit."
	legacyEditImageDesc     = "Edit or generate images from reference images and a prompt using the configured image model. Pass images as URLs (use upload_image first for local files) or small base64 images. An optional PNG mask selects areas to edit on supported OpenAI-compatible models. Returns image URLs and metadata."
	legacyEditImageDescV2   = "Edit images or create variations from reference images with an image generation model. Covers image transformations: add or remove objects, replace backgrounds, change styles or colors, and combine references, as supported by the configured model. Pass prompt to describe the requested changes and images with one or more HTTP(S) URLs, data URLs, or small base64 images. For LOCAL files, call upload_image and run its returned upload_command first. An optional PNG mask must match the first image's dimensions; transparent areas are edited on supported OpenAI-compatible models. Gemini does not support masks. Returns: downloadable image URLs and metadata. Set return_mode=inline to also include native MCP image content when size limits allow."
)

func applyImageGenDefaultDescriptions(c *model.ImageGenConfig) {
	if strings.TrimSpace(c.GenerateImageDesc) == "" || c.GenerateImageDesc == legacyGenerateImageDesc {
		c.GenerateImageDesc = model.DefaultGenerateImageDesc
	}
	if strings.TrimSpace(c.EditImageDesc) == "" || c.EditImageDesc == legacyEditImageDesc || c.EditImageDesc == legacyEditImageDescV2 {
		c.EditImageDesc = model.DefaultEditImageDesc
	}
}

func imageGenDetail(c *model.ImageGenConfig) *ImageGenDetail {
	config := *c
	if config.EditRequestFormat == "" {
		config.EditRequestFormat = imagegen.EditFormatMultipart
	}
	applyImageGenDefaultDescriptions(&config)
	return &ImageGenDetail{
		ImageGenConfig:           config,
		DefaultParams:            json.RawMessage(c.DefaultParams),
		HasAPIKey:                c.ApiKey != "",
		GenerateImageDescDefault: model.DefaultGenerateImageDesc,
		EditImageDescDefault:     model.DefaultEditImageDesc,
		UploadImageDesc:          model.DefaultImageGenUploadDesc,
	}
}
func (s *ImageGenService) List(uid int64) ([]*ImageGenDetail, error) {
	var rows []model.ImageGenConfig
	if err := model.DB.Where("user_id = ?", uid).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*ImageGenDetail, 0, len(rows))
	for i := range rows {
		out = append(out, imageGenDetail(&rows[i]))
	}
	return out, nil
}
func (s *ImageGenService) Get(uid, id int64) (*ImageGenDetail, error) {
	c, err := model.GetImageGenConfig(uid, id)
	if err != nil {
		return nil, err
	}
	return imageGenDetail(c), nil
}

func applyImageGenConnection(c *model.ImageGenConfig, r *dto.ImageGenConfigReq) error {
	if r.Provider != nil {
		c.Provider = strings.TrimSpace(*r.Provider)
	}
	if r.EndpointURL != nil {
		c.EndpointURL = strings.TrimSpace(*r.EndpointURL)
	}
	if r.ApiKey != nil && strings.TrimSpace(*r.ApiKey) != "" {
		c.ApiKey = strings.TrimSpace(*r.ApiKey)
	}
	if c.EndpointURL == "" {
		c.EndpointURL = imagegen.DefaultEndpoint(c.Provider)
	}
	if len(c.EndpointURL) > 512 || len(c.ApiKey) > 512 {
		return fmt.Errorf("endpoint or API key is too long")
	}
	if _, err := imagegen.BaseURL(c.EndpointURL, c.Provider); err != nil {
		return err
	}
	if c.ApiKey == "" {
		return fmt.Errorf("API key is required")
	}
	return nil
}

func applyImageGenReq(c *model.ImageGenConfig, r *dto.ImageGenConfigReq) error {
	if err := applyImageGenConnection(c, r); err != nil {
		return err
	}
	if r.Name != nil {
		c.Name = strings.TrimSpace(*r.Name)
	}
	if r.Description != nil {
		c.Description = *r.Description
	}
	if r.ModelName != nil {
		c.ModelName = strings.TrimSpace(*r.ModelName)
	}
	if r.Operation != nil {
		c.Operation = *r.Operation
	}
	if r.EditRequestFormat != nil {
		c.EditRequestFormat = *r.EditRequestFormat
	}
	if c.EditRequestFormat == "" {
		c.EditRequestFormat = imagegen.EditFormatMultipart
		// Apply the new default only after the provider is known. Persisted
		// configurations with no format retain the legacy multipart behavior.
		if c.ID == 0 && c.Provider == "openai" {
			c.EditRequestFormat = imagegen.EditFormatGenerationsJSON
		}
	}
	if r.TimeoutSeconds != nil {
		c.TimeoutSeconds = *r.TimeoutSeconds
	}
	if r.DefaultParams != nil {
		c.DefaultParams = string(*r.DefaultParams)
	}
	if r.GenerateImageDesc != nil {
		c.GenerateImageDesc = *r.GenerateImageDesc
	}
	if r.EditImageDesc != nil {
		c.EditImageDesc = *r.EditImageDesc
	}
	if c.Name == "" || len(c.Name) > 128 {
		return fmt.Errorf("name must contain 1 to 128 bytes")
	}
	if c.ModelName == "" || len(c.ModelName) > 128 {
		return fmt.Errorf("model_name is required (max 128 bytes)")
	}
	if err := imagegen.ValidateEditRequestFormat(c.Provider, c.EditRequestFormat); err != nil {
		return err
	}
	if c.Operation != "generate" && c.Operation != "edit" && c.Operation != "both" {
		return fmt.Errorf("operation must be generate, edit or both")
	}
	if c.TimeoutSeconds < 0 || c.TimeoutSeconds > 86400 {
		return fmt.Errorf("timeout_seconds must be between 0 and 86400")
	}
	if len(c.DefaultParams) > 65536 {
		return fmt.Errorf("default_params exceeds 64 KiB")
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(c.DefaultParams), &params); err != nil || params == nil {
		return fmt.Errorf("default_params must be a JSON object")
	}
	if err := imagegen.ValidateParams(c.Provider, params, true); err != nil {
		return err
	}
	if len(c.GenerateImageDesc) > 4096 || len(c.EditImageDesc) > 4096 {
		return fmt.Errorf("tool descriptions must not exceed 4096 bytes")
	}
	applyImageGenDefaultDescriptions(c)
	return nil
}
func newImageGenConfig(uid int64) *model.ImageGenConfig {
	return &model.ImageGenConfig{UserID: uid, Provider: "openai", Operation: "both", DefaultParams: "{}", TimeoutSeconds: 180, Status: common.StatusEnabled, GenerateImageDesc: model.DefaultGenerateImageDesc, EditImageDesc: model.DefaultEditImageDesc}
}
func (s *ImageGenService) Create(uid int64, r *dto.ImageGenConfigReq) (*ImageGenDetail, error) {
	c := newImageGenConfig(uid)
	if err := applyImageGenReq(c, r); err != nil {
		return nil, err
	}
	if err := model.DB.Create(c).Error; err != nil {
		return nil, err
	}
	return imageGenDetail(c), nil
}
func (s *ImageGenService) Update(uid, id int64, r *dto.ImageGenConfigReq) error {
	imageGenConfigMu.Lock()
	defer imageGenConfigMu.Unlock()
	var cfg model.ImageGenConfig
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, uid).First(&cfg).Error; e != nil {
			return e
		}
		if e := applyImageGenReq(&cfg, r); e != nil {
			return e
		}
		if e := tx.Save(&cfg).Error; e != nil {
			return e
		}
		if cfg.RegisteredServiceID != nil {
			return syncImageGenService(tx, &cfg)
		}
		return nil
	})
	if err == nil && cfg.RegisteredServiceID != nil {
		s.register(&cfg)
	}
	return err
}
func syncImageGenService(tx *gorm.DB, c *model.ImageGenConfig) error {
	tools, _ := json.Marshal(buildImageGenTools(c))
	now := time.Now()
	res := tx.Model(&model.McpService{}).Where("id = ? AND user_id = ? AND source = ?", *c.RegisteredServiceID, c.UserID, "imagegen").Updates(map[string]any{"display_name": c.Name, "description": c.Description, "tools_cache": string(tools), "tools_updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var n int64
		tx.Model(&model.McpService{}).Where("id = ? AND user_id = ? AND source = ?", *c.RegisteredServiceID, c.UserID, "imagegen").Count(&n)
		if n == 0 {
			return fmt.Errorf("registered image service is missing")
		}
	}
	return nil
}
func (s *ImageGenService) Enable(uid, id int64) error {
	imageGenConfigMu.Lock()
	defer imageGenConfigMu.Unlock()
	var cfg model.ImageGenConfig
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, uid).First(&cfg).Error; e != nil {
			return e
		}
		if cfg.AutoRegister && cfg.RegisteredServiceID != nil {
			return syncImageGenService(tx, &cfg)
		}
		tools, _ := json.Marshal(buildImageGenTools(&cfg))
		now := time.Now()
		svc := model.McpService{UserID: uid, Name: fmt.Sprintf("imagegen_%d", id), DisplayName: cfg.Name, Description: cfg.Description, TransportType: "virtual", Source: "imagegen", Config: fmt.Sprintf(`{"virtual_type":"imagegen","ref_id":%d}`, id), ToolsCache: string(tools), ToolsUpdatedAt: &now, Status: common.StatusEnabled, HealthStatus: "healthy"}
		if e := tx.Create(&svc).Error; e != nil {
			return e
		}
		cfg.AutoRegister = true
		cfg.RegisteredServiceID = &svc.ID
		return tx.Save(&cfg).Error
	})
	if err == nil {
		s.register(&cfg)
	}
	return err
}
func (s *ImageGenService) register(c *model.ImageGenConfig) {
	if VirtualRegistry != nil && c.RegisteredServiceID != nil {
		VirtualRegistry.Register(*c.RegisteredServiceID, c.UserID, fmt.Sprintf("imagegen_%d", c.ID), map[string]interface{}{"virtual_type": "imagegen", "ref_id": float64(c.ID)}, virtual.ImageGenHandler)
	}
}
func (s *ImageGenService) remove(uid, id int64, deleteConfig bool) error {
	imageGenConfigMu.Lock()
	defer imageGenConfigMu.Unlock()
	var serviceID int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var cfg model.ImageGenConfig
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, uid).First(&cfg).Error; e != nil {
			return e
		}
		if cfg.RegisteredServiceID != nil {
			serviceID = *cfg.RegisteredServiceID
			for _, m := range []any{&model.McpGroupTool{}, &model.McpGroupService{}, &model.McpGroupItem{}} {
				if e := tx.Where("service_id = ?", serviceID).Delete(m).Error; e != nil {
					return e
				}
			}
			if e := tx.Where("id = ? AND user_id = ? AND source = ?", serviceID, uid, "imagegen").Delete(&model.McpService{}).Error; e != nil {
				return e
			}
		}
		cfg.AutoRegister = false
		cfg.RegisteredServiceID = nil
		if e := tx.Save(&cfg).Error; e != nil {
			return e
		}
		if deleteConfig {
			return tx.Delete(&cfg).Error
		}
		return nil
	})
	if err == nil && serviceID != 0 && VirtualRegistry != nil {
		VirtualRegistry.Unregister(serviceID)
	}
	return err
}
func (s *ImageGenService) Disable(uid, id int64) error { return s.remove(uid, id, false) }
func (s *ImageGenService) Delete(uid, id int64) error  { return s.remove(uid, id, true) }
func (s *ImageGenService) SyncAllRegisteredTools() int {
	var rows []model.ImageGenConfig
	if model.DB.Where("auto_register = ? AND registered_service_id IS NOT NULL", true).Find(&rows).Error != nil {
		return 0
	}
	n := 0
	for i := range rows {
		if syncImageGenService(model.DB, &rows[i]) == nil {
			n++
		}
	}
	return n
}

func buildImageGenTools(c *model.ImageGenConfig) []map[string]interface{} {
	config := *c
	applyImageGenDefaultDescriptions(&config)
	c = &config
	tools := []map[string]interface{}{}
	for _, operation := range []string{"generate", "edit"} {
		if c.Operation != operation && c.Operation != "both" {
			continue
		}
		name, desc := model.GenerateImageToolName, c.GenerateImageDesc
		required := []string{"prompt"}
		properties := map[string]any{"prompt": map[string]any{"type": "string", "minLength": 1, "description": "Description of the desired image or edit"}, "return_mode": map[string]any{"type": "string", "enum": []string{"url", "inline"}, "default": "url", "description": "Return stored image links; inline also includes MCP images within the response limit"}, "extra_body": map[string]any{"type": "object", "description": "Additional native upstream parameters (for example seed, watermark, prompt_extend)"}}
		if c.Provider == "openai" || c.Provider == imagegen.ProviderOpenRouter {
			properties["n"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 10, "description": "Number of images; defaults to 1"}
			for _, key := range []string{"size", "quality", "background", "output_format"} {
				properties[key] = map[string]any{"type": "string", "description": "Optional upstream " + key + "; omitted when unset"}
			}
			if c.Provider == imagegen.ProviderOpenRouter {
				for _, key := range []string{"aspect_ratio", "resolution"} {
					properties[key] = map[string]any{"type": "string", "description": "Optional OpenRouter " + key + "; supported values depend on the model"}
				}
				properties["extra_body"] = map[string]any{"type": "object", "description": "OpenRouter Image API options such as seed, output_compression and provider routing/options. Cannot override model, prompt, input_references or streaming."}
			}
		} else {
			for _, key := range []string{"aspect_ratio", "image_size"} {
				properties[key] = map[string]any{"type": "string", "description": "Optional Gemini " + key}
			}
		}
		if operation == "edit" {
			name, desc = model.EditImageToolName, c.EditImageDesc
			required = append(required, "images")
			properties["images"] = map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "minItems": 1, "maxItems": 16, "description": "Reference image URLs, data URLs or small base64 images. For local files use upload_image and run its upload command first."}
			if imagegen.SupportsMask(c.Provider, c.EditRequestFormat) {
				properties["mask"] = map[string]any{"type": "string", "description": "Optional PNG mask URL/base64; dimensions must match the first image. Transparent areas are edited on supported models."}
			}
		}
		tools = append(tools, map[string]interface{}{"name": name, "description": desc, "inputSchema": map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}})
	}
	if c.Operation != "generate" {
		upload := virtual.UploadImageTool()
		upload["description"] = model.DefaultImageGenUploadDesc
		tools = append(tools, upload)
	}
	return tools
}
func (s *ImageGenService) probe(uid int64, r *dto.ImageGenProbeReq, modelsOnly bool) (*model.ImageGenConfig, error) {
	c := newImageGenConfig(uid)
	c.Name = "Image generation test"
	if r.ConfigID > 0 {
		var err error
		c, err = model.GetImageGenConfig(uid, r.ConfigID)
		if err != nil {
			return nil, fmt.Errorf("image configuration not found")
		}
	}
	var err error
	if modelsOnly {
		err = applyImageGenConnection(c, &r.ImageGenConfigReq)
	} else {
		err = applyImageGenReq(c, &r.ImageGenConfigReq)
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}
func (s *ImageGenService) Models(ctx context.Context, uid int64, r *dto.ImageGenProbeReq) ([]imagegen.ModelInfo, error) {
	c, err := s.probe(uid, r, true)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return (&imagegen.Client{Provider: c.Provider, EndpointURL: c.EndpointURL, APIKey: c.ApiKey, Model: c.ModelName, MaxResponseBytes: 4 << 20}).ListModels(ctx)
}
func (s *ImageGenService) Test(ctx context.Context, uid int64, r *dto.ImageGenProbeReq) (*dto.CallToolResult, error) {
	c, err := s.probe(uid, r, false)
	if err != nil {
		return nil, err
	}
	tool := r.Tool
	if tool == "" {
		tool = model.GenerateImageToolName
	}
	start := time.Now()
	raw, err := virtual.RunImageGen(ctx, UploadStore, uid, c, tool, r.Arguments)
	if err != nil {
		return &dto.CallToolResult{IsError: true, Error: err.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
	}
	return parseTestResult(raw, start)
}
