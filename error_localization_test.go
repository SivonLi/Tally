package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"tally/locales"
)

func TestErrorLocalization(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    "WebDAV 服务器地址不能为空",
			expected: "WebDAV server URL is required",
		},
		{
			input:    "仅允许 HTTPS 或局域网内网 HTTP 连接",
			expected: "Only HTTPS or local network HTTP connections are allowed",
		},
		{
			input:    "创建上传快照失败: permission denied",
			expected: "Failed to create upload snapshot: permission denied",
		},
		{
			input:    "打开校验临时数据库失败: corrupted",
			expected: "Failed to open temporary database for verification: corrupted",
		},
		{
			input:    "下载的数据未通过 SQLite 完整性检查，放弃替换",
			expected: "Downloaded data failed SQLite integrity check, replacement aborted",
		},
		{
			input:    "考试已经存在",
			expected: "Exam already exists",
		},
	}

	// 1. 测试英文环境映射
	t.Run("English locale", func(t *testing.T) {
		i18n.SetLocaleFunc(func() string { return "en" })
		defer i18n.SetLocaleFunc(nil)

		for _, tc := range cases {
			actual := localizeErrorMessage(tc.input)
			if actual != tc.expected {
				t.Errorf("localizeErrorMessage(%q) in English = %q, expected %q", tc.input, actual, tc.expected)
			}
		}

		a := &app{}
		if a.localizeError(errors.New("级已经存在")) != "Cohort already exists" {
			t.Errorf("localizeError failed for Cohort already exists")
		}
		if a.localizeError(nil) != "" {
			t.Errorf("localizeError(nil) should be empty")
		}
	})

	// 2. 测试中文环境原样保留
	t.Run("Chinese locale", func(t *testing.T) {
		i18n.SetLocaleFunc(func() string { return "zh-Hans" })
		defer i18n.SetLocaleFunc(nil)

		for _, tc := range cases {
			actual := localizeErrorMessage(tc.input)
			if actual != tc.input {
				t.Errorf("localizeErrorMessage(%q) in Chinese = %q, expected %q", tc.input, actual, tc.input)
			}
		}
	})
}

func TestSyncAndAnalysisBilingualStrings(t *testing.T) {
	// 验证同步与分析文案在 zh-Hans 与 en 都有合法值
	locales.In("zh-Hans")
	zhReq := locales.In("zh-Hans").SettingsSyncWebDAVURLRequired()
	if !strings.Contains(zhReq, "不能为空") {
		t.Errorf("expected Chinese required message, got %q", zhReq)
	}

	enReq := locales.In("en").SettingsSyncWebDAVURLRequired()
	if !strings.Contains(enReq, "required") {
		t.Errorf("expected English required message, got %q", enReq)
	}

	zhPts := fmt.Sprintf(locales.In("zh-Hans").AnalysisScorePoints(), "95.5")
	if zhPts != "95.5 分" {
		t.Errorf("expected 95.5 分, got %q", zhPts)
	}

	enPts := fmt.Sprintf(locales.In("en").AnalysisScorePoints(), "95.5")
	if enPts != "95.5 pts" {
		t.Errorf("expected 95.5 pts, got %q", enPts)
	}

	zhRemote := locales.In("zh-Hans").SettingsSyncCheckRemote()
	if zhRemote != "检查远端" {
		t.Errorf("expected 检查远端, got %q", zhRemote)
	}

	enRemote := locales.In("en").SettingsSyncCheckRemote()
	if enRemote != "Check Remote" {
		t.Errorf("expected Check Remote, got %q", enRemote)
	}
}
