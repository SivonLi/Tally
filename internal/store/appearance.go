package store

import (
	"context"
	"fmt"
)

// Appearance 是界面外观的取值：跟随系统、浅色或者深色。
const (
	AppearanceSystem = "system"
	AppearanceLight  = "light"
	AppearanceDark   = "dark"
)

// Appearance 读取保存的界面外观，没有设置时返回跟随系统。
func (s *Store) Appearance(ctx context.Context) (string, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE key = 'appearance'`)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 {
		return AppearanceSystem, nil
	}
	value, _ := rows.Values[0][0].(string)
	switch value {
	case AppearanceLight, AppearanceDark:
		return value, nil
	default:
		return AppearanceSystem, nil
	}
}

// SetAppearance 保存界面外观。
func (s *Store) SetAppearance(ctx context.Context, value string) error {
	switch value {
	case AppearanceSystem, AppearanceLight, AppearanceDark:
	default:
		return fmt.Errorf("未知的界面外观 %q", value)
	}
	_, err := s.db.Execute(ctx, `INSERT INTO app_settings(key, value) VALUES ('appearance', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, value)
	return err
}
