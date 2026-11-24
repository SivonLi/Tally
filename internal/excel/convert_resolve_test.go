package excel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSoffice(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "soffice")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}

	// 配置有效路径
	st := ResolveSoffice(fakeBin)
	if !st.Found || st.Source != "configured" || st.Path != fakeBin {
		t.Fatalf("expected configured found, got %+v", st)
	}

	// 配置不存在路径
	missing := filepath.Join(dir, "missing-soffice")
	st = ResolveSoffice(missing)
	if st.Found || st.Source != "configured" || st.Error == "" {
		t.Fatalf("expected configured missing, got %+v", st)
	}

	// 配置目录应报错
	st = ResolveSoffice(dir)
	if st.Found || st.Source != "configured" {
		t.Fatalf("expected directory rejected, got %+v", st)
	}

	// 环境变量覆盖
	t.Setenv("TALLY_SOFFICE", fakeBin)
	st = ResolveSoffice("")
	if !st.Found || st.Source != "env" || st.Path != fakeBin {
		t.Fatalf("expected env found, got %+v", st)
	}
}
