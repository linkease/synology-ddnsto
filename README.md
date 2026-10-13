# DDNSTO 群晖套件

本仓库将 `ddns-client` 的 Go 客户端封装为群晖 DSM SPK，提供 Token 安装向导、安装后的设备设置页、配置迁移和服务启停。

## 构建

构建机器需要 Linux、Go（本次使用 1.24.4；设置程序源码要求 Go 1.20 或以上）、Python 3 和客户端 Go 依赖。DSM 上不需要 Python 或 Go 工具链。

在工作区根目录执行：

```sh
make -C clients/ddns-client release-cli-synology
sh clients/synology-ddnsto/build.sh
```

默认读取兄弟仓库的 `bin/cli_ddnsto/synology/`，包括 `latest`、`synology.arm`、`synology.amd64` 及存在时必须匹配的版本/hash JSON。缺失文件、错误 ELF 架构、版本/hash 不一致会终止构建。

客户端使用 `CGO_ENABLED=0`、`GOOS=linux`、`FeatureSerial=synology`、`-tags synology`，ARM 默认 `GOARM=5`。群晖产物不使用 UPX，以保留原生 ELF、Go 构建信息和进程身份。ARM64 尚未单独构建；INFO 中的历史 ARM 平台名不代表已验证 32 位兼容。

可指定外部输入和输出：

```sh
DDNSTO_PATH=/absolute/path/to/binaries OUT_DIR=/absolute/path/to/output PKG_REVISION=3 sh build.sh
# 单独生成一个包（同样需要 DDNSTO_PATH/latest）：
DDNSTO_PATH=/absolute/path/to/binaries sh pkg.sh ddnsto_x86 dsm7
```

`ARM_BIN` / `X86_BIN` 可覆盖对应二进制。`PKG_REVISION` 默认 4，套件版本为 `<客户端版本>-<修订号>`。脚本可从任意工作目录执行，在临时目录封装，源码 INFO 保持不变。全部目标生成成功后才替换目标产物；构建失败后原输出可能仍保留上次成功的文件，应以退出码和 manifest 判断本次是否成功。

## 产物

默认目录为 `dist/<客户端版本>-<修订号>/`，本次为 `dist/4.2.7-4/`：

- `ddnsto_arm-dsm6.spk`
- `ddnsto_arm-dsm7.spk`
- `ddnsto_x86-dsm6.spk`
- `ddnsto_x86-dsm7.spk`
- `SHA256SUMS`：四个 SPK 及 manifest 的 SHA256
- `build-manifest.json`：客户端/套件版本、源码 commit、未提交状态、二进制及 SPK 哈希

SPK 是外层 tar，包含 INFO、脚本、向导、图标、权限配置和内层 `package.tgz`；后者包含真实 `bin/ddnsto`、同架构的 `web/settings.cgi` 和桌面资源。打包时自动从 `settings/` 编译 CGI，页面资源内嵌于该程序。x86 包使用 amd64，不支持 32 位 x86。

## 安装、升级和数据

在 DSM 套件中心手动安装匹配版本和架构的 SPK，填写控制台 Token 及 0–99 的设备编号。安装完成后，点击 DSM 主菜单的 DDNSTO 图标或套件中心的“打开”，进入本机设备设置页。页面使用 DSM 原有 HTTP/HTTPS 入口，不另开端口，也不依赖 Web Station、PHP 或 Python。

### 安装后的设置页

- 入口：`/webman/3rdparty/ddnsto/index.html`。先登录 DSM，再从主菜单的 DDNSTO 图标打开。静态入口从同源 DSM 主窗口取得 `SynoToken`，随后打开设置 CGI；直接输入 CGI 地址不携带该参数，在启用 DSM CSRF 校验时返回 403。
- 直接打开静态入口、主窗口已关闭或跨域时显示登录指引，不绕过管理员认证。
- 查看当前 Token（默认隐藏，可点击显示）、修改 Token 和 0–99 的设备编号。
- 保存时原子更新配置，保留 `supplierCode` 等其他字段，清除过期启动缓存。
- 客户端运行时保存会短暂重启；原本停止的套件保持停止。保存或重启失败会尝试恢复原配置和启动缓存，恢复失败会明确提示。
- 使用请求校验和配置版本校验，拒绝未授权、跨站及旧页面覆盖提交；Token 不进入 URL，响应禁止缓存。
- 页面区分进程运行与服务端连接状态，显示运行实例提供的设备 ID 和版本；无法读取时明确显示未知，不把进程运行当作上线。设备编号不是设备 ID。
- 查看连接日志及启动输出的最近 200 行（每文件最多 64 KiB）；刷新页面更新，当前 Token 及常见凭证字段会遮盖，但分享前仍应检查其他敏感信息。
- 控制台、安装向导和套件信息链接统一为 `https://web.ddnsto.com/`。

