package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

func TestSmartSearchConfigAndUserGroupAccess(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:smart_search_settings?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	if err := db.AutoMigrate(&model.SmartSearchConfig{}, &model.User{}, &model.McpCallLog{}); err != nil {
		t.Fatal(err)
	}
	model.OptionMapMutex.Lock()
	previousGroups := model.OptionMap["UserGroupOptions"]
	model.OptionMap["UserGroupOptions"] = "default,vip,svip"
	model.OptionMapMutex.Unlock()
	t.Cleanup(func() {
		model.OptionMapMutex.Lock()
		model.OptionMap["UserGroupOptions"] = previousGroups
		model.OptionMapMutex.Unlock()
	})
	svc := &SmartSearchService{}
	initial, err := svc.Get()
	if err != nil || initial.Enabled || initial.BatchSize != 200 || initial.Concurrency != 4 || !initial.AllGroups {
		t.Fatalf("bad defaults: %+v %v", initial, err)
	}
	if _, err := svc.SetEnabled(model.Operator{ID: 1, Username: "admin"}, true); err == nil {
		t.Fatal("enabled smart search without saved credentials")
	}
	in := &dto.SmartSearchConfigInput{Enabled: true, Provider: "typesafe", ModelName: "jev-latest", APIKey: "secret-test-key", AllGroups: false, Groups: []string{"svip"}, BatchSize: 200, Concurrency: 4}
	if _, err := svc.Update(model.Operator{ID: 1, Username: "admin"}, in); err != nil {
		t.Fatal(err)
	}
	saved, err := model.GetSmartSearchConfig()
	if err != nil || saved.APIKey == in.APIKey || strings.Contains(saved.APIKey, in.APIKey) {
		t.Fatalf("key not encrypted: %+v %v", saved, err)
	}
	user := model.User{Username: "member", Password: "test", Group: "vip", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	allowed, err := model.SmartSearchAllowed(saved, user.ID)
	if err != nil || allowed {
		t.Fatalf("vip should not be allowed: %v %v", allowed, err)
	}
	if err := db.Model(&user).Update("group", "svip").Error; err != nil {
		t.Fatal(err)
	}
	allowed, err = model.SmartSearchAllowed(saved, user.ID)
	if err != nil || !allowed {
		t.Fatalf("svip should be allowed: %v %v", allowed, err)
	}
	in.APIKey = ""
	if _, err := svc.Update(model.Operator{ID: 1, Username: "admin"}, in); err != nil {
		t.Fatal(err)
	}
	retained, _ := model.GetSmartSearchConfig()
	if retained.APIKey != saved.APIKey {
		t.Fatal("blank edit replaced the saved key")
	}
	if _, err := svc.SetEnabled(model.Operator{ID: 1, Username: "admin"}, false); err != nil {
		t.Fatal(err)
	}
	afterToggle, err := model.GetSmartSearchConfig()
	if err != nil || afterToggle.Enabled || afterToggle.APIKey != retained.APIKey || afterToggle.ModelName != retained.ModelName || afterToggle.GroupsJSON != retained.GroupsJSON {
		t.Fatalf("toggle changed unrelated configuration: %+v %v", afterToggle, err)
	}
	if _, err := svc.SetEnabled(model.Operator{ID: 1, Username: "admin"}, true); err != nil {
		t.Fatal(err)
	}
	afterToggle, err = model.GetSmartSearchConfig()
	if err != nil || !afterToggle.Enabled {
		t.Fatalf("toggle did not enable search: %+v %v", afterToggle, err)
	}
}
