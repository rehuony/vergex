package database

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

// LoadSetting reads a module's settings from core.settings into dst, and reports
// whether it has any yet.
func LoadSetting(db *gorm.DB, module string, dst any) (bool, error) {
	var rows []string
	if err := db.Raw("SELECT value::text FROM core.settings WHERE module = ?", module).Scan(&rows).Error; err != nil {
		return false, fmt.Errorf("read %s settings: %w", module, err)
	}
	if len(rows) == 0 {
		return false, nil
	}
	if err := json.Unmarshal([]byte(rows[0]), dst); err != nil {
		return false, fmt.Errorf("parse %s settings: %w", module, err)
	}
	return true, nil
}

// PatchSetting merges fields into a module's settings, creating them if needed. Only
// the given fields change, so two writers of different fields can't undo each other.
func PatchSetting(db *gorm.DB, module string, fields map[string]any) error {
	body, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	err = db.Exec(`
		INSERT INTO core.settings (module, value) VALUES (?, ?::jsonb)
		ON CONFLICT (module) DO UPDATE
		SET value = core.settings.value || EXCLUDED.value, updated_at = now()`, module, string(body)).Error
	if err != nil {
		return fmt.Errorf("save %s settings: %w", module, err)
	}
	return nil
}
