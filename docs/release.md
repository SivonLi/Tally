# 发布流程

## 一次性准备

### 更新签名密钥

已安装的应用只接受用同一把私钥签名的更新，私钥丢失会让所有已安装的应用无法更新。

```sh
mygo keygen
```

密钥写在 `~/Library/Application Support/mygo/update-keys/`：

- `mygo-update.pub` 的内容放进 `mygo.json` 的 `updates.publicKey`
- `mygo-update.key` 存进密码管理器，同时作为 GitHub Actions 的 `MYGO_UPDATER_PRIVATE_KEY` 密钥

### `mygo.json` 的更新配置

```json
{
  "updates": {
    "publicKey": "<mygo-update.pub 的内容>",
    "github": "<owner>/<repo>",
    "changelog": "CHANGELOG.md"
  }
}
```

`github` 与 `url` 只能填一个。主渠道用 `github`，中国镜像在应用里通过 `mygo.Updater.SetSource` 接入，不需要写进配置。

### 镜像

中国镜像使用 `gh-proxy.com` 实时读取 GitHub Release，不另外存放发布文件。GitHub 地址与镜像地址一一对应：

| 用途 | 地址 |
|---|---|
| 主渠道清单 | `https://github.com/<owner>/<repo>/releases/latest/download/update-<target>.json` |
| 镜像清单 | `https://gh-proxy.com/github.com/<owner>/<repo>/releases/latest/download/update-<target>.json` |
| 主渠道文件 | `https://github.com/<owner>/<repo>/releases/download/<tag>/<file>` |
| 镜像文件 | `https://gh-proxy.com/github.com/<owner>/<repo>/releases/download/<tag>/<file>` |
| 镜像发布列表 | `https://gh-proxy.com/github.com/<owner>/<repo>/releases.atom` |

应用读取镜像上的清单，把清单里以 `https://github.com/` 开头的文件地址改写为 `https://gh-proxy.com/github.com/` 开头。版本、大小、签名原样保留，签名校验由 `mygo.Updater` 完成。

镜像不可用时，自动渠道回到 GitHub 直连。GitHub Release 是发布文件的唯一来源，镜像不改变发布内容。

### GitHub Actions 密钥

| 名称 | 内容 |
|---|---|
| `MYGO_UPDATER_PRIVATE_KEY` | `mygo-update.key` 的全部内容 |
| `MACOS_SIGNING_IDENTITY` | Developer ID 证书名称，当前未配置 |

### 代码签名

当前没有 macOS 与 Windows 的代码签名证书，构建使用临时签名：

- macOS：用户从 dmg 安装后首次启动会被 Gatekeeper 拦截，需要在「系统设置 → 隐私与安全性」里选择「仍要打开」，或右键选择「打开」
- Windows：未签名的安装包会触发 SmartScreen 提示，用户需要点「更多信息」再点「仍要运行」

自动更新不受影响。更新包用 `mygo keygen` 生成的 Ed25519 密钥签名，与操作系统的代码签名无关，校验失败会拒绝安装。

之后取得证书时补上 `macos.signingIdentity` 与公证凭据（`APPLE_ID`、`APPLE_TEAM_ID`、`APPLE_APP_PASSWORD`），Windows 补上安装包签名配置。

## 版本号

语义化版本 `MAJOR.MINOR.PATCH`：

- `MAJOR`：公开接口或数据格式的不兼容改动
- `MINOR`：新增功能
- `PATCH`：只修 bug

`0.x` 期间 `MINOR` 允许包含不兼容改动，`PATCH` 只修 bug。

发布前同时修改两处，且两处必须一致：

- `mygo.json` 的 `version`
- `CHANGELOG.md` 里 `## <版本>` 开头的小节（MyGo 用它生成更新说明）

新版本必须严格高于最近一次正式发布版本。工作流会拒绝重复版本和更低的版本。

## 正式版本

`test` 分支只运行质量检查，不发布测试版本。

首次发布前，通过 GitHub API 或网页从初始提交创建 `main`。直接推送 `main` 会触发发布工作流，这一步应在 PR 之前完成。

1. 功能分支合并进 `test`，通过 `gates` 与 `windows-build` 检查
2. 从 `test` 向 `main` 开 PR，在 PR 内修改 `mygo.json` 的 `version` 与 `CHANGELOG.md` 的对应小节
3. PR 检查通过后合并；`release.yml` 在 `main` 的推送提交上重新运行全部门槛
4. 工作流校验版本号与变更记录，构建 macOS 与 Windows 包，创建指向 `main` 提交的 `vX.Y.Z` 标签与 Release
5. Release 先以草稿创建，附件全部上传后再发布，应用不会读到缺少清单或缺少文件的版本

如果草稿创建后上传失败，下一次发布会先删除同名草稿与标签，再重新创建。已经正式发布的版本不会被覆盖。

发布前必须保证 `mygo.json` 的版本尚未发布。版本号重复时工作流会终止。标签由工作流创建，不需要手动推送。

## 回退

应用只会升级到更高的版本号，删除发布不能让已升级的用户退回。需要回退时：从上一个正常版本的分支构建，把版本号改成比出问题的版本更高，重新发布。
