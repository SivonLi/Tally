ObjC.import("stdlib");

var APP = "Tally";

function systemEvents() {
  return Application("System Events");
}

// wake 唤醒显示器与辅助功能接口。显示器休眠时窗口列表为空，截图也是黑屏。
function wake() {
  var app = Application.currentApplication();
  app.includeStandardAdditions = true;
  app.doShellScript("/usr/bin/caffeinate -u -t 2 >/dev/null 2>&1 || true");
  delay(0.3);
}

function tapp() {
  var proc = systemEvents().processes.byName(APP);
  if (!proc.exists()) throw new Error("Tally 未运行");
  return proc;
}

function attr(el, name) {
  try {
    var value = el.attributes.byName(name).value();
    if (value === null || value === undefined) return "";
    return String(value);
  } catch (e) {
    return "";
  }
}

function subroleOf(el) {
  try {
    return String(el.subrole());
  } catch (e) {
    return "";
  }
}

function nameOf(el) {
  try {
    return String(el.name());
  } catch (e) {
    return "";
  }
}

function allWindows() {
  return tapp().windows();
}

function standardWindow() {
  var list = allWindows();
  for (var i = 0; i < list.length; i++) {
    if (nameOf(list[i]) === APP && subroleOf(list[i]) === "AXStandardWindow") return list[i];
  }
  for (var j = 0; j < list.length; j++) {
    if (subroleOf(list[j]) === "AXStandardWindow" && nameOf(list[j]) !== "设置") return list[j];
  }
  for (var k = 0; k < list.length; k++) {
    if (nameOf(list[k]) === APP) return list[k];
  }
  throw new Error("没有找到 Tally 主窗口");
}

function dialogWindow() {
  var list = allWindows();
  for (var i = 0; i < list.length; i++) {
    if (subroleOf(list[i]) === "AXDialog") return list[i];
  }
  throw new Error("没有找到文件对话框");
}

function dialogCount() {
  var count = 0;
  var list = allWindows();
  for (var i = 0; i < list.length; i++) {
    if (subroleOf(list[i]) === "AXDialog") count++;
  }
  return count;
}

function collect(el, depth, out, maxDepth, limit) {
  if (limit > 0 && out.length >= limit) return;
  if (maxDepth >= 0 && depth > maxDepth) return;
  var role = "";
  try {
    role = String(el.role());
  } catch (e) {
    return;
  }
  var pos = "";
  var x = 0;
  var y = 0;
  var w = 0;
  var h = 0;
  var hasFrame = false;
  try {
    var p = el.position();
    var s = el.size();
    x = Number(p[0]);
    y = Number(p[1]);
    w = Number(s[0]);
    h = Number(s[1]);
    hasFrame = true;
    pos = x + "," + y + " " + w + "x" + h;
  } catch (e) {}
  out.push({
    el: el,
    depth: depth,
    role: role,
    title: attr(el, "AXTitle"),
    desc: attr(el, "AXDescription"),
    value: attr(el, "AXValue"),
    enabled: attr(el, "AXEnabled"),
    pos: pos,
    x: x,
    y: y,
    w: w,
    h: h,
    hasFrame: hasFrame,
  });
  var kids = [];
  try {
    kids = el.uiElements();
  } catch (e) {
    return;
  }
  for (var i = 0; i < kids.length; i++) {
    if (limit > 0 && out.length >= limit) return;
    collect(kids[i], depth + 1, out, maxDepth, limit);
  }
}

function tree(dialog, maxDepth, limit) {
  var out = [];
  collect(dialog ? dialogWindow() : standardWindow(), 0, out, maxDepth, limit);
  return out;
}

function findElement(el, role, text, depth, maxDepth) {
  if (maxDepth >= 0 && depth > maxDepth) return null;
  var foundRole = "";
  try {
    foundRole = String(el.role());
  } catch (e) {
    return null;
  }
  if (foundRole === role) {
    var title = attr(el, "AXTitle");
    var desc = attr(el, "AXDescription");
    var value = attr(el, "AXValue");
    if (title === text || desc === text || value === text) return el;
  }
  var kids = [];
  try {
    kids = el.uiElements();
  } catch (e) {
    return null;
  }
  for (var i = 0; i < kids.length; i++) {
    var found = findElement(kids[i], role, text, depth + 1, maxDepth);
    if (found) return found;
  }
  return null;
}

function render(nodes) {
  var lines = [];
  for (var i = 0; i < nodes.length; i++) {
    var n = nodes[i];
    lines.push(
      [
        i,
        n.role,
        n.title,
        n.desc,
        n.value,
        n.enabled,
        n.pos,
      ].join(" | "),
    );
  }
  return lines.join("\n");
}

function waitFor(condition, seconds, what) {
  var steps = Math.ceil(seconds * 10);
  for (var i = 0; i < steps; i++) {
    try {
      if (condition()) return;
    } catch (e) {}
    delay(0.1);
  }
  throw new Error("等待超时: " + what);
}

