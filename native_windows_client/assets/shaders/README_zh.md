# Windows 原生版画质着色器

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

Windows 内嵌的公开 `media_kit_video` 2.0.1 插件仅在本工程中修改：ANGLE 优先尝试 GLES 3 配置及上下文，失败时保留原 GLES 2 配置。mpv 的高级处理需要 RG 纹理；无法实际执行着色器或进入软件渲染时，由控制器关闭增强。应用没有修改全局插件缓存或 Android 客户端。

为读取实际 `vo-passes` 及处理耗时，Windows 渲染桥接还启用了 mpv advanced control。更新回调始终在独立渲染线程处理，暂停时也服务控制请求；该线程的同步核心属性查询已移除，销毁顺序保证先释放 mpv 渲染上下文、后释放 GL 上下文。后台类型通过独立原子状态查询返回界面，软件渲染时关闭增强。具体源码检查见 `evidence/fsr_shader/render_loop_audit.md`。

`tools/validate_fsr_shader.py` 在与发布版相同的 ANGLE 库上建立独立 pbuffer，编译 GLSL 100/300，并将 1280×720 合成 SDR 纹理处理为 1920×1080，检查每个处理阶段的 8,210 个 RGB 样本、边缘及全黑/白/灰区域。结果在 `evidence/fsr_shader/angle_rgb_validation.json`。该检查是着色器编译及像素算法检查，播放吞吐、切页暂停及自动降档另见实际播放器检查。当前设备仅公开 RTX 4060，不能将其速度结果当作核显实测。
