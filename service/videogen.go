package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/mcp/videogen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type VideoGenService struct{}

var videoGenConfigMu sync.Mutex

// Credentials and private snapshots must not appear in GORM's SQL logging.
func videoConfigDB() *gorm.DB {
	return model.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}

var videoModelHTTPClient = http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
	if len(via) >= 3 || r.URL.Scheme != via[0].URL.Scheme || !strings.EqualFold(r.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("model API redirect changed origin")
	}
	return nil
}}

type VideoGenDetail struct {
	model.VideoGenConfig
	DefaultParams                     json.RawMessage `json:"default_params"`
	HasAPIKey                         bool            `json:"has_api_key"`
	GenerateVideoDescDefault          string          `json:"generate_video_desc_default"`
	GenerateVideoFromImageDescDefault string          `json:"generate_video_from_image_desc_default"`
	GetVideoStatusDescDefault         string          `json:"get_video_status_desc_default"`
	UploadImageDesc                   string          `json:"upload_image_desc"`
}

func videoGenDetail(c *model.VideoGenConfig) *VideoGenDetail {
	return &VideoGenDetail{VideoGenConfig: *c, DefaultParams: json.RawMessage(c.DefaultParams), HasAPIKey: c.APIKey != "", GenerateVideoDescDefault: model.DefaultGenerateVideoDesc, GenerateVideoFromImageDescDefault: model.DefaultGenerateVideoFromImageDesc, GetVideoStatusDescDefault: model.DefaultGetVideoStatusDesc, UploadImageDesc: model.DefaultVideoUploadDesc}
}

func newVideoGenConfig(uid int64) *model.VideoGenConfig {
	return &model.VideoGenConfig{UserID: uid, Protocol: "openai_video", Operation: "both", ImageRequestFormat: "multipart", DefaultParams: "{}", RequestTimeoutSeconds: 30, TaskTimeoutSeconds: 3600, Status: common.StatusEnabled, GenerateVideoDesc: model.DefaultGenerateVideoDesc, GenerateVideoFromImageDesc: model.DefaultGenerateVideoFromImageDesc, GetVideoStatusDesc: model.DefaultGetVideoStatusDesc}
}

func applyVideoGenConnection(c *model.VideoGenConfig, r *dto.VideoGenConfigReq) error {
	if r.Protocol != nil {
		c.Protocol = strings.TrimSpace(*r.Protocol)
	}
	if r.EndpointURL != nil {
		c.EndpointURL = strings.TrimSpace(*r.EndpointURL)
	}
	if len(c.EndpointURL) > 512 || (r.APIKey != nil && len(*r.APIKey) > 512) {
		return fmt.Errorf("endpoint or API key exceeds 512 bytes")
	}
	if r.APIKey != nil && strings.TrimSpace(*r.APIKey) != "" {
		key, err := common.Encrypt(strings.TrimSpace(*r.APIKey))
		if err != nil {
			return err
		}
		c.APIKey = key
	}
	if c.Protocol != "openai_video" {
		return fmt.Errorf("protocol must be openai_video")
	}
	if _, err := videogen.BaseURL(c.EndpointURL); err != nil {
		return err
	}
	if c.APIKey == "" {
		return fmt.Errorf("API key is required")
	}
	return nil
}

