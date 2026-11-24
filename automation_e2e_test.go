//go:build automation

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAutomation_E2E 使用 mygo-automation 官方插件与 CLI 进行端到端黑盒自动化测试。
func TestAutomation_E2E(t *testing.T) {
	cli, err := exec.LookPath("mygo-automation")
	if err != nil {
		t.Skip("mygo-automation CLI not found in PATH")
	}

	scratch := filepath.Join(".", ".scratch", "automation")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	sockPath := filepath.Join(scratch, fmt.Sprintf("tally-test-%d.sock", time.Now().UnixNano()))
	defer os.Remove(sockPath)

	// 隔离用户数据目录，避免污染本机真实 Tally 数据库。
	isolatedHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(isolatedHome, "Library", "Application Support"), 0o755); err != nil {
		t.Fatalf("mkdir isolated home: %v", err)
	}

	binPath := filepath.Join(scratch, "tally-autotest")
	buildCmd := exec.Command("go", "build", "-tags", "automation", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build automation binary: %v, output: %s", err, string(out))
	}
	defer os.Remove(binPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Env = append(os.Environ(),
		"HOME="+isolatedHome,
		"MYGO_LOCALE=zh-Hans",
		"MYGO_AUTOMATION="+sockPath,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start app: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	// 等待套接字就绪
	var conn net.Conn
	for i := 0; i < 40; i++ {
		conn, err = net.Dial("unix", sockPath)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("socket not ready: %v", err)
	}

	session := fmt.Sprintf("gotest-%d", time.Now().UnixNano()%100000)

	// 1. mygo-automation open
	openCmd := exec.Command(cli, "open", "--socket", sockPath, "--session", session)
	if out, err := openCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation open: %v\noutput: %s", err, string(out))
	}
	defer func() {
		_ = exec.Command(cli, "close", "--session", session).Run()
	}()

	// 2. 轮询等待主窗口建立
	var winOut string
	for i := 0; i < 50; i++ {
		out, err := exec.Command(cli, "windows", "--session", session).CombinedOutput()
		if err == nil && strings.Contains(string(out), "w1 ") {
			winOut = string(out)
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.Contains(winOut, "w1 ") {
		t.Fatalf("main window w1 did not appear; last windows output: %s", winOut)
	}

	// 3. mygo-automation snapshot -i
	snapCmd := exec.Command(cli, "snapshot", "-i", "--session", session)
	out, err := snapCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("automation snapshot: %v\noutput: %s", err, string(out))
	}
	if len(out) == 0 {
		t.Fatalf("empty snapshot output")
	}

	// 4. mygo-automation screenshot
	shotDir := filepath.Join("testdata", "snapshots")
	_ = os.MkdirAll(shotDir, 0o755)
	shotFile := filepath.Join(shotDir, "e2e_overview.png")
	shotCmd := exec.Command(cli, "screenshot", shotFile, "--session", session)
	if out, err := shotCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation screenshot: %v\noutput: %s", err, string(out))
	}
	if fi, err := os.Stat(shotFile); err != nil || fi.Size() == 0 {
		t.Fatalf("screenshot file empty or missing: %s", shotFile)
	}

	// 5. 验证稳定侧栏键：plans / help / settings
	for _, key := range []string{"plans", "help"} {
		pressCmd := exec.Command(cli, "press", "key="+key, "--settle", "--session", session)
		if out, err := pressCmd.CombinedOutput(); err != nil {
			t.Fatalf("automation press key=%s: %v\noutput: %s", key, err, string(out))
		}
	}

	// 6. 切回 plans 并截图
	if out, err := exec.Command(cli, "press", "key=plans", "--settle", "--session", session).CombinedOutput(); err != nil {
		t.Fatalf("automation press key=plans: %v\noutput: %s", err, string(out))
	}
	plansShotFile := filepath.Join(shotDir, "e2e_plans.png")
	shotPlansCmd := exec.Command(cli, "screenshot", plansShotFile, "--session", session)
	if out, err := shotPlansCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation screenshot plans: %v\noutput: %s", err, string(out))
	}
	if fi, err := os.Stat(plansShotFile); err != nil || fi.Size() == 0 {
		t.Fatalf("screenshot file empty or missing: %s", plansShotFile)
	}

	// 7. 打开设置窗口并验证语言切换时设置窗口标题同步更新
	if out, err := exec.Command(cli, "press", "key=settings", "--settle", "--session", session).CombinedOutput(); err != nil {
		t.Fatalf("automation press key=settings: %v\noutput: %s", err, string(out))
	}
	var settingsWin string
	for i := 0; i < 20; i++ {
		out, err := exec.Command(cli, "windows", "--session", session).CombinedOutput()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if strings.HasPrefix(line, "w2 ") {
					settingsWin = "w2"
					break
				}
			}
		}
		if settingsWin != "" {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if settingsWin == "" {
		t.Fatalf("settings window w2 did not open")
	}
	if out, err := exec.Command(cli, "press", "key=appearance", "--window", settingsWin, "--settle", "--session", session).CombinedOutput(); err != nil {
		t.Fatalf("open appearance settings: %v\noutput: %s", err, string(out))
	}
	snapOut, err := exec.Command(cli, "snapshot", "-i", "--window", settingsWin, "--session", session).CombinedOutput()
	if err != nil {
		t.Fatalf("snapshot settings window: %v\noutput: %s", err, string(snapOut))
	}
	var enRef string
	for _, line := range strings.Split(string(snapOut), "\n") {
		if strings.Contains(line, `"English"`) {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				enRef = fields[0]
				break
			}
		}
	}
	if enRef == "" {
		t.Fatalf("could not find English toggle in settings snapshot:\n%s", string(snapOut))
	}
	if out, err := exec.Command(cli, "press", enRef, "--settle", "--session", session).CombinedOutput(); err != nil {
		t.Fatalf("switch language to English: %v\noutput: %s", err, string(out))
	}
	time.Sleep(200 * time.Millisecond)
	afterWinOut, err := exec.Command(cli, "windows", "--session", session).CombinedOutput()
	if err != nil {
		t.Fatalf("windows after locale change: %v\noutput: %s", err, string(afterWinOut))
	}
	if !strings.Contains(string(afterWinOut), `"Settings"`) {
		t.Fatalf("settings window title did not update to English: %s", string(afterWinOut))
	}

	// 8. mygo-automation close
	closeCmd := exec.Command(cli, "close", "--session", session)
	if out, err := closeCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation close: %v\noutput: %s", err, string(out))
	}
}
