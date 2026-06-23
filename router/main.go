package router

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	frontend "github.com/mujkjk/newmcp" // package frontend: embedded web/dist SPA assets
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/camera"
	"github.com/mujkjk/newmcp/internal/mcp/cloud"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
)

var (
	GatewayHandler  *handler.GatewayHandler
	SessionPool     *bridge.SessionPool
	CloudManager    *cloud.Manager
	VirtualRegistry *virtual.VirtualToolRegistry
	CameraStream    *camera.CameraStreamManager
)

func InitGateway() {
	SessionPool = bridge.NewSessionPool()
	toolRouter := bridge.NewToolRouter(SessionPool)
	VirtualRegistry = virtual.NewVirtualToolRegistry()
	CameraStream = camera.NewCameraStreamManager()

	virtual.StreamManager = CameraStream

	GatewayHandler = handler.NewGatewayHandler(SessionPool, toolRouter, VirtualRegistry)

	loadVirtualServices()

	CloudManager = cloud.NewManager(SessionPool, toolRouter, GatewayHandler)
	service.CloudManager = CloudManager
	service.SessionPool = SessionPool
	service.VirtualRegistry = VirtualRegistry
	service.CameraStreamMgr = CameraStream
}

func loadVirtualServices() {
	var services []model.McpService
	model.DB.Where("transport_type = ?", "virtual").Find(&services)

	for _, svc := range services {
		config := virtual.ParseConfig(svc.Config)

		virtualType, _ := config["virtual_type"].(string)
		switch virtualType {
		case "vision":
			VirtualRegistry.Register(svc.ID, svc.UserID, svc.Name, config, virtual.VisionHandler)
		case "camera":
			VirtualRegistry.Register(svc.ID, svc.UserID, svc.Name, config, virtual.CameraHandler)
		default:
			log.Printf("[virtual] unknown virtual_type %q for service %d", virtualType, svc.ID)
		}
	}
	log.Printf("[virtual] loaded %d virtual services", len(services))
}

func SetRouter(engine *gin.Engine) {
	InitGateway()
	SetApiRouter(engine)
	SetMCPRouter(engine, GatewayHandler)
	serveFrontend(engine)
}

// serveFrontend serves the React SPA from the frontend assets embedded into the
// binary (web/dist, via //go:embed). Embedding the build output removes any
// runtime dependency on the filesystem layout, so the served paths always match
// what rsbuild produced — no more /static vs /assets mismatches.
func serveFrontend(engine *gin.Engine) {
	dist, err := fs.Sub(frontend.DistFS, "web/dist")
	if err != nil {
		log.Printf("[web] embedded frontend unavailable: %v", err)
		return
	}
	fileServer := http.FileServer(http.FS(dist))

	engine.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// API and MCP transport endpoints must always answer with JSON, never the
		// SPA shell. If a non-POST method (e.g. the GET a Streamable-HTTP MCP
		// client sends to open its server->client channel) lands here and gets
		// index.html back as HTTP 200 text/html, the client tries to parse it as
		// JSON and dies with "Unexpected token '<'". This covers /mcp, the
		// smart-mode transport under /smart/mcp (+ /smart/mcp/ws), /api/, and the
		// RFC 8615 well-known paths an MCP client probes on a 401
		// (/.well-known/oauth-protected-resource/mcp etc.) — those are reserved
		// discovery URLs, never SPA routes. Use trailing slashes ("/api/",
		// "/smart/") so frontend pages such as /api-keys or /smart-config are NOT
		// mistaken for API calls.
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/mcp/") || strings.HasPrefix(path, "/smart/") || strings.HasPrefix(path, "/.well-known/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// A programmatic call that hits an unknown path should also get JSON, not
		// the HTML shell — an SPA only navigates via GET, so any POST/PUT/DELETE/
		// PATCH here is a misconfigured client (e.g. wrong MCP base URL). Returning
		// HTML 200 would hide the error and break JSON parsing on the client.
		if c.Request.Method != http.MethodGet {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// Serve a real embedded asset (js/css/svg/png/favicon) when present.
		if path != "/" {
			rel := strings.TrimPrefix(path, "/")
			if f, openErr := dist.Open(rel); openErr == nil {
				isDir := false
				if st, statErr := f.Stat(); statErr == nil {
					isDir = st.IsDir()
				}
				f.Close()
				if !isDir {
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
			}
		}

		// SPA fallback: unknown routes serve index.html for client-side routing.
		// Serve the bytes directly via c.Data — http.FileServer redirects any
		// path ending in /index.html to "./", which would loop forever on "/".
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", frontend.IndexHTML)
	})
}

func StartCloudConnections() {
	if CloudManager != nil {
		CloudManager.StartAll()
	}
}

func StopCloudConnections() {
	if CloudManager != nil {
		CloudManager.StopAll()
	}
}
