#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
app="$root/build/darwin-arm64/Tally.app"
bin="$app/Contents/MacOS/Tally"
data="$HOME/Library/Application Support/Tally"
domain="cn.ac.yo.tally"
work="$root/.scratch/ui-test"
snapshot="$work/snapshot"

is_running() {
  pgrep -f "$bin" > /dev/null 2>&1
}

require_stopped() {
  if is_running; then
    echo "Tally 正在运行。先执行 scripts/ui-test/app.sh stop" >&2
    exit 1
  fi
}

wait_for_window() {
  local deadline=$((SECONDS + ${1:-20}))
  while (( SECONDS < deadline )); do
    if osascript -l JavaScript "$root/scripts/ui-test/ax.js" wait 2 > /dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "等待 Tally 主窗口超时" >&2
  exit 1
}

start() {
  if is_running; then
    echo "Tally 已在运行"
    return 0
  fi
  open "$app"
  wait_for_window 20
  sleep 2
  echo "Tally 已启动"
}

stop() {
  if ! is_running; then
    echo "Tally 未运行"
    return 0
  fi
  osascript -e 'tell application "Tally" to quit' > /dev/null 2>&1 || true
  local deadline=$((SECONDS + 10))
  while (( SECONDS < deadline )); do
    if ! is_running; then
      echo "Tally 已退出"
      return 0
    fi
    sleep 0.5
  done
  pkill -f "$bin" || true
  sleep 1
  if is_running; then
    echo "Tally 未能退出" >&2
    exit 1
  fi
  echo "Tally 已终止"
}

snapshot() {
  require_stopped
  mkdir -p "$work"
  rm -rf "$snapshot"
  mkdir -p "$snapshot/data"
  if [ -d "$data" ]; then
    cp -R "$data/." "$snapshot/data/"
  fi
  if defaults read "$domain" > /dev/null 2>&1; then
    defaults export "$domain" "$snapshot/defaults.plist"
    touch "$snapshot/domain.existed"
  fi
  date "+%Y-%m-%d %H:%M:%S" > "$snapshot/created_at"
  echo "已保存测试前状态到 $snapshot"
}

restore() {
  require_stopped
  if [ ! -f "$snapshot/created_at" ]; then
    echo "没有可恢复的快照。先执行 scripts/ui-test/app.sh snapshot" >&2
    exit 1
  fi
  rm -rf "$data"
  mkdir -p "$data"
  cp -R "$snapshot/data/." "$data/"
  if [ -f "$snapshot/domain.existed" ]; then
    defaults import "$domain" "$snapshot/defaults.plist"
  else
    defaults delete "$domain" > /dev/null 2>&1 || true
  fi
  echo "已恢复测试前状态"
}

status() {
  if is_running; then
    echo "运行中: $(pgrep -f "$bin" | head -n 1)"
  else
    echo "未运行"
  fi
  echo "数据目录: $data"
  if [ -f "$snapshot/created_at" ]; then
    echo "快照时间: $(cat "$snapshot/created_at")"
  else
    echo "快照: 无"
  fi
}

visual() {
  if [ "$#" -lt 1 ]; then
    echo "用法: $0 visual <名称> [--dialog]" >&2
    exit 1
  fi
  local name="$1"
  local dir="$work/screens"
  mkdir -p "$dir"
  if [ "${2:-}" = "--dialog" ]; then
    osascript -l JavaScript "$root/scripts/ui-test/ax.js" visual "$dir" "$name" --dialog
  else
    osascript -l JavaScript "$root/scripts/ui-test/ax.js" visual "$dir" "$name"
  fi
}

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  snapshot) snapshot ;;
  restore)
    restore
    if [ "${2:-}" = "--start" ]; then
      start
    fi
    ;;
  status) status ;;
  visual)
    shift
    visual "$@"
    ;;
  *)
    echo "用法: $0 {start|stop|restart|snapshot|restore [--start]|status|visual <名称> [--dialog]}" >&2
    exit 1
    ;;
esac
