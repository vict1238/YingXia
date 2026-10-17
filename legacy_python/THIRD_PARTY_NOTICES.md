# 来源、许可与依赖

本项目新增 Python / HTML / CSS / JavaScript 源码采用 GPL-3.0 发布，完整源码随包提供，见 LICENSE.txt。
界面与匹配逻辑重新实现，没有直接运行或打包 Tampermonkey 脚本。

配置来源参考 88lin/video_vip 的 3.2.9 版本：
https://github.com/88lin/video_vip/blob/main/video_vip.user.js
核对快照：30260d8fb3db47b4f14fcc1a3faf8efadf99a43a。
原作者标注：茉灵智库；原项目标注 GPL-3.0。
来源名称和地址只是配置事实，不构成本站授权、合作、安全或可用性背书。

运行时只使用 Python 标准库与用户电脑已安装的 Edge / Chrome。
本源码包不分发 Python 解释器、Edge、Chrome、第三方脚本库或任何字体文件。
BUILD_EXE.bat 可选从 PyPI 安装 PyInstaller 6.22.3；构建时应保留 Python/PyInstaller 相关许可。
参考：https://pypi.org/project/pyinstaller/6.22.3/ 。

web/demo 下两段 WebM 是开发时用 FFmpeg 生成的测试图案和合成正弦音，
不含任何影视、音乐作品素材。FFmpeg 本身未随包分发。
演示输出供测试使用，可在本项目许可下复制和修改。

evidence 下的截图来自本项目实际前端在 Linux Chromium 中的内存渲染，
不是 Windows 真机截图，也不是外部影视实播证据。
