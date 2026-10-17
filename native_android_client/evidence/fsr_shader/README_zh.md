# FSR 算法来源与历史检查

本目录从冻结的 Windows 源码复制算法来源、GLSL 像素检查与历史渲染审计，原文件大小及 SHA-256 见 [inherited_files.json](inherited_files.json)。它们说明本轮继承的 FSR1 着色器来源和算法移植，不代表 Android 真机性能或 Android APK 运行结果。

- [shader_provenance.json](shader_provenance.json) 固定 AMD MIT 上游提交、两个执行着色器及当时 Windows 插件修改哈希。其 `private_gpu_harness` 是历史 Windows 检查工具的原路径，未纳入 Android 构建。
- [angle_rgb_validation.json](angle_rgb_validation.json) 是历史 ANGLE pbuffer 的 GLSL 100/300 编译与 RGB 样本检查。
- [mpv_rcas_metadata_fix.md](mpv_rcas_metadata_fix.md) 和 [render_loop_audit.md](render_loop_audit.md) 保留当时的 mpv 元数据修正与 Windows 渲染线程审计。

AMD 原件与两个执行着色器位于 [assets/shaders](../../assets/shaders/README_zh.md)，Android Surface 修改说明位于 [第三方说明](../../THIRD_PARTY_NOTICES.md)。本轮 Android 控制器与实际设备证据位于 `../android`，GPU fixture 采证步骤见 [tool/android_gpu](../../tool/android_gpu/README.md)。Android ARM 真机吞吐、功耗和热表现尚未实测。
