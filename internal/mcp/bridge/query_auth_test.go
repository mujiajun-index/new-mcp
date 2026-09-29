package bridge

import (
	"strings"
	"testing"

	"github.com/mujkjk/newmcp/model"
)

func TestStaticQueryOptionKeepsKeyOutOfEndpoint(t *testing.T) {
	svc := &model.McpService{AuthType: "query_param", AuthConfig: `{"query_param_name":"tavilyApiKey"}`}
	base, opts := staticQueryOption(svc, "https://example.test/mcp?region=cn&tavilyApiKey=secret")
	if len(opts) != 1 || strings.Contains(base, "secret") || !strings.Contains(base, "region=cn") {
		t.Fatalf("base=%q opts=%d", base, len(opts))
	}
}
