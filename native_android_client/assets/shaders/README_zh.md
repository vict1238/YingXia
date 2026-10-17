# Android 原生版画质着色器

`fsr1_enhanced.glsl` 使用 AMD FSR 1 的 EASU 边缘自适应放大及 RCAS 对比度自适应锐化。EASU 保留上游完整的 12 个采样、方向估算、各向异性 Lanczos 近似、四邻域去振铃；第二个独立画面处理阶段执行五采样 RCAS，并启用上游的噪声适应。

`fsr1_light.glsl` 使用播放器原有的双线性放大，再执行单阶段、五采样、较低强度的 RCAS。轻量档不执行 EASU，便于降低处理负担。增强档的 RCAS 使用 1.0 stop，轻量档使用 1.5 stop（数值越大，锐化越轻）。关闭档由界面控制器清空自定义着色器。

来源为 AMD 官方 MIT 许可代码，固定提交 `a21ffb8f6c13233ba336352bdff293894c706575`：

- https://github.com/GPUOpen-Effects/FidelityFX-FSR/tree/a21ffb8f6c13233ba336352bdff293894c706575
- `upstream/ffx_fsr1.h` 是官方 EASU/RCAS 算法原件。
- `upstream/ffx_a.h` 是官方兼容及数学辅助原件。
- `LICENSE_AMD_FSR1.txt` 是同一提交的 MIT 许可证；两个执行着色器也保留完整许可。

移植使用普通片段着色器及 FP32，不需要计算着色器、FP16、`textureGather` 或整数位运算。12 个 RGB 像素中心采样替代上游每个通道的 gather 访问；带零除保护的精确倒数和逆平方根替代上游位运算近似。公式、采样布局、去振铃和 RCAS 限幅保持对应关系。全黑、全白等平坦区域的零除保护避免 NaN。

EASU 在 mpv 的 `MAIN` 阶段执行，输出尺寸取 `OUTPUT.w/h`，其条件要求源图像两个方向均小于输出图像。RCAS 在 `SCALED` 阶段执行，由界面依据实际播放区及视频尺寸提供放大条件：只有存在有效放大目标时才加载本脚本，改回原尺寸之前先清空自定义着色器。RCAS 不额外引用未保存的 `NATIVE` 纹理，也不为条件检查增加一个预处理纹理。HDR PQ/HLG 输入由界面禁用本链路，因为此处 EASU 位于 mpv 的 HDR 色调映射之前；原播放及色调映射保留。

mpv 应使用 `scale=bilinear`、`cscale=bilinear`、`fbo-format=rgba8`、`gpu-dumb-mode=no`、`interpolation=no`。必须调整实际播放器纹理尺寸，单纯更换 Flutter 控件布局不会让 mpv 执行 720p 到 1080p 的放大。生效时的实际 `vo-passes` 描述为：

- `YingXia FSR1 EASU`
- `YingXia FSR1 RCAS Enhanced`
- `YingXia FSR1 RCAS Light`

Android 使用本工程中的 `media_kit_video` 2.0.1 MIT 源码。GPU 输出按真实 Surface 尺寸设置 mpv `android-surface-size`，等待 Surface 与 JNI 引用确认后恢复 VO。控制器同时核对 `current-vo`、`osd-dimensions` 与实际 `vo-passes`，未执行着色器、输出尺寸无法确认或输出不支持时回退关闭。模拟器默认采用不运行本链路的 `mediacodec_embed`，会显示具体回退原因。

继承的 Windows 渲染桥接包含 ANGLE 上下文和 advanced control 修改，作为 vendored 插件的非 Android 平台源码保留。其历史线程审计见 [render_loop_audit.md](../../evidence/fsr_shader/render_loop_audit.md)。Android 直接查询当前 mpv VO 和处理阶段，未使用 Windows 插件通道。

历史 Windows 工具在 ANGLE pbuffer 中编译 GLSL 100/300，将 1280×720 合成 SDR 纹理处理为 1920×1080，并检查每阶段 8,210 个 RGB 样本及平坦区域。该工具不属于 Android 构建；继承结果见 [angle_rgb_validation.json](../../evidence/fsr_shader/angle_rgb_validation.json) 与 [证据范围说明](../../evidence/fsr_shader/README_zh.md)。Android 本轮 GPU 检查按 [采证说明](../../tool/android_gpu/README.md) 独立记录，实际运行以发布验证记录为准；Android ARM 真机性能仍需测量。
