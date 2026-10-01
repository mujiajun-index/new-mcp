package router

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPassiveRouterTest(t *testing.T) (*httptest.Server, *dto.ServiceDetail) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "passive.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.McpService{}, &model.McpServiceKey{},
		&model.McpGroup{}, &model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}, &model.ApiKey{}, &model.McpCallLog{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldPool, oldServicePool := model.DB, SessionPool, service.SessionPool
	model.DB = db
	SessionPool = bridge.NewSessionPool()
	service.SessionPool = SessionPool
	model.InitOptionMap()
	t.Cleanup(func() {
		SessionPool.CloseAll()
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
		model.DB, SessionPool, service.SessionPool = oldDB, oldPool, oldServicePool
	})
	if err := db.Create(&model.User{ID: 1, Username: "owner", Status: 1, Group: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	gateway := handler.NewGatewayHandler(SessionPool, bridge.NewToolRouter(SessionPool), virtual.NewVirtualToolRegistry())
	SetMCPRouter(engine, gateway)
	SetApiRouter(engine)
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	if err := model.UpdateOption("ServerAddress", srv.URL); err != nil {
		t.Fatal(err)
	}
	detail, err := (&service.McpServiceService{}).Create(1, &dto.CreateServiceReq{Name: "local", TransportType: common.TransportPassiveWS})
	if err != nil {
		t.Fatal(err)
	}
	return srv, detail
}

// A local MCP server behind a transparent WebSocket bridge. Unknown modern
// discovery returns the error used by old Python MCP SDKs, on the same socket.
func connectPassivePeer(t *testing.T, endpoint, serverName string) (*websocket.Conn, <-chan struct{}) {
	return connectPausedPassivePeer(t, endpoint, serverName, nil, nil)
}

func connectPausedPassivePeer(t *testing.T, endpoint, serverName string, beforeList <-chan struct{}, listSeen chan<- struct{}) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial passive endpoint: %v (HTTP %d)", err, resp.StatusCode)
		}
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(data, &req) != nil || len(req.ID) == 0 {
				continue
			}
			response := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID}
			switch req.Method {
			case "server/discover":
				response["error"] = map[string]interface{}{"code": -32602, "message": "Invalid request parameters"}
			case "initialize":
				response["result"] = map[string]interface{}{"protocolVersion": "2024-11-05", "serverInfo": map[string]string{"name": serverName, "version": "1"}, "capabilities": map[string]interface{}{"tools": map[string]interface{}{}}}
			case "tools/list":
				if listSeen != nil {
					listSeen <- struct{}{}
					<-beforeList
				}
				response["result"] = map[string]interface{}{"tools": []interface{}{map[string]interface{}{"name": "echo", "description": "Echo", "inputSchema": map[string]interface{}{"type": "object"}}}}
			case "tools/call":
				response["result"] = map[string]interface{}{"content": []interface{}{map[string]string{"type": "text", "text": serverName + "-ok"}}}
			case "ping":
				response["result"] = map[string]interface{}{}
			default:
				response["error"] = map[string]interface{}{"code": -32601, "message": "Method not found"}
			}
			if err := conn.WriteJSON(response); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = conn.Close(); <-done })
	return conn, done
}