func applyVideoGenReq(c *model.VideoGenConfig, r *dto.VideoGenConfigReq) error {
	fields := []struct {
		target *string
		value  *string
	}{{&c.Name, r.Name}, {&c.Description, r.Description}, {&c.ModelName, r.ModelName}, {&c.Operation, r.Operation}, {&c.ImageRequestFormat, r.ImageRequestFormat}, {&c.GenerateVideoDesc, r.GenerateVideoDesc}, {&c.GenerateVideoFromImageDesc, r.GenerateVideoFromImageDesc}, {&c.GetVideoStatusDesc, r.GetVideoStatusDesc}}
	for _, f := range fields {
		if f.value != nil {
			*f.target = strings.TrimSpace(*f.value)
		}
	}
	if r.DefaultParams != nil {
		c.DefaultParams = string(*r.DefaultParams)
	}
	if r.RequestTimeoutSeconds != nil {
		c.RequestTimeoutSeconds = *r.RequestTimeoutSeconds
	}
	if r.TaskTimeoutSeconds != nil {
		c.TaskTimeoutSeconds = *r.TaskTimeoutSeconds
	}
	if c.Name == "" || len(c.Name) > 128 || c.ModelName == "" || len(c.ModelName) > 128 {
		return fmt.Errorf("name and model_name are required (max 128 bytes)")
	}
	if err := applyVideoGenConnection(c, r); err != nil {
		return err
	}
	if c.Operation != "generate" && c.Operation != "image" && c.Operation != "both" {
		return fmt.Errorf("operation must be generate, image or both")
	}
	if c.ImageRequestFormat != "multipart" && c.ImageRequestFormat != "json" {
		return fmt.Errorf("image_request_format must be multipart or json")
	}
	if c.RequestTimeoutSeconds < 1 || c.RequestTimeoutSeconds > 300 {
		return fmt.Errorf("request_timeout_seconds must be from 1 to 300")
	}
	if c.TaskTimeoutSeconds < 60 || c.TaskTimeoutSeconds > 86400 {
		return fmt.Errorf("task_timeout_seconds must be from 60 to 86400")
	}
	if len(c.DefaultParams) > 65536 {
		return fmt.Errorf("default_params exceeds 64 KiB")
	}
	var params map[string]any
	if json.Unmarshal([]byte(c.DefaultParams), &params) != nil || params == nil {
		return fmt.Errorf("default_params must be a JSON object")
	}
	if err := videogen.ValidateParams(params, true); err != nil {
		return err
	}
	for _, f := range []struct {
		target   *string
		fallback string
	}{{&c.GenerateVideoDesc, model.DefaultGenerateVideoDesc}, {&c.GenerateVideoFromImageDesc, model.DefaultGenerateVideoFromImageDesc}, {&c.GetVideoStatusDesc, model.DefaultGetVideoStatusDesc}} {
		if len(*f.target) > 4096 {
			return fmt.Errorf("tool descriptions exceed 4096 bytes")
		}
		if *f.target == "" {
			*f.target = f.fallback
		}
	}
	return nil
}

func (s *VideoGenService) List(uid int64) ([]*VideoGenDetail, error) {
	var rows []model.VideoGenConfig
	if err := model.DB.Where("user_id = ?", uid).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*VideoGenDetail, 0, len(rows))
	for i := range rows {
		out = append(out, videoGenDetail(&rows[i]))
	}
	return out, nil
}
func (s *VideoGenService) Get(uid, id int64) (*VideoGenDetail, error) {
	c, err := model.GetVideoGenConfig(uid, id)
	if err != nil {
		return nil, err
	}
	return videoGenDetail(c), nil
}
func (s *VideoGenService) Create(uid int64, r *dto.VideoGenConfigReq) (*VideoGenDetail, error) {
	c := newVideoGenConfig(uid)
	if err := applyVideoGenReq(c, r); err != nil {
		return nil, err
	}
	if err := videoConfigDB().Create(c).Error; err != nil {
		return nil, err
	}
	return videoGenDetail(c), nil
}

func lockedVideoGenConfig(tx *gorm.DB, uid, id int64) (*model.VideoGenConfig, error) {
	q := tx.Where("id = ? AND user_id = ?", id, uid)
	if tx.Dialector.Name() != "sqlite" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var c model.VideoGenConfig
	err := q.First(&c).Error
	return &c, err
}

