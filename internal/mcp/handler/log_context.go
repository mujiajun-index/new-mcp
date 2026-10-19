package handler

import (
	"encoding/json"

	"github.com/mujkjk/newmcp/model"
)

// CloudConnectionInfo is a snapshot of the cloud connection used for a call.
// PeerIP identifies the connected server or proxy, not the end-user device.
type CloudConnectionInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CloudType string `json:"cloud_type"`
	PeerIP    string `json:"peer_ip"`
	PeerPort  int    `json:"peer_port"`
	PeerHost  string `json:"peer_host"`
}

func (c *LogContext) ipType() string {
	if c.CloudConnection != nil {
		return model.LogIPTypeCloudPeer
	}
	return model.LogIPTypeClient
}

func (c *LogContext) extraJSON() string {
	if c.CloudConnection == nil {
		return ""
	}
	payload, _ := json.Marshal(struct {
		Source          string               `json:"source"`
		CloudConnection *CloudConnectionInfo `json:"cloud_connection"`
	}{Source: "cloud", CloudConnection: c.CloudConnection})
	return string(payload)
}
