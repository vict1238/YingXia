# 映匣 Android 原生版 2.2.0

安装 `YingXia_v2.2.0_Android.apk`，支持 Android 8.0 及以上。通用 APK 包含 arm64-v8a、armeabi-v7a、x86_64。可覆盖更新此前同签名的 Android 2.0，保留应用数据。手机独立运行，无需电脑后台或浏览器。

以 Windows 2.2 为基线，保留内置 47 个来源、访问检测与可取消的搜索进度、来源测速、查找筛选、分页、启停、自定义来源管理，以及 88lin/Kazumi 手动与自动更新。更新后实际来源数量会变化。来源测速表示接口响应，具体媒体入口继续由逐片访问检测判断。

播放工作台保留选集、切换片源、倍速、全屏、暂停、停止重播及平台绑定。切页或进入后台时暂停；回到前台等待手动继续。收藏、历史和设置存放在应用私有目录，清空记录保留收藏。手机竖屏及短横屏采用底部导航；空播放区保留选集提示，实际播放使用下方按钮。

超分默认关闭，可临时选择关闭／轻量／增强，也可在来源与诊断设置独立的下次启动默认档位。轻量为双线性放大加 RCAS，增强为 EASU 加 RCAS；实际 Surface 输出按比例放大、最高 1920×1080，持续渲染压力会自动降档。设备输出不支持或无法核实着色器执行时回退关闭。Android GPU 支持与持续性能依设备而定；本轮未做 ARM 真机、耗电或发热测试。没有插帧。

开发人员：翻翻蓄小激光（cqh.vict1238@gmail.com）、流风驻尘闲（yuchihaobo@gmail.com）。诊断报告可通过 Android 系统保存窗口导出 JSON。

验证记录包含 49 项 Dart 测试、Android 实际 Go 测试（14 个引擎测试、5 个子例与 1 个生命周期测试）、正式 APK 与隔离合成视频的安装运行检查。合成测试 APK 的 12 个界面与解码原生库与正式 APK 逐字节一致。运行范围和 GPU 探针条件详见 `YingXia_v2.2.0_Android_VALIDATION.json` 与源码中的原始证据。

APK 大小：124,905,502 bytes；SHA-256：`423289feea2dccb2d9ebd019e73d8a12d3b81b4bb9ba6b1253289ea4e6133e23`。包名 `com.yingxia.yingxia_native`，版本码 20200，发布签名与旧版相同。源码、许可与构建说明在 `YingXia_v2.2.0_Android_Source.zip`；完整依赖通知在源码 `native_android_client/THIRD_PARTY_NOTICES.md`。
