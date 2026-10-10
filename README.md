# DDNSTO 群晖套件

本仓库将 `ddns-client` 的 Go 客户端封装为群晖 DSM SPK，提供 Token 安装向导、配置迁移、服务启停和控制台入口。

## 构建

构建机器需要 Linux、Go（本次使用 1.24.4）、Python 3 和客户端 Go 依赖。DSM 上不需要 Python 或 Go 工具链。

在工作区根目录执行：

```sh
make -C clients/ddns-client release-cli-synology
sh clients/synology-ddnsto/build.sh
```

默认读取兄弟仓库的 `bin/cli_ddnsto/synology/`，包括 `latest`、`synology.arm`、`synology.amd64` 及存在时必须匹配的版本/hash JSON。缺失文件、错误 ELF 架构、版本/hash 不一致会终止构建。

客户端使用 `CGO_ENABLED=0`、`GOOS=linux`、`FeatureSerial=synology`、`-tags synology`，ARM 默认 `GOARM=5`。群晖产物不使用 UPX，以保留原生 ELF、Go 构建信息和进程身份。ARM64 尚未单独构建；INFO 中的历史 ARM 平台名不代表已验证 32 位兼容。

可指定外部输入和输出：

```sh
DDNSTO_PATH=/absolute/path/to/binaries OUT_DIR=/absolute/path/to/output PKG_REVISION=2 sh build.sh
# 单独生成一个包（同样需要 DDNSTO_PATH/latest）：
DDNSTO_PATH=/absolute/path/to/binaries sh pkg.sh ddnsto_x86 dsm7
```

`ARM_BIN` / `X86_BIN` 可覆盖对应二进制。`PKG_REVISION` 默认 1，套件版本为 `<客户端版本>-<修订号>`。脚本可从任意工作目录执行，在临时目录封装，源码 INFO 保持不变。全部目标生成成功后才替换目标产物；构建失败后原输出可能仍保留上次成功的文件，应以退出码和 manifest 判断本次是否成功。

## 产物

默认目录为 `dist/<客户端版本>-<修订号>/`，本次为 `dist/4.2.7-1/`：

- `ddnsto_arm-dsm6.spk`
- `ddnsto_arm-dsm7.spk`
- `ddnsto_x86-dsm6.spk`
- `ddnsto_x86-dsm7.spk`
- `SHA256SUMS`：四个 SPK 及 manifest 的 SHA256
- `build-manifest.json`：客户端/套件版本、源码 commit、未提交状态、二进制及 SPK 哈希

SPK 是外层 tar，包含 INFO、脚本、向导、图标、权限配置和内层 `package.tgz`；后者包含真实 `bin/ddnsto` 及桌面资源。x86 包使用 amd64，不支持 32 位 x86。

## 安装、升级和数据

在 DSM 套件中心手动安装匹配版本和架构的 SPK，填写控制台 Token 及 0–99 的设备编号。桌面入口打开 DDNSTO 控制台，不提供旧 README 描述的 8899 EasyExplorer 页面。

套件以 package 用户运行。DSM 7 使用 `SYNOPKG_PKGVAR`，DSM 6 使用 `/var/packages/ddnsto/etc` 保存配置、启动状态、PID 和日志，目录权限 700，初始配置文件权限 600；客户端后续写启动缓存的权限行为仍由客户端决定，外层目录继续限制访问。

- 安装时优先保留持久配置，然后兼容旧 target/bin 配置和旧包 `/tmp/.ddnsto/ddnsto-config.json` 备份。
- `preupgrade` 在旧 target 被替换前迁移配置和启动缓存；`postinst` 重建指向持久文件的链接。
- 卸载不主动删除持久数据，便于重装；实际目录保留行为需在目标 DSM 验证。
- 服务检查 PID 对应的可执行文件，避免 `killall` 影响其他客户端；启动失败会返回失败，未运行状态返回 3。
- 桌面链接交给 DSM 的 `dsmuidir` 管理，`dsmappname` 与 web/config 对齐。依据：[Synology 桌面集成](https://help.synology.com/developer-guide/integrate_dsm/desktopapp.html)、[权限](https://help.synology.com/developer-guide/privilege/privilege_config.html)、[FHS](https://help.synology.com/developer-guide/integrate_dsm/fhs.html)。

客户端已修复数字型 routeridx 被读为 0 的问题。**旧配置若填写非零数字但此前实际按 0 运行，修复后设备身份可能改变**；上线前需针对这类设备确定迁移策略，不能承诺所有旧设备身份无变化。

## 验证与发布

```sh
python3 -m unittest discover -s tests -v
# 在客户端仓库执行：
make test
# 在产物目录执行：
sha256sum -c SHA256SUMS
```

本次 Linux 主机完成了打包和生命周期回归测试、真实 ARM/amd64 编译、SPK 解包/哈希核对及 amd64 版本运行检查。生命周期测试使用临时目录和独立进程，不代替 DSM 实机测试。

客户端完整 `make test` 有一个既有阻断：`TestGoUpdateVersionParityCorpus` 读取兄弟仓库 `ddnsto-rust` 中不存在的 fixture。使用 `go test -count=1 -timeout 60s -skip '^TestGoUpdateVersionParityCorpus$' ./cmd/cli_ddnsto ./easyexplorer` 后其余测试通过。未修改该外部测试契约。

INFO 的最低 DSM 声明为 6.0-5940 / 7.0-41890；仍须验证目标设备的 CPU、内核与 Go 运行时兼容性。这些产物属于待实机验收构建，尚未签名或上传。发布前验证：新装、线上旧包升级、非零编号、Token/设备身份保留、开机自启、启停、卸载重装，以及 package 用户的共享文件访问权限。

分发沿用 [KoolCenter 群晖下载目录](https://fw.koolcenter.com/binary/ddnsto/synology/) 的四个固定文件名。先在 beta 验收，保留旧版备份，再推广正式版及校验信息。仓库没有远端凭据或上传流程，`build.sh` 只生成本地产物。
