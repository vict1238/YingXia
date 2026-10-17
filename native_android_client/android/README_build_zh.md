# Android 2.2 原生宿主与构建

应用包名为 `com.yingxia.yingxia_native`，版本来自 `../pubspec.yaml`，最低 API26，包含 arm64-v8a、armeabi-v7a、x86_64。完整步骤与工具版本见 [客户端 README](../README_zh.md) 和 [Go 后台 README](../../native_android_core/README_zh.md)。

`MainActivity` 通过 `yingxia/core` MethodChannel 启动 gomobile JNI 后台，数据位于应用私有目录。退出通过 JNI 与 Activity 所有权检查执行，旧 Activity 的释放不能关闭替换后的宿主。后台只监听随机本机端口，会话与 CSRF 凭据通过私有连接描述交给 Dart，不写入证据。

同一通道的 `saveDiagnosticReport` 只接收最多 2 MiB 的 JSON 对象；文件类型固定为 `application/json`，默认文件名固定为 `yingxia_diagnostics.json`。系统 `ACTION_CREATE_DOCUMENT` 返回用户选择的单个 content URI，独立文件线程负责写入，取消返回 false；不接收路径、不保留 URI grant、不申请存储权限，也不阻塞 JNI 后台的宿主串行队列。

正式 Gradle 依赖为 `app/libs/yingxia_core.aar`，由 `../tools/build_android.ps1` 从 `../../native_android_core/build/yingxia_core.aar` 复制。该文件必须与后台 `evidence/android_core_build.json` 的正式哈希一致。正式包使用外置发布签名，设置 `YINGXIA_ANDROID_KEY_PROPERTIES` 指向源码外的签名 properties；不使用调试签名。

```powershell
# 在客户端根目录执行，工具链和签名均位于源码之外。
flutter pub get
./tools/build_android.ps1 -Online
```

构建候选输出至 `../build/candidates/YingXia_v2.2.0_Android.apk`，脚本调用 `apksigner verify --verbose --print-certs` 核验签名。发布前仍需对最终相同字节完成安装与运行验证。

模拟器仅按 ranchu/goldfish 或 Google sdk_gphone 标识启用兼容的 MediaCodec 直出；手机使用默认 GPU 输出。停止时保留 Surface 至解码器确认，重播等待停止，新媒体依照 media_kit 自身流程关闭旧流。进入后台暂停，回前台不自动播放。

合成媒体测试使用独立包名 `com.yingxia.yingxia_native.fixture`，`-Fixture` 选择测试 AAR，脚本在构建结束后恢复正式 AAR。GPU 探针必须再带 `-QaGpu`；两项 Dart QA define 默认关闭。测试 APK 不属于生产交付，真实设备性能以对应设备的实际报告为准。