func syncVideoGenService(tx *gorm.DB, c *model.VideoGenConfig) error {
	tools, _ := json.Marshal(buildVideoGenTools(c))
	res := tx.Model(&model.McpService{}).Where("id = ? AND user_id = ? AND source = ?", *c.RegisteredServiceID, c.UserID, "videogen").Updates(map[string]any{"display_name": c.Name, "description": c.Description, "tools_cache": string(tools), "tools_updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("registered video service is missing")
	}
	return nil
}

func (s *VideoGenService) Update(uid, id int64, r *dto.VideoGenConfigReq) error {
	videoGenConfigMu.Lock()
	defer videoGenConfigMu.Unlock()
	var c *model.VideoGenConfig
	err := videoConfigDB().Transaction(func(tx *gorm.DB) error {
		var e error
		c, e = lockedVideoGenConfig(tx, uid, id)
		if e != nil {
			return e
		}
		if e = applyVideoGenReq(c, r); e != nil {
			return e
		}
		if e = tx.Save(c).Error; e != nil {
			return e
		}
		if c.RegisteredServiceID != nil {
			return syncVideoGenService(tx, c)
		}
		return nil
	})
	if err == nil {
		s.register(c)
	}
	return err
}

func (s *VideoGenService) register(c *model.VideoGenConfig) {
	if VirtualRegistry != nil && c.RegisteredServiceID != nil {
		VirtualRegistry.Register(*c.RegisteredServiceID, c.UserID, fmt.Sprintf("videogen_%d", c.ID), map[string]any{"virtual_type": "videogen", "ref_id": float64(c.ID)}, virtual.VideoGenHandler)
	}
}

func (s *VideoGenService) Enable(uid, id int64) error {
	videoGenConfigMu.Lock()
	defer videoGenConfigMu.Unlock()
	var c *model.VideoGenConfig
	err := videoConfigDB().Transaction(func(tx *gorm.DB) error {
		var e error
		c, e = lockedVideoGenConfig(tx, uid, id)
		if e != nil {
			return e
		}
		if c.AutoRegister && c.RegisteredServiceID != nil {
			return syncVideoGenService(tx, c)
		}
		tools, _ := json.Marshal(buildVideoGenTools(c))
		now := time.Now()
		svc := model.McpService{UserID: uid, Name: fmt.Sprintf("videogen_%d", id), DisplayName: c.Name, Description: c.Description, TransportType: "virtual", Source: "videogen", Config: fmt.Sprintf(`{"virtual_type":"videogen","ref_id":%d}`, id), ToolsCache: string(tools), ToolsUpdatedAt: &now, Status: common.StatusEnabled, HealthStatus: "healthy"}
		if e := tx.Create(&svc).Error; e != nil {
			return e
		}
		c.AutoRegister = true
		c.RegisteredServiceID = &svc.ID
		return tx.Save(c).Error
	})
	if err == nil {
		s.register(c)
	}
	return err
}

func (s *VideoGenService) remove(uid, id int64, deleteConfig bool) error {
	videoGenConfigMu.Lock()
	defer videoGenConfigMu.Unlock()
	var serviceID int64
	err := videoConfigDB().Transaction(func(tx *gorm.DB) error {
		c, e := lockedVideoGenConfig(tx, uid, id)
		if e != nil {
			return e
		}
		if c.RegisteredServiceID != nil {
			serviceID = *c.RegisteredServiceID
			for _, m := range []any{&model.McpGroupTool{}, &model.McpGroupService{}, &model.McpGroupItem{}} {
				if e := tx.Where("service_id = ?", serviceID).Delete(m).Error; e != nil {
					return e
				}
			}
			if e := tx.Where("id = ? AND user_id = ? AND source = ?", serviceID, uid, "videogen").Delete(&model.McpService{}).Error; e != nil {
				return e
			}
		}
		c.AutoRegister = false
		c.RegisteredServiceID = nil
		if deleteConfig {
			c.APIKey = ""
		}
		if e := tx.Save(c).Error; e != nil {
			return e
		}
		if deleteConfig {
			return tx.Delete(c).Error
		}
		return nil
	})
	if err == nil && serviceID != 0 && VirtualRegistry != nil {
		VirtualRegistry.Unregister(serviceID)
	}
	return err
}
func (s *VideoGenService) Disable(uid, id int64) error { return s.remove(uid, id, false) }
func (s *VideoGenService) Delete(uid, id int64) error  { return s.remove(uid, id, true) }
func (s *VideoGenService) SyncAllRegisteredTools() int {
	var rows []model.VideoGenConfig
	if model.DB.Where("auto_register = ? AND registered_service_id IS NOT NULL", true).Find(&rows).Error != nil {
		return 0
	}
	n := 0
	for i := range rows {
		if syncVideoGenService(model.DB, &rows[i]) == nil {
			n++
		}
	}
	return n
}

func buildVideoGenTools(c *model.VideoGenConfig) []map[string]any {
	tools := []map[string]any{}
	for _, op := range []string{"generate", "image"} {
		if c.Operation != op && c.Operation != "both" {
			continue
		}
		name, desc := model.GenerateVideoToolName, c.GenerateVideoDesc
		required := []string{"prompt"}
		props := map[string]any{"prompt": map[string]any{"type": "string", "minLength": 1}, "seconds": map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": 3600, "description": "Model-dependent video duration; omitted when unset"}, "size": map[string]any{"type": "string", "description": "Model-dependent output size, such as 1280x720"}, "metadata": map[string]any{"type": "object", "description": "Native gateway video plugin parameters"}, "extra_body": map[string]any{"type": "object", "description": "Additional upstream fields; cannot override model, prompt, reference or local controls"}, "wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 30, "default": 10}, "request_id": map[string]any{"type": "string", "maxLength": 128, "description": "Reuse this ID when retrying the same submission"}}
		if op == "image" {
			name, desc = model.GenerateVideoFromImageToolName, c.GenerateVideoFromImageDesc
			required = append(required, "image")
			props["image"] = map[string]any{"type": "string", "minLength": 1, "description": "Reference image URL, data URL or small base64; upload local files with upload_image first"}
		}
		tools = append(tools, map[string]any{"name": name, "description": desc, "inputSchema": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}})
	}
	tools = append(tools, map[string]any{"name": model.GetVideoStatusToolName, "description": c.GetVideoStatusDesc, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"task_id": map[string]any{"type": "string"}, "wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 30, "default": 0}, "retry_download": map[string]any{"type": "boolean", "default": false}}, "required": []string{"task_id"}, "additionalProperties": false}})
	if c.Operation != "generate" {
		upload := virtual.UploadImageTool()
		upload["description"] = model.DefaultVideoUploadDesc
		tools = append(tools, upload)
	}
	return tools
}

func loadVideoGenProbeConfig(uid, configID int64) (*model.VideoGenConfig, error) {
	c := newVideoGenConfig(uid)
	c.Name = "Video generation test"
	if configID > 0 {
		var err error
		c, err = model.GetVideoGenConfig(uid, configID)
		if err != nil {
			return nil, fmt.Errorf("video configuration not found")
		}
	}
	return c, nil
}
func (s *VideoGenService) probe(uid int64, r *dto.VideoGenProbeReq) (*model.VideoGenConfig, error) {
	c, err := loadVideoGenProbeConfig(uid, r.ConfigID)
	if err != nil {
		return nil, err
	}
	if err := applyVideoGenReq(c, &r.VideoGenConfigReq); err != nil {
		return nil, err
	}
	return c, nil
}
func (s *VideoGenService) Models(ctx context.Context, uid int64, r *dto.VideoGenProbeReq) ([]imagegen.ModelInfo, error) {
	c, err := loadVideoGenProbeConfig(uid, r.ConfigID)
	if err != nil {
		return nil, err
	}
	// Model discovery only needs a connection, even when generation fields in
	// an unsaved form are empty or invalid.
	if err := applyVideoGenConnection(c, &r.VideoGenConfigReq); err != nil {
		return nil, err
	}
	key, err := common.Decrypt(c.APIKey)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return (&imagegen.Client{Provider: "openai", EndpointURL: c.EndpointURL, APIKey: key, Model: c.ModelName, MaxResponseBytes: 4 << 20, HTTPClient: &videoModelHTTPClient}).ListModels(ctx)
}
func (s *VideoGenService) Test(ctx context.Context, uid int64, r *dto.VideoGenProbeReq) (*dto.CallToolResult, error) {
	c, err := s.probe(uid, r)
	if err != nil {
		return nil, err
	}
	tool := r.Tool
	if tool == "" {
		tool = model.GenerateVideoToolName
	}
	start := time.Now()
	raw, err := virtual.RunVideoGen(ctx, uid, c, tool, r.Arguments, "test")
	if err != nil {
		return &dto.CallToolResult{IsError: true, Error: err.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
	}
	return parseTestResult(raw, start)
}

func (s *VideoGenService) RetryInput(uid int64, id string) (map[string]any, error) {
	if virtual.VideoManager == nil {
		return nil, fmt.Errorf("video manager not initialized")
	}
	t, err := virtual.VideoManager.Get(uid, nil, id, false)
	if err != nil {
		return nil, err
	}
	args, complete, err := videogen.RetryInput(t)
	if err != nil {
		return nil, err
	}
	return map[string]any{"task_id": t.ID, "config_id": t.ConfigID, "operation": t.Operation, "arguments": args, "has_input_snapshot": complete}, nil
}

func (s *VideoGenService) Retry(ctx context.Context, uid int64, id string, r *dto.VideoGenRetryReq) (*model.VideoGenTask, error) {
	if virtual.VideoManager == nil {
		return nil, fmt.Errorf("video manager not initialized")
	}
	t, err := virtual.VideoManager.Get(uid, nil, id, false)
	if err != nil {
		return nil, err
	}
	configID := r.ConfigID
	if configID == 0 {
		configID = t.ConfigID
	}
	if configID <= 0 {
		return nil, fmt.Errorf("select a saved video configuration to retry this draft task")
	}
	c, err := model.GetVideoGenConfig(uid, configID)
	if err != nil {
		return nil, fmt.Errorf("video configuration not found; select another saved configuration")
	}
	var args map[string]any
	if len(r.Arguments) > 0 {
		if json.Unmarshal(r.Arguments, &args) != nil || args == nil {
			return nil, fmt.Errorf("retry arguments must be a JSON object")
		}
		if _, ok := args["request_id"]; ok {
			return nil, fmt.Errorf("use the retry request_id field outside arguments")
		}
		if _, ok := args["wait_seconds"]; ok {
			return nil, fmt.Errorf("retry arguments must not contain wait_seconds")
		}
	}
	return virtual.VideoManager.RetryGeneration(ctx, uid, id, c, args, r.RequestID)
}
