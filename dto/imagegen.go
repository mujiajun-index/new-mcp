package dto

import "encoding/json"

// Pointer fields distinguish an omitted update/default from an explicit zero.
type ImageGenConfigReq struct {
	Name              *string          `json:"name"`
	Description       *string          `json:"description"`
	Provider          *string          `json:"provider"`
	ModelName         *string          `json:"model_name"`
	EndpointURL       *string          `json:"endpoint_url"`
	ApiKey            *string          `json:"api_key"`
	Operation         *string          `json:"operation"`
	DefaultParams     *json.RawMessage `json:"default_params"`
	TimeoutSeconds    *int             `json:"timeout_seconds"`
	GenerateImageDesc *string          `json:"generate_image_desc"`
	EditImageDesc     *string          `json:"edit_image_desc"`
}

type ImageGenProbeReq struct {
	ImageGenConfigReq
	ConfigID  int64           `json:"config_id"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}
