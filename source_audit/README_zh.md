# 88lin 来源预检包（2026-10-10）

## 先看结论

这是验证报告与复测工具，不是可以看片的 v1.0，也不是 Windows EXE。
核对到 GitHub 脚本版本 3.2.9，blob SHA：30260d8fb3db47b4f14fcc1a3faf8efadf99a43a。
覆盖其 8 条配置来源；没有任何一条在本次环境完成“搜索 → 匹配 → 选集 → 解码播放”的验证。
这不代表 8 条全部失效：本地普通网站对照也发生 DNS 失败，网页读取工具又不能代替浏览器视频解码。

## 文件说明

- `audit_report.html`：浏览器直接打开的详细验证报告。
- `source_check.py`：Python 3.10+ 标准库网络/目录复测工具，不需要 pip 安装依赖。
- `run_check.bat`：Windows 启动脚本；要求已安装 Python。默认搜索“三体”，读取最多 3 个候选的详情。
- `matching_regression.js`：从指定源码版本提取的纯函数，使用定向输入复现匹配风险。复跑需要 Node.js；阅读报告不需要 Node.js。
- `matching_results.json`：10 个原脚本定向用例的实际运行结果；4 个满足预设要求，6 个未满足。不是影片成功率。
- `test_source_check.py` / `checker_unit_results.txt`：本次编写的 Python 预检工具的独立单元测试，10 个通过；这与原脚本的 10 个定向测试是两套测试。
- `initial_results.json` / `control_results.json`：本次环境的原始请求记录。
- `web_observations.json`：网页工具交叉核对记录，明确区分缓存页面与真实播放。
- `checker_selftest/`：预检工具在本次 Linux 环境执行的报告；不代表 Windows 上实测过。

## 在 Windows 上复测

安装 Python 3.10 或以上后，解压本包，双击 `run_check.bat`。
结果写入同目录的 `local_check_results/report.txt` 和 `report.json`。

也可在终端中运行：

```bat
py -3 source_check.py --title "红楼梦" --details
```

只检测七条解析站入口不会证明它们能解析某部影片。可显式提供一个平台播放页：

```bat
py -3 source_check.py --platform-url "你实际选定的 HTTPS 平台播放页地址"
```

**该参数会把输入的平台网址发送给七条第三方解析站，不要包含私人参数或访问令牌。**
不提供该参数时，仅访问线路入口。工具始终把 `playback_verified` 设为 `false`：
它不运行 JavaScript，不做浏览器点击，不进行视频解码，也不因为 HTTP 200 就声称播放成功。
目录里返回的片名、年份、版本要核对；自动抓取前几个候选详情并不等于已经正确匹配。

## 复跑离线测试

```bat
node matching_regression.js
py -3 -m unittest -v test_source_check
```

## 版权与来源

`matching_regression.js` 中标明的函数片段来自 88lin/video_vip（作者标注：茉灵智库），原脚本声明 GPL-3.0。
这些摘录遵循 GPL-3.0，许可证全文见 `LICENSE-GPL-3.0.txt`。
本包新增 Python 代码和测试也按 GPL-3.0 发布，完整源码已提供。
不包含影视媒体、账号、Cookie、解密密钥或远程可执行更新。