function goTo(path) {
  var se = systemEvents();
  tapp().frontmost = true;
  delay(0.3);
  se.keystroke("g", { using: ["command down", "shift down"] });
  waitFor(function () {
    return dialogWindow().sheets().length > 0;
  }, 5, "前往文件夹面板");
  var dialog = dialogWindow();
  var sheet = dialog.sheets()[0];
  var field = sheet.textFields()[0];
  field.value = path;
  field.attributes.byName("AXFocused").value = true;
  delay(0.2);
  se.keyCode(36);
  waitFor(function () {
    return dialog.sheets().length === 0;
  }, 10, "前往文件夹面板关闭");
}

function pick(path) {
  var dialog = dialogWindow();
  goTo(path);
  delay(0.5);
  var group = dialog.splitterGroups()[0];
  group.buttons()[1].click();
  waitFor(function () {
    return dialogCount() === 0;
  }, 15, "文件对话框关闭");
  return "已选择 " + path;
}

function save(path) {
  var slash = path.lastIndexOf("/");
  if (slash <= 0) throw new Error("导出路径需要是绝对路径");
  var dir = path.slice(0, slash);
  var base = path.slice(slash + 1);
  var dialog = dialogWindow();
  goTo(dir);
  delay(0.5);
  var group = dialog.splitterGroups()[0];
  var fields = group.textFields();
  if (fields.length > 0) {
    fields[0].value = base;
    delay(0.2);
  }
  group.buttons()[1].click();
  waitFor(function () {
    return dialogCount() === 0;
  }, 15, "保存对话框关闭");
  return "已保存到 " + path;
}

