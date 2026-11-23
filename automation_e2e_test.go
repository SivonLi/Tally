//go:build automation

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	_ = os.MkdirAll(scratch, 0o755)
	sockPath := filepath.Join(scratch, fmt.Sprintf("tally-test-%d.sock", time.Now().UnixNano()))
	defer os.Remove(sockPath)

	binPath := filepath.Join(scratch, "tally-autotest")
	buildCmd := exec.Command("go", "build", "-tags", "automation", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build automation binary: %v, output: %s", err, string(out))
	}
	defer os.Remove(binPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Env = append(os.Environ(), "MYGO_AUTOMATION="+sockPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start app: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	// 等待套接字就绪
	var conn net.Conn
	for i := 0; i < 30; i++ {
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

	// 等待首帧窗口建立
	time.Sleep(1200 * time.Millisecond)

	session := fmt.Sprintf("gotest-%d", time.Now().UnixNano()%100000)

	// 1. mygo-automation open
	openCmd := exec.Command(cli, "open", "--socket", sockPath, "--session", session)
	if out, err := openCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation open: %v\noutput: %s", err, string(out))
	}
	defer func() {
		_ = exec.Command(cli, "close", "--session", session).Run()
	}()

	// 2. mygo-automation windows
	winCmd := exec.Command(cli, "windows", "--session", session)
	if out, err := winCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation windows: %v\noutput: %s", err, string(out))
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

	// 5. mygo-automation press key=plans --settle
	pressCmd := exec.Command(cli, "press", "key=plans", "--settle", "--session", session)
	if out, err := pressCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation press key=plans: %v\noutput: %s", err, string(out))
	}

	// 6. mygo-automation screenshot plans
	plansShotFile := filepath.Join(shotDir, "e2e_plans.png")
	shotPlansCmd := exec.Command(cli, "screenshot", plansShotFile, "--session", session)
	if out, err := shotPlansCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation screenshot plans: %v\noutput: %s", err, string(out))
	}
	if fi, err := os.Stat(plansShotFile); err != nil || fi.Size() == 0 {
		t.Fatalf("screenshot file empty or missing: %s", plansShotFile)
	}

	// 7. mygo-automation close
	closeCmd := exec.Command(cli, "close", "--session", session)
	if out, err := closeCmd.CombinedOutput(); err != nil {
		t.Fatalf("automation close: %v\noutput: %s", err, string(out))
	}
}
