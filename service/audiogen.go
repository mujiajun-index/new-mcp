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
	"github.com/mujkjk/newmcp/internal/mcp/audiogen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type AudioGenService struct{}

var audioConfigMu sync.Mutex

func audioDB() *gorm.DB {
	return model.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}
func audioDetail(c *model.AudioGenConfig) *model.AudioGenConfig {
	copy := *c
	copy.Connections = append([]model.AudioGenConnection{}, c.Connections...)
	for i := range copy.Connections {
		copy.Connections[i].HasAPIKey = copy.Connections[i].APIKey != ""
		copy.Connections[i].APIKey = ""
	}
	return &copy
}
func applyAudioConnections(c *model.AudioGenConfig, r *dto.AudioGenConfigReq) error {
	if r.Connections == nil {
		return nil
	}
	old := map[string]model.AudioGenConnection{}
	for _, v := range c.Connections {
		old[v.Key] = v
	}
	connections := []model.AudioGenConnection{}
	seen := map[string]bool{}
	for _, v := range *r.Connections {
		v.Key = strings.TrimSpace(v.Key)
		v.BaseURL = strings.TrimSpace(v.BaseURL)
		protocol := strings.TrimSpace(v.Protocol)
		if !audiogen.Contains(audiogen.ChannelProtocols, protocol) {
			return fmt.Errorf("invalid audio connection protocol %q", protocol)
		}
		if v.BaseURL == "" {
			v.BaseURL = audiogen.DefaultChannelURL(protocol)
		}
		if v.Key == "" || len(v.Key) > 64 || seen[v.Key] || len(v.BaseURL) > 512 {
			return fmt.Errorf("connection keys must be unique, nonempty and at most 64 bytes")
		}
		seen[v.Key] = true
		if _, e := audiogen.BaseURL(v.BaseURL, "openai_speech"); e != nil {
			return e
		}
		previous := old[v.Key]
		key := previous.APIKey
		if v.APIKey != nil && strings.TrimSpace(*v.APIKey) != "" {
			if len(*v.APIKey) > 4096 {
				return fmt.Errorf("API key too long")
			}
			var e error
			key, e = common.Encrypt(strings.TrimSpace(*v.APIKey))
			if e != nil {
				return e
			}
		} else if previous.BaseURL != "" && previous.BaseURL != v.BaseURL {
			return fmt.Errorf("provide an API key when changing a connection URL")
		}
		if key == "" {
			return fmt.Errorf("API key is required for connection %s", v.Key)
		}
		connections = append(connections, model.AudioGenConnection{ConfigID: c.ID, Key: v.Key, Name: strings.TrimSpace(v.Name), Protocol: protocol, BaseURL: v.BaseURL, APIKey: key})
	}
	c.Connections = connections
	return nil
}
func audioConnectionProtocol(c *model.AudioGenConfig, key, kind string) (string, error) {
	for _, conn := range c.Connections {
		if conn.Key == key {
			return audiogen.ChannelProtocol(conn.Protocol, kind)
		}
	}
	return "", fmt.Errorf("model connection is missing")
}
func applyAudio(c *model.AudioGenConfig, r *dto.AudioGenConfigReq) error {
	if r.Name != nil {
		c.Name = strings.TrimSpace(*r.Name)
	}
	if r.Description != nil {
		c.Description = strings.TrimSpace(*r.Description)
	}
	if c.Name == "" || len(c.Name) > 128 {
		return fmt.Errorf("name is required (max 128 bytes)")
	}
	if e := applyAudioConnections(c, r); e != nil {
		return e
	}
	if r.Models != nil {
		slots := []model.AudioGenModel{}
		for _, v := range *r.Models {
			v.ConnectionKey = strings.TrimSpace(v.ConnectionKey)
			protocol, e := audioConnectionProtocol(c, v.ConnectionKey, v.Kind)
			if e != nil {
				return e
			}
			s := model.AudioGenModel{ConfigID: c.ID, Kind: v.Kind, Enabled: v.Enabled, ConnectionKey: v.ConnectionKey, Protocol: protocol, Model: strings.TrimSpace(v.Model), DefaultParams: string(v.DefaultParams), TimeoutSeconds: v.TimeoutSeconds, Description: v.Description, Capabilities: audiogen.ModelPreset(v.Kind, protocol, v.Model)}
			if s.DefaultParams == "" {
				s.DefaultParams = "{}"
			}
			if s.TimeoutSeconds == 0 {
				s.TimeoutSeconds = 600
			}
			if s.Description == "" {
				s.Description = audiogen.DefaultDescription(s.Kind)
			}
			if len(s.Description) > 4096 || len(s.DefaultParams) > 65536 {
				return fmt.Errorf("description or default parameters too long")
			}
			if v.Capabilities != nil {
				s.Capabilities = *v.Capabilities
			}
			slots = append(slots, s)
		}
		c.Models = slots
	}
	if len(c.Models) == 0 || len(c.Models) > 3 {
		return fmt.Errorf("configure one to three model slots")
	}
	seen := map[string]bool{}
	for i := range c.Models {
		s := &c.Models[i]
		if seen[s.Kind] {
			return fmt.Errorf("only one model per kind is allowed")
		}
		seen[s.Kind] = true
		protocol, e := audioConnectionProtocol(c, s.ConnectionKey, s.Kind)
		if e != nil {
			return e
		}
		if protocol != s.Protocol {
			s.Protocol = protocol
			s.Capabilities = audiogen.ModelPreset(s.Kind, protocol, s.Model)
		}
		if e := audiogen.ValidateSlot(s); e != nil {
			return e
		}
	}
	return nil
}
func (s *AudioGenService) List(uid int64) ([]*model.AudioGenConfig, error) {
	var rows []model.AudioGenConfig
	if e := audioDB().Preload("Connections").Preload("Models").Where("user_id = ?", uid).Order("created_at DESC").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []*model.AudioGenConfig{}
	for i := range rows {
		out = append(out, audioDetail(&rows[i]))
	}
	return out, nil
}
func (s *AudioGenService) Get(uid, id int64) (*model.AudioGenConfig, error) {
	c, e := model.GetAudioGenConfig(uid, id)
	if e != nil {
		return nil, e
	}
	return audioDetail(c), nil
}
func saveAudioChildren(tx *gorm.DB, c *model.AudioGenConfig) error {
	if e := tx.Where("config_id = ?", c.ID).Delete(&model.AudioGenConnection{}).Error; e != nil {
		return e
	}
	if e := tx.Where("config_id = ?", c.ID).Delete(&model.AudioGenModel{}).Error; e != nil {
		return e
	}
	for i := range c.Connections {
		c.Connections[i].ID = 0
		c.Connections[i].ConfigID = c.ID
	}
	for i := range c.Models {
		c.Models[i].ID = 0
		c.Models[i].ConfigID = c.ID
	}
	if len(c.Connections) > 0 {
		if e := tx.Create(&c.Connections).Error; e != nil {
			return e
		}
	}
	return tx.Create(&c.Models).Error
}
func (s *AudioGenService) Create(uid int64, r *dto.AudioGenConfigReq) (*model.AudioGenConfig, error) {
	c := &model.AudioGenConfig{UserID: uid, Status: common.StatusEnabled}
	if e := applyAudio(c, r); e != nil {
		return nil, e
	}
	e := audioDB().Transaction(func(tx *gorm.DB) error {
		if e := tx.Omit(clause.Associations).Create(c).Error; e != nil {
			return e
		}
		return saveAudioChildren(tx, c)
	})
	return audioDetail(c), e
}
func lockedAudio(tx *gorm.DB, uid, id int64) (*model.AudioGenConfig, error) {
	q := tx.Preload("Connections").Preload("Models").Where("id = ? AND user_id = ?", id, uid)
	if tx.Dialector.Name() != "sqlite" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var c model.AudioGenConfig
	e := q.First(&c).Error
	return &c, e
}
func syncAudioService(tx *gorm.DB, c *model.AudioGenConfig) error {
	if c.RegisteredServiceID == nil {
		return nil
	}
	tools, _ := json.Marshal(buildAudioGenTools(c))
	res := tx.Model(&model.McpService{}).Where("id = ? AND user_id = ? AND source = ?", *c.RegisteredServiceID, c.UserID, "audiogen").Updates(map[string]any{"display_name": c.Name, "description": c.Description, "tools_cache": string(tools), "tools_updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("registered audio service is missing")
	}
	return nil
}
func (s *AudioGenService) register(c *model.AudioGenConfig) {
	if VirtualRegistry != nil && c.RegisteredServiceID != nil {
		VirtualRegistry.Register(*c.RegisteredServiceID, c.UserID, fmt.Sprintf("audiogen_%d", c.ID), map[string]any{"virtual_type": "audiogen", "ref_id": float64(c.ID)}, virtual.AudioGenHandler)
	}
}
func (s *AudioGenService) Update(uid, id int64, r *dto.AudioGenConfigReq) error {
	audioConfigMu.Lock()
	defer audioConfigMu.Unlock()
	var c *model.AudioGenConfig
	e := audioDB().Transaction(func(tx *gorm.DB) error {
		var e error
		c, e = lockedAudio(tx, uid, id)
		if e != nil {
			return e
		}
		if e = applyAudio(c, r); e != nil {
			return e
		}
		if c.AutoRegister && !audioHasEnabled(c) {
			return fmt.Errorf("enabled configuration needs an enabled slot")
		}
		if e = tx.Omit(clause.Associations).Save(c).Error; e != nil {
			return e
		}
		if e = saveAudioChildren(tx, c); e != nil {
			return e
		}
		return syncAudioService(tx, c)
	})
	if e == nil {
		s.register(c)
	}
	return e
}
func audioHasEnabled(c *model.AudioGenConfig) bool {
	for _, s := range c.Models {
		if s.Enabled {
			return true
		}
	}
	return false
}
func (s *AudioGenService) Enable(uid, id int64) error {
	audioConfigMu.Lock()
	defer audioConfigMu.Unlock()
	var c *model.AudioGenConfig
	e := audioDB().Transaction(func(tx *gorm.DB) error {
		var e error
		c, e = lockedAudio(tx, uid, id)
		if e != nil {
			return e
		}
		if !audioHasEnabled(c) {
			return fmt.Errorf("enable at least one model slot")
		}
		if c.RegisteredServiceID != nil {
			return syncAudioService(tx, c)
		}
		tools, _ := json.Marshal(buildAudioGenTools(c))
		now := time.Now()
		svc := model.McpService{UserID: uid, Name: fmt.Sprintf("audiogen_%d", id), DisplayName: c.Name, Description: c.Description, TransportType: "virtual", Source: "audiogen", Config: fmt.Sprintf(`{"virtual_type":"audiogen","ref_id":%d}`, id), ToolsCache: string(tools), ToolsUpdatedAt: &now, Status: common.StatusEnabled, HealthStatus: "healthy"}
		if e = tx.Create(&svc).Error; e != nil {
			return e
		}
		c.AutoRegister = true
		c.RegisteredServiceID = &svc.ID
		return tx.Omit(clause.Associations).Save(c).Error
	})
	if e == nil {
		s.register(c)
	}
	return e
}
func (s *AudioGenService) remove(uid, id int64, del bool) error {
	audioConfigMu.Lock()
	defer audioConfigMu.Unlock()
	var sid int64
	e := audioDB().Transaction(func(tx *gorm.DB) error {
		c, e := lockedAudio(tx, uid, id)
		if e != nil {
			return e
		}
		if c.RegisteredServiceID != nil {
			sid = *c.RegisteredServiceID
			for _, m := range []any{&model.McpGroupTool{}, &model.McpGroupService{}, &model.McpGroupItem{}} {
				if e = tx.Where("service_id = ?", sid).Delete(m).Error; e != nil {
					return e
				}
			}
			if e = tx.Where("id = ? AND user_id = ? AND source = ?", sid, uid, "audiogen").Delete(&model.McpService{}).Error; e != nil {
				return e
			}
		}
		c.AutoRegister = false
		c.RegisteredServiceID = nil
		if e = tx.Omit(clause.Associations).Save(c).Error; e != nil {
			return e
		}
		if del {
			if e = tx.Where("config_id = ?", id).Delete(&model.AudioGenConnection{}).Error; e != nil {
				return e
			}
			if e = tx.Where("config_id = ?", id).Delete(&model.AudioGenModel{}).Error; e != nil {
				return e
			}
			return tx.Delete(c).Error
		}
		return nil
	})
	if e == nil && sid != 0 && VirtualRegistry != nil {
		VirtualRegistry.Unregister(sid)
	}
	return e
}
func (s *AudioGenService) Disable(uid, id int64) error { return s.remove(uid, id, false) }
func (s *AudioGenService) Delete(uid, id int64) error  { return s.remove(uid, id, true) }
func (s *AudioGenService) SyncAllRegisteredTools() int {
	var rows []model.AudioGenConfig
	_ = audioDB().Preload("Connections").Preload("Models").Where("auto_register = ?", true).Find(&rows).Error
	n := 0
	for i := range rows {
		if syncAudioService(audioDB(), &rows[i]) == nil {
			n++
		}
	}
	return n
}
func (s *AudioGenService) probe(uid int64, r *dto.AudioGenProbeReq) (*model.AudioGenConfig, error) {
	c := &model.AudioGenConfig{UserID: uid, Name: "Audio generation test", Status: common.StatusEnabled}
	if r.ConfigID > 0 {
		var e error
		c, e = model.GetAudioGenConfig(uid, r.ConfigID)
		if e != nil {
			return nil, e
		}
	}
	if e := applyAudio(c, &r.AudioGenConfigReq); e != nil {
		return nil, e
	}
	return c, nil
}
func (s *AudioGenService) Test(ctx context.Context, uid int64, r *dto.AudioGenProbeReq) (*dto.CallToolResult, error) {
	c, e := s.probe(uid, r)
	if e != nil {
		return nil, e
	}
	start := time.Now()
	raw, e := virtual.RunAudioGen(ctx, uid, c, audiogen.Tool(r.Kind), r.Arguments, "test")
	if e != nil {
		return &dto.CallToolResult{IsError: true, Error: e.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
	}
	return parseTestResult(raw, start)
}
func (s *AudioGenService) Models(ctx context.Context, uid int64, r *dto.AudioGenProbeReq) ([]audiogen.ModelInfo, error) {
	c := &model.AudioGenConfig{UserID: uid}
	if r.ConfigID > 0 {
		var e error
		c, e = model.GetAudioGenConfig(uid, r.ConfigID)
		if e != nil {
			return nil, e
		}
	}
	if e := applyAudioConnections(c, &r.AudioGenConfigReq); e != nil {
		return nil, e
	}
	key := strings.TrimSpace(r.ConnectionKey)
	for _, slot := range c.Models {
		if slot.Kind == r.Kind {
			if key == "" {
				key = slot.ConnectionKey
			}
		}
	}
	for _, v := range c.Connections {
		if v.Key == key {
			protocol, e := audiogen.ChannelProtocol(v.Protocol, r.Kind)
			if e != nil {
				return nil, e
			}
			secret, e := common.Decrypt(v.APIKey)
			if e != nil {
				return nil, e
			}
			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			return (&audiogen.Client{Connection: audiogen.Connection{BaseURL: v.BaseURL, APIKey: secret, Slot: model.AudioGenModel{Protocol: protocol, Kind: r.Kind}}}).ListModels(ctx)
		}
	}
	return nil, fmt.Errorf("connection not found")
}
