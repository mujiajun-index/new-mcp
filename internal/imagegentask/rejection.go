package imagegentask

import (
	"strings"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/model"
)

// Repair old queue rejections without changing their IDs, errors, or timestamps.
// Match the original state/error and exclude leased tasks and generated outputs.
func (m *Manager) reconcileRejectedSubmissions() error {
	const prefix = "image API HTTP 503: "
	cursor := ""
	for {
		var rows []model.ImageGenTask
		query := m.DB.WithContext(m.ctx).Where("status = ? AND lease_owner = ? AND result_key = ? AND result_count = 0 AND error LIKE ? AND id > ?", "submission_unknown", "", "", prefix+"%", cursor)
		if err := freeLease(query, time.Now()).Order("id").Limit(100).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			cursor = row.ID
			if !strings.HasPrefix(row.Error, prefix) {
				continue
			}
			rejection := &imagegen.APIError{StatusCode: 503, Message: strings.TrimPrefix(row.Error, prefix)}
			if !rejection.QueueFull() {
				continue
			}
			query := m.DB.WithContext(m.ctx).Model(&model.ImageGenTask{}).Where("id = ? AND status = ? AND error = ? AND lease_owner = ? AND result_key = ? AND result_count = 0", row.ID, "submission_unknown", row.Error, "", "")
			if err := freeLease(query, time.Now()).UpdateColumns(map[string]any{"status": "failed", "artifact_status": "failed", "snapshot": ""}).Error; err != nil {
				return err
			}
		}
	}
}
