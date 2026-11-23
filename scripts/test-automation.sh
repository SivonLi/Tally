#!/usr/bin/env bash
set -euo pipefail

# scripts/test-automation.sh
# 使用 mygo-automation 官方插件与 CLI 执行 Tally 端到端黑盒自动化测试

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if command -v mise &>/dev/null; then
  eval "$(mise activate bash)"
fi

echo "=== 1. 检查与准备 mygo-automation CLI ==="
if ! command -v mygo-automation &>/dev/null; then
  echo "安装 mygo-automation..."
  go install github.com/egoist/mygo/plugins/automation/cmd/mygo-automation
fi

MYGO_AUTO_BIN="$(command -v mygo-automation)"
echo "已就绪: $MYGO_AUTO_BIN ($("$MYGO_AUTO_BIN" version || true))"

echo "=== 2. 编译带 automation 标签的测试可执行程序 ==="
mkdir -p build
go build -tags automation -o build/tally-autotest .

SOCKET="/tmp/tally-auto-$$.sock"
rm -f "$SOCKET"

cleanup() {
  echo "清理自动化环境..."
  if [[ -n "${APP_PID:-}" ]] && kill -0 "$APP_PID" 2>/dev/null; then
    kill "$APP_PID" 2>/dev/null || true
    wait "$APP_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

echo "=== 3. 启动 Tally 实例并监听 Unix Domain Socket ==="
MYGO_AUTOMATION="$SOCKET" ./build/tally-autotest &
APP_PID=$!

echo "等待 Socket 建立 ($SOCKET)..."
for i in {1..30}; do
  if [[ -S "$SOCKET" ]]; then
    break
  fi
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "错误: Tally 进程过早退出" >&2
    exit 1
  fi
  sleep 0.2
done

if [[ ! -S "$SOCKET" ]]; then
  echo "错误: 超时未能创建 Socket: $SOCKET" >&2
  exit 1
fi

# 等待主窗口就绪
sleep 1.2

echo "=== 4. 使用 mygo-automation 执行端到端操作 ==="
SESSION="test-$$"

# 4.1 连接会话并列出视窗
echo ">> 连接 Socket 会话..."
"$MYGO_AUTO_BIN" open --socket "$SOCKET" --session "$SESSION"

echo ">> 获取视窗列表..."
"$MYGO_AUTO_BIN" windows --session "$SESSION"

# 4.2 获取可交互控件树快照
echo ">> 获取当前页面控件快照..."
"$MYGO_AUTO_BIN" snapshot -i --session "$SESSION"

# 4.3 截取总览页截图
mkdir -p testdata/snapshots
OVERVIEW_SHOT="testdata/snapshots/e2e_overview.png"
echo ">> 保存总览页截图至 $OVERVIEW_SHOT..."
"$MYGO_AUTO_BIN" screenshot "$OVERVIEW_SHOT" --session "$SESSION"

# 4.4 模拟交互：点击「考核方案」导航进入方案页
echo ">> 点击「考核方案」进入方案页..."
"$MYGO_AUTO_BIN" press "key=plans" --settle --session "$SESSION"

# 4.5 保存方案页截图
PLANS_SHOT="testdata/snapshots/e2e_plans.png"
echo ">> 保存方案页截图至 $PLANS_SHOT..."
"$MYGO_AUTO_BIN" screenshot "$PLANS_SHOT" --session "$SESSION"

# 4.6 模拟交互：切回「总览」
echo ">> 切回「总览」..."
"$MYGO_AUTO_BIN" press "key=overview" --settle --session "$SESSION"

# 4.7 验证截图文件生成
if [[ -f "$OVERVIEW_SHOT" ]] && [[ -s "$OVERVIEW_SHOT" ]] && [[ -f "$PLANS_SHOT" ]] && [[ -s "$PLANS_SHOT" ]]; then
  echo ">> 截图保存成功:"
  ls -lh "$OVERVIEW_SHOT" "$PLANS_SHOT"
else
  echo "错误: 截图文件缺失或为空" >&2
  exit 1
fi

# 4.8 正常关闭会话
echo ">> 关闭测试会话..."
"$MYGO_AUTO_BIN" close --session "$SESSION"

echo "=== 5. 端到端自动化验证全部通过 ==="