认证使用 DSM 官方 `authenticate.cgi`，并检查登录用户属于 `administrators` 组。`conf/privilege` 为 `web/settings.cgi` 声明套件用户的 `4755` 权限，让 CGI 能读写套件私有配置；它不是 root 程序，不放宽配置目录权限。CGI 使用固定路径和受限子进程环境，不接受请求指定文件或命令。[认证说明](https://help.synology.com/developer-guide/integrate_dsm/web_authentication.html) / [tool 权限机制](https://help.synology.com/developer-guide/privilege/privilege_config.html)。

DSM 7.1-42661 / aarch64 实机已验证：同一管理员会话仅带 Cookie 时 CGI 返回 403，附加 SynoToken 后返回 200。已在真实 DSM 弹窗执行相同入口脚本，验证显示在线状态、设备 ID、配置表单。用户已确认设置页可用，修改为正确 Token 后设备恢复正常在线。生产程序不含测试登录开关。

### 配置持久化

套件以 package 用户运行。DSM 7 使用 `SYNOPKG_PKGVAR`，DSM 6 使用 `/var/packages/ddnsto/etc` 保存配置、启动状态、PID 和日志，目录权限 700，初始配置文件权限 600；客户端后续写启动缓存的权限行为仍由客户端决定，外层目录继续限制访问。

- 安装时优先保留持久配置，然后兼容旧 target/bin 配置和旧包 `/tmp/.ddnsto/ddnsto-config.json` 备份。
- `preupgrade` 在旧 target 被替换前迁移配置和启动缓存；`postinst` 重建指向持久文件的链接。
- 卸载不主动删除持久数据，便于重装；实际目录保留行为需在目标 DSM 验证。
- 服务检查 PID 对应的可执行文件，避免 `killall` 影响其他客户端；启动失败会返回失败，未运行状态返回 3。
- 桌面链接交给 DSM 的 `dsmuidir` 管理，`dsmappname` 与 web/config 对齐。依据：[Synology 桌面集成](https://help.synology.com/developer-guide/integrate_dsm/desktopapp.html)、[权限](https://help.synology.com/developer-guide/privilege/privilege_config.html)、[FHS](https://help.synology.com/developer-guide/integrate_dsm/fhs.html)。

客户端已修复数字型 routeridx 被读为 0 的问题。**旧配置若填写非零数字但此前实际按 0 运行，修复后设备身份可能改变**；上线前需针对这类设备确定迁移策略，不能承诺所有旧设备身份无变化。

## 验证与发布

入口回归测试额外需要 Node.js（本次使用 22）。

```sh
python3 -m unittest discover -s tests -v
(cd settings && go test -race ./...)
# 在客户端仓库执行：
make test
# 在产物目录执行：
sha256sum -c SHA256SUMS
```

本次设置页使用现有已验证的 Go 客户端 4.2.7 二进制，额外编译 ARM/amd64 CGI。Linux 主机验证覆盖配置读写、管理员认证拒绝、请求校验、停止状态保存、重启失败回滚、重复提交、SPK 内容及哈希；浏览器验证覆盖显示 Token、修改、保存、刷新保留及移动端布局。生命周期测试使用临时目录和独立进程，不代替 DSM 实机测试。

客户端完整 `make test` 有一个既有阻断：`TestGoUpdateVersionParityCorpus` 读取兄弟仓库 `ddnsto-rust` 中不存在的 fixture。使用 `go test -count=1 -timeout 60s -skip '^TestGoUpdateVersionParityCorpus$' ./cmd/cli_ddnsto ./easyexplorer` 后其余测试通过。未修改该外部测试契约。

INFO 的最低 DSM 声明为 6.0-5940 / 7.0-41890；仍须验证目标设备的 CPU、内核与 Go 运行时兼容性。当前已完成一台 DSM 7.1 / aarch64 设备的设置页与连接验证，尚未签名或上传；其他平台仍需验收。发布前验证：新装、线上旧包升级、非零编号、Token/设备身份保留、开机自启、启停、卸载重装，以及 package 用户的共享文件访问权限。

分发沿用 [KoolCenter 群晖下载目录](https://fw.koolcenter.com/binary/ddnsto/synology/) 的四个固定文件名。先在 beta 验收，保留旧版备份，再推广正式版及校验信息。仓库没有远端凭据或上传流程，`build.sh` 只生成本地产物。

## 设备上排障

先登录 DSM，再从主菜单点击 DDNSTO；请勿直接收藏带 SynoToken 的 CGI 地址。入口为 `/webman/3rdparty/ddnsto/index.html`。如遇到 403，先区分 DSM 会话校验与文件权限，不能仅凭通用 nginx 错误页判断 CGI 是否执行。以下只读脚本会检查入口和运行状态：

```sh
sudo sh /var/packages/ddnsto/scripts/diagnose
```

SSH 上查看日志（可能包含敏感数据，分享前先遮盖）：

```sh
sudo tail -n 200 /var/packages/ddnsto/var/logs/ddnstoshell.log
sudo tail -n 100 /var/packages/ddnsto/var/ddnsto.log
# DSM 6 若没有 var，改用 /var/packages/ddnsto/etc
```

连接日志通过 `--log-path` 显式写到套件数据目录，沿用客户端 5 MiB、3 个备份、3 天保留和压缩轮转；`ddnsto.log` 保留启动早期错误及标准输出。旧版默认日志目录中的历史日志不会自动迁移。2026-10-13 实机检查时，4.2.7 状态接口为 connected=true，日志有 regist ok 和 connected to server；用户随后确认此前离线是 Token 不正确，修改 Token 后已正常。修正排障脚本在 DSM 7 未指定环境变量时误查 etc 目录而报告停止的问题。