func waitPassiveDetail(t *testing.T, id int64, connected bool, serverName string) *dto.ServiceDetail {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		detail, err := (&service.McpServiceService{}).GetByID(1, id)
		if err == nil && detail.PassiveConnected == connected &&
			(serverName == "" || detail.ServerInfo["name"] == serverName) && (!connected || len(detail.ToolsCache) == 1) {
			return detail
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("passive state did not settle")
	return nil
}

func TestPassiveEndpointHTTPGatewayAndReconnect(t *testing.T) {
	srv, detail := setupPassiveRouterTest(t)
	first, firstDone := connectPassivePeer(t, detail.PassiveURL, "first")
	waitPassiveDetail(t, detail.ID, true, "first")
	group := &model.McpGroup{UserID: 1, Name: "local_group", EndpointSlug: "local_group", AutoDiscover: true, Status: 1, ExposeMode: "direct"}
	if err := group.Insert(); err != nil {
		t.Fatal(err)
	}
	if err := model.AddServicesToGroup(group.ID, []int64{detail.ID}); err != nil {
		t.Fatal(err)
	}
	key := "sk-passive-test-key"
	hash := sha256.Sum256([]byte(key))
	if err := model.DB.Create(&model.ApiKey{UserID: 1, Name: "test", KeyHash: hex.EncodeToString(hash[:]), Permissions: `{"groups":["*"]}`, Status: 1, UnlimitedQuota: true}).Error; err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"local__echo","arguments":{}}}`} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+key)
		response, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || bytes.Contains(data, []byte(`"error"`)) || !bytes.Contains(data, []byte(`"result"`)) {
			t.Fatalf("gateway failed: %d %s", response.StatusCode, data)
		}
		if strings.Contains(body, "tools/call") && !bytes.Contains(data, []byte("first-ok")) {
			t.Fatalf("gateway did not reach local server: %s", data)
		}
		if strings.Contains(body, "tools/list") && !bytes.Contains(data, []byte("local__echo")) {
			t.Fatalf("local tool absent from gateway: %s", data)
		}
	}
	if result, err := (&service.McpServiceService{}).Test(1, detail.ID); err != nil || !result.Connected {
		t.Fatalf("test current connection: %+v, %v", result, err)
	}
	second, _ := connectPassivePeer(t, detail.PassiveURL, "second")
	waitPassiveDetail(t, detail.ID, true, "second")
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("replacement did not close old socket")
	}
	_ = first.Close()
	waitPassiveDetail(t, detail.ID, true, "second")
	_ = second.Close()
	offline := waitPassiveDetail(t, detail.ID, false, "")
	if len(offline.ToolsCache) != 1 {
		t.Fatal("disconnect discarded catalog")
	}
	if _, err := (&service.McpServiceService{}).RefreshTools(1, detail.ID); err == nil {
		t.Fatal("offline refresh unexpectedly succeeded")
	}
}

func TestPassiveEndpointCredentialResetAndDisable(t *testing.T) {
	_, detail := setupPassiveRouterTest(t)
	_, done := connectPassivePeer(t, detail.PassiveURL, "reset")
	waitPassiveDetail(t, detail.ID, true, "reset")
	replacement, err := (&service.McpServiceService{}).ResetPassiveToken(1, detail.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reset did not close socket")
	}
	for _, endpoint := range []string{detail.PassiveURL, strings.Replace(detail.PassiveURL, "token=", "invalid=", 1)} {
		conn, resp, err := websocket.DefaultDialer.Dial(endpoint, nil)
		if conn != nil {
			conn.Close()
		}
		if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("revoked token allowed upgrade: %v, %+v", err, resp)
		}
		resp.Body.Close()
	}
	_, disabledDone := connectPassivePeer(t, replacement.PassiveURL, "disabled")
	waitPassiveDetail(t, detail.ID, true, "disabled")
	status := 0
	if err := (&service.McpServiceService{}).Update(1, detail.ID, &dto.UpdateServiceReq{Status: &status}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-disabledDone:
	case <-time.After(time.Second):
		t.Fatal("disable did not close socket")
	}
	conn, resp, err := websocket.DefaultDialer.Dial(replacement.PassiveURL, nil)
	if conn != nil {
		conn.Close()
	}
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled service allowed upgrade: %v, %+v", err, resp)
	}
	resp.Body.Close()
	u, _ := url.Parse(replacement.PassiveURL)
	if _, err := service.ValidatePassiveToken(u.Query().Get("token")); err == nil {
		t.Fatal("disabled token validated")
	}
}

func TestPassiveEndpointRejectsRevocationDuringHandshake(t *testing.T) {
	for _, action := range []string{"reset", "disable", "delete"} {
		t.Run(action, func(t *testing.T) {
			_, detail := setupPassiveRouterTest(t)
			release, listSeen := make(chan struct{}), make(chan struct{}, 1)
			_, done := connectPausedPassivePeer(t, detail.PassiveURL, "late", release, listSeen)
			select {
			case <-listSeen:
			case <-time.After(time.Second):
				close(release)
				t.Fatal("peer did not reach tool discovery")
			}
			s := &service.McpServiceService{}
			var err error
			switch action {
			case "reset":
				_, err = s.ResetPassiveToken(1, detail.ID)
			case "disable":
				status := 0
				err = s.Update(1, detail.ID, &dto.UpdateServiceReq{Status: &status})
			case "delete":
				err = s.Delete(1, detail.ID)
			}
			close(release)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("revoked connection was not rejected after handshake")
			}
			if SessionPool.Get(detail.ID) != nil {
				t.Fatal("revoked connection was attached")
			}
		})
	}
}
