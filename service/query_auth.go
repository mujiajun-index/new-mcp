package service

import (
	"fmt"
	"net/url"
	"strings"
)

func validateQueryParamName(name string) error {
	if name == "" || len(name) > 255 || strings.TrimSpace(name) != name ||
		strings.ContainsAny(name, "&=?#\r\n\t") {
		return fmt.Errorf("请填写有效的 URL 认证参数名")
	}
	return nil
}

func parseAuthURL(raw string) (*url.URL, url.Values, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, nil, fmt.Errorf("请填写有效的 HTTP 服务 URL")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("服务 URL 查询参数格式错误")
	}
	return u, q, nil
}

// takeQueryCredential 从完整 URL 收编唯一的目标参数值，返回不含该参数的基础 URL。
func takeQueryCredential(raw, name string) (string, string, error) {
	if err := validateQueryParamName(name); err != nil {
		return "", "", err
	}
	u, q, err := parseAuthURL(raw)
	if err != nil {
		return "", "", err
	}
	values := q[name]
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return "", "", fmt.Errorf("服务 URL 中须恰有一个非空的 %s 参数", name)
	}
	delete(q, name)
	u.RawQuery = q.Encode()
	return u.String(), values[0], nil
}

func putQueryCredential(raw, name, value string) (string, error) {
	if err := validateQueryParamName(name); err != nil {
		return "", err
	}
	u, q, err := parseAuthURL(raw)
	if err != nil {
		return "", err
	}
	q.Set(name, value)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func hasQueryCredential(raw, name string) bool {
	_, q, err := parseAuthURL(raw)
	return err == nil && len(q[name]) > 0
}

func dropQueryCredential(raw, name string) string {
	u, q, err := parseAuthURL(raw)
	if err != nil {
		return raw
	}
	delete(q, name)
	u.RawQuery = q.Encode()
	return u.String()
}

func renameQueryCredential(raw, oldName, newName string) (string, error) {
	if oldName == "" || oldName == newName {
		return raw, nil
	}
	u, q, err := parseAuthURL(raw)
	if err != nil {
		return "", err
	}
	if len(q[newName]) == 0 && len(q[oldName]) == 1 {
		q.Set(newName, q.Get(oldName))
	}
	delete(q, oldName)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func maskQueryCredential(raw, name string) string {
	u, q, err := parseAuthURL(raw)
	if err != nil || len(q[name]) == 0 {
		return raw
	}
	for i, value := range q[name] {
		q[name][i] = maskSecret(value)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// mergeMaskedQueryCredential 只还原与旧值掩码完全相同的目标参数。
func mergeMaskedQueryCredential(incoming, stored, name string) string {
	u, q, err := parseAuthURL(incoming)
	if err != nil {
		return incoming
	}
	_, old, err := parseAuthURL(stored)
	if err != nil || len(q[name]) != 1 || len(old[name]) != 1 {
		return incoming
	}
	if q.Get(name) == maskSecret(old.Get(name)) {
		q.Set(name, old.Get(name))
		u.RawQuery = q.Encode()
		return u.String()
	}
	return incoming
}
