package store

import (
	"context"
	"time"
)

const (
	appearanceSettingKey = "appearance"

	// AppearanceSystem 表示外观偏好跟随系统设置。
	AppearanceSystem = ""
	// AppearanceLight 表示固定浅色外观。
	AppearanceLight = "light"
	// AppearanceDark 表示固定深色外观。
	AppearanceDark = "dark"
)

// Appearance 返回保存的外观模式，未设置时返回空字符串表示跟随系统。
func (s *Store) Appearance(ctx context.Context) (string, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE "key" = ?`, appearanceSettingKey)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 {
		return "", nil
	}
	value := stringValue(rows.Values[0][0])
	switch value {
	case "light", "dark":
		return value, nil
	default:
		return "", nil
	}
}

// SetAppearance 保存界面外观偏好（light、dark 或空字符串跟随系统）。
func (s *Store) SetAppearance(ctx context.Context, value string) error {
	switch value {
	case "light", "dark":
	default:
		value = ""
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `INSERT INTO app_settings("key", value, updated_at)
		VALUES (?, ?, ?) ON CONFLICT("key") DO UPDATE SET value = excluded.value,
		updated_at = excluded.updated_at`, appearanceSettingKey, value, now)
	return err
}