function shellQuote(value) {
  return "'" + String(value).replace(/'/g, "'\\''") + "'";
}

function projectRoot() {
  var fm = $.NSFileManager.defaultManager;
  var dir = fm.currentDirectoryPath.js;
  for (var i = 0; i < 12; i++) {
    if (fm.fileExistsAtPath(dir + "/scripts/ui-test/windowid.swift")) return dir;
    var cut = dir.lastIndexOf("/");
    if (cut <= 0) break;
    dir = dir.slice(0, cut);
  }
  throw new Error("找不到项目根目录");
}

function writeText(path, text) {
  var data = $.NSString.stringWithString(text).dataUsingEncoding(
    $.NSUTF8StringEncoding,
  );
  if (!data.writeToFileAtomically(path, true)) {
    throw new Error("写入文件失败: " + path);
  }
}

function capture(path, dialog) {
  if (path.charAt(0) !== "/") throw new Error("截图路径需要是绝对路径");
  var win = dialog ? dialogWindow() : standardWindow();
  var app = Application.currentApplication();
  app.includeStandardAdditions = true;
  try {
    Application(APP).activate();
  } catch (e) {}
  tapp().frontmost = true;
  delay(0.6);
  var p = win.position();
  var s = win.size();
  var root = projectRoot();
  var helper = root + "/scripts/ui-test/windowid.swift";
  var id = String(
    app.doShellScript(
      "/usr/bin/swift " +
        shellQuote(helper) +
        " Tally " +
        p[0] +
        " " +
        p[1] +
        " " +
        s[0] +
        " " +
        s[1],
    ),
  ).trim();
  if (!id) throw new Error("没有找到窗口编号");
  var slash = path.lastIndexOf("/");
  if (slash <= 0) throw new Error("截图路径需要是绝对路径");
  app.doShellScript("/bin/mkdir -p " + shellQuote(path.slice(0, slash)));
  app.doShellScript("/usr/sbin/screencapture -x -o -l" + id + " " + shellQuote(path));
  var bytes = Number(
    String(app.doShellScript("/usr/bin/stat -f%z " + shellQuote(path))).trim(),
  );
  if (!(bytes > 1024)) throw new Error("截图文件为空或者过小: " + path);
  return {
    path: path,
    x: Number(p[0]),
    y: Number(p[1]),
    width: Number(s[0]),
    height: Number(s[1]),
    bytes: bytes,
  };
}

function shot(path, dialog) {
  var cap = capture(path, dialog);
  var app = Application.currentApplication();
  app.includeStandardAdditions = true;
  var dims = String(
    app.doShellScript(
      "/usr/bin/sips -g pixelWidth -g pixelHeight " + shellQuote(path),
    ),
  ).trim();
  return (
    "截图: " +
    cap.path +
    "\n窗口: " +
    cap.width +
    "x" +
    cap.height +
    " 点\n" +
    dims
  );
}

function visual(dir, name, dialog) {
  if (!dir || dir.charAt(0) !== "/") throw new Error("截图目录需要是绝对路径");
  if (!name || name === "." || name === ".." || /[\/\\\0]/.test(name)) {
    throw new Error("截图名称不能为空，也不能包含路径分隔符");
  }
  var base = dir.replace(/\/+$/, "") + "/" + name;
  var png = base + ".png";
  var jsonPath = base + ".json";
  var cap = capture(png, dialog);
  var nodes = tree(dialog, -1, 200000);
  var items = [];
  var seen = {};
  for (var i = 0; i < nodes.length; i++) {
    var n = nodes[i];
    if (n.role !== "AXStaticText" && n.role !== "AXHeading") continue;
    if (!n.hasFrame) continue;
    var text = n.title || n.desc || n.value;
    if (!text) continue;
    text = String(text).replace(/\s+/g, " ").trim();
    if (!text) continue;
    if (n.w < 2 || n.h < 2) continue;
    var key = text + "|" + n.x + "|" + n.y + "|" + n.w + "|" + n.h;
    if (seen[key]) continue;
    seen[key] = true;
    items.push({
      role: n.role,
      text: text,
      x: n.x,
      y: n.y,
      width: n.w,
      height: n.h,
    });
  }
  writeText(
    jsonPath,
    JSON.stringify({
      window: {
        x: cap.x,
        y: cap.y,
        width: cap.width,
        height: cap.height,
      },
      nodes: items,
    }),
  );
  var app = Application.currentApplication();
  app.includeStandardAdditions = true;
  var helper = projectRoot() + "/scripts/ui-test/visualcheck.swift";
  var report = String(
    app.doShellScript(
      "/usr/bin/swift " +
        shellQuote(helper) +
        " " +
        shellQuote(png) +
        " " +
        shellQuote(jsonPath),
    ),
  );
  return report + "\n控件快照: " + jsonPath + "\n截图文件: " + png;
}

function main(argv) {
  var dialog = false;
  var depth = -1;
  var limit = 500;
  var args = [];
  for (var i = 0; i < argv.length; i++) {
    if (argv[i] === "--dialog") {
      dialog = true;
    } else if (argv[i] === "--depth") {
      depth = Number(argv[i + 1]);
      i++;
    } else if (argv[i] === "--limit") {
      limit = Number(argv[i + 1]);
      i++;
    } else {
      args.push(argv[i]);
    }
  }
  var action = args[0];
  wake();
  if (action === "windows") {
    var list = allWindows();
    var lines = [];
    for (var w = 0; w < list.length; w++) {
      var size = "";
      try {
        var s = list[w].size();
        size = s[0] + "x" + s[1];
      } catch (e) {}
      lines.push(w + " | " + nameOf(list[w]) + " | " + subroleOf(list[w]) + " | " + size);
    }
    return lines.join("\n");
  }
  if (action === "wait") {
    var seconds = args[1] ? Number(args[1]) : 20;
    waitFor(standardWindow, seconds, "主窗口");
    return "主窗口已就绪";
  }
  if (action === "dump") return render(tree(dialog, depth, limit));
  if (action === "texts") {
    var nodes = tree(dialog, depth, limit);
    var texts = [];
    for (var t = 0; t < nodes.length; t++) {
      var n = nodes[t];
      if (
        n.role === "AXStaticText" ||
        n.role === "AXHeading" ||
        n.role === "AXButton" ||
        n.role === "AXRadioButton" ||
        n.role === "AXPopUpButton" ||
        n.role === "AXTextField"
      ) {
        texts.push(n.role + ":" + (n.title || n.desc || n.value));
      }
    }
    return texts.join("\n");
  }
  if (action === "press") {
    var win = dialog ? dialogWindow() : standardWindow();
    var element = findElement(win, args[1], args[2], 0, depth);
    if (!element && !dialog) {
      var list = allWindows();
      for (var w = 0; w < list.length; w++) {
        if (subroleOf(list[w]) === "AXStandardWindow" && list[w] !== win) {
          element = findElement(list[w], args[1], args[2], 0, depth);
          if (element) break;
        }
      }
    }
    if (!element) throw new Error("没有找到 " + args[1] + " " + args[2]);
    element.actions.byName("AXPress").perform();
    delay(0.4);
    return "pressed";
  }
  if (action === "press-index") {
    var nodesToIndex = tree(dialog, depth, limit);
    var target = Number(args[1]);
    if (!nodesToIndex[target]) throw new Error("索引超出范围: " + target);
    nodesToIndex[target].el.actions.byName("AXPress").perform();
    delay(0.4);
    return String(target);
  }
  if (action === "set") {
    var setWin = dialog ? dialogWindow() : standardWindow();
    var setElement = findElement(setWin, args[1], args[2], 0, depth);
    if (!setElement) throw new Error("没有找到 " + args[1] + " " + args[2]);
    setElement.attributes.byName("AXValue").value = args[3];
    delay(0.3);
    return "set";
  }
  if (action === "key") {
    systemEvents().keyCode(Number(args[1]));
    return "key " + args[1];
  }
  if (action === "type") {
    systemEvents().keystroke(args[1]);
    return "typed";
  }
  if (action === "pick") return pick(args[1]);
  if (action === "save") return save(args[1]);
  if (action === "shot") return shot(args[1], dialog);
  if (action === "visual") return visual(args[1], args[2], dialog);
  throw new Error("未知命令: " + action);
}

function run(argv) {
  return main(argv);
}
