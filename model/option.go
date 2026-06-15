package model

import (
	"strconv"
	"strings"
	"sync"
)

type Option struct {
	Key   string `json:"key" gorm:"primaryKey;size:128"`
	Value string `json:"value" gorm:"type:text"`
}

func (Option) TableName() string { return "options" }

var (
	OptionMap      = make(map[string]string)
	OptionMapMutex sync.RWMutex
)

var defaultOptions = map[string]string{
	"SystemName":                    "NewMCP",
	"ServerAddress":                 "http://localhost:3000",
	"Footer":                        "MCP Protocol Gateway",
	"RegisterEnabled":               "true",
	"EmailVerificationEnabled":      "false",
	"EmailDomainRestrictionEnabled": "false",
	"EmailDomainWhitelist":          "",
	"RateLimitEnabled":              "false",
	"RateLimitMaxRequests":          "60",
	"RateLimitWindowMinutes":        "1",
	"RateLimitGroupConfig":          "{}",
	"SMTPServer":                    "",
	"SMTPPort":                      "465",
	"SMTPAccount":                   "",
	"SMTPToken":                     "",
	"SMTPFrom":                      "",
	"SMTPSSLEnabled":                "true",
}

var sensitiveKeys = map[string]bool{
	"SMTPToken": true,
}

var publicKeys = map[string]bool{
	"SystemName":    true,
	"Footer":        true,
	"ServerAddress": true,
}

func InitOptionMap() {
	OptionMapMutex.Lock()
	defer OptionMapMutex.Unlock()

	OptionMap = make(map[string]string)
	for k, v := range defaultOptions {
		OptionMap[k] = v
	}

	var options []Option
	DB.Find(&options)
	for _, opt := range options {
		OptionMap[opt.Key] = opt.Value
	}
}

func UpdateOption(key string, value string) error {
	option := Option{Key: key}
	DB.FirstOrCreate(&option, Option{Key: key})
	option.Value = value
	if err := DB.Save(&option).Error; err != nil {
		return err
	}

	OptionMapMutex.Lock()
	OptionMap[key] = value
	OptionMapMutex.Unlock()
	return nil
}

func GetOptionString(key string) string {
	OptionMapMutex.RLock()
	defer OptionMapMutex.RUnlock()
	return OptionMap[key]
}

func GetOptionBool(key string) bool {
	OptionMapMutex.RLock()
	v := OptionMap[key]
	OptionMapMutex.RUnlock()
	return v == "true"
}

func GetOptionInt(key string) int {
	OptionMapMutex.RLock()
	v := OptionMap[key]
	OptionMapMutex.RUnlock()
	n, _ := strconv.Atoi(v)
	return n
}

func IsSensitiveKey(key string) bool {
	return sensitiveKeys[key]
}

func IsPublicKey(key string) bool {
	return publicKeys[key]
}

func IsEmailDomainAllowed(email string) bool {
	if !GetOptionBool("EmailDomainRestrictionEnabled") {
		return true
	}
	if email == "" {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])
	whitelist := GetOptionString("EmailDomainWhitelist")
	if whitelist == "" {
		return false
	}
	for _, d := range strings.Split(whitelist, ",") {
		if strings.TrimSpace(strings.ToLower(d)) == domain {
			return true
		}
	}
	return false
}
