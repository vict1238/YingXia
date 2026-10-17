# Android 2.2 交付核验

APK：`YingXia_v2.2.0_Android.apk`，124905502 bytes，SHA-256 `423289feea2dccb2d9ebd019e73d8a12d3b81b4bb9ba6b1253289ea4e6133e23`。

源码：`YingXia_v2.2.0_Android_Source.zip`，38565324 bytes，SHA-256 `b1c48ae890df6b6af832d110db4fb4d5567d30f6b2373f2dc8b2d852b2957274`。871个文件的ZIP CRC与解压SHA均经核验。原始检查和软件对应见Android 2.2 VALIDATION/PACKAGING。旧发布件基线哈希已复核不变。下文为历史交接核验。

# 本次交接核验记录

日期：2026-10-12。核验对象：当前会话中实际挂载的交付文件及其中源码。应用源码未修改，本次没有联网重验第三方服务，没有执行 Windows 程序。

## 文件存在性与完整性

此前交付的 EXE、Windows 发布 ZIP、Python 源码 ZIP、Python 使用指南 HTML、来源预检 ZIP、来源报告 HTML 均找到并原字节保留。本机 EXE 与原 Windows 发布 ZIP 内 EXE 完全一致。

嵌套的 `source/YingXia_v1.0_EXE_source.zip` 已解开到 `current_source/`。早期源码与预检也解开；各解开文件与原压缩包对应成员逐项比较，结果在本目录配套核验数据中。

额外三张图片附件归档到 history_screenshots。截图可能来自中间版本或测试浏览器，不作为 Windows 执行/影视实播证据。

## EXE 格式

7,226,368 字节；MZ + PE 签名；Machine 0x8664；PE32+ optional header 0x20b；GUI subsystem 2；Security Directory 为 0，未见 Authenticode 签名目录。

SHA-256：`e7d47fe49c185c1d682c52713150f1098a19beda61e6e70128ddba21768a238b`。

## 本次重新执行的检查

| 检查 | 结果 |
|---|---|
| Go `test -race -count=1 -v ./...` | 成功；119 条 PASS = 9 个顶层 + 110 个子用例；1 个手工浏览器入口按设计 SKIP |
| Linux `go vet ./...` | 成功，无输出 |
| Windows 目标 `go vet ./...` | 成功，无输出 |
| Windows x64 交叉构建 | 成功 |
| 重建 EXE 对比原 EXE | 逐字节相同，SHA-256 相同 |

工具链来自当前环境：Go 1.23.2 linux/amd64。`GOTOOLCHAIN=local`、`GOPROXY=off`、`GOSUMDB=off`；无依赖下载。详见 `handoff_verification/handoff_checks.json` 和对应原始日志。

为减少冗余，重建副本没有再单独放一个同字节 EXE，releases 中保留原交付文件。复建结果不替换原版本，不隐式改变应用。

## 没有做的检查

没有 Windows VM/Wine/真机执行，没有第三方搜索到实际影片的全链路播放，没有重新执行历史 17 项前端浏览器用例，没有评定签名/杀毒兼容。历史截图和浏览器 JSON 原样保留并明确标为历史证据。

## 清单规则

`MANIFEST.json` 保存每个负载文件的相对路径、字节数与 SHA-256；`SHA256SUMS.txt` 为同一集合的文本清单。二者互相不计算自身哈希，避免循环依赖。最终 ZIP 的独立 SHA-256 位于同级 `.sha256.txt` 文件。

许可原件保留；不加入字体文件、账号、Cookie、用户真实数据目录、私有令牌或第三方影视素材。web/demo 两段 WebM 是原交付合成测试片，保留用于离线回归。

## 恢复范围说明

本包足够恢复已交付 Go 工程与早期 Python 工程，不承诺恢复原对话的所有临时调试目录。没有找到完整原始 88lin .user.js 附件；预检包中含所使用的函数摘录和上游引用。ViewDesk 中间稿只有本对话可取得的截图，没有伪造或重新制造其源码。
