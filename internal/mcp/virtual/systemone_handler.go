package virtual

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/systemone"
	"github.com/mujkjk/newmcp/model"
)

func SystemOneHandler(ctx context.Context, serviceID int64, _ map[string]interface{}, toolName string, args json.RawMessage) (json.RawMessage, error) {
	if toolName != "evaluate" {
		return nil, fmt.Errorf("unknown System One tool: %s", toolName)
	}
	config, err := model.GetSystemOneConfigByServiceID(serviceID)
	if err != nil || !config.AutoRegister {
		return nil, fmt.Errorf("System One config not found or disabled")
	}
	key, err := common.Decrypt(config.APIKey)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt System One API key: %w", err)
	}
	client, err := systemone.NewClient(config.Provider, config.EndpointURL, key, config.ModelName)
	if err != nil {
		return nil, err
	}
	answer, err := systemone.Evaluate(ctx, client, args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(answer)}}})
}
