# Android 原生版

客户端使用 Flutter 原生渲染和 media_kit 播放器，Go JNI 后台位于 `app/libs/yingxia_core.aar`。不依赖电脑，不包含 WebView、Chrome 或 JavaScript 播放器。包名 `com.yingxia.yingxia_native`，最低 Android 8.0（API 26），通用 APK 包含 `armeabi-v7a`、`arm64-v8a`、`x86_64`。

Go 后台通过 `yingxia/core` Flutter 通道在应用私有目录启动，连接信息只返回当前客户端。HTTP 接口监听本机回环随机端口，保留会话、CSRF 和 Host/Origin 检查。网络安全配置仅允许 `127.0.0.1`、`localhost` 明文连接，外部连接保持 TLS 校验。切换页面、应用进入后台时暂停播放。

Android 退出也只经过宿主通道，由进程内串行队列和 Activity 所有权检查执行；旧 Activity 销毁或旧 Dart 连接释放时，不直接调用 HTTP 退出接口关闭替换后的宿主。桥接仅对 ranchu/goldfish 或 Google sdk_gphone 模拟器标识返回 `emulator=true`。该环境使用 `mediacodec_embed` / `mediacodec` 播放输出，绕开已确认的模拟器 EGL 上下文错误；手机仍使用播放器默认输出。相关上游问题见 [media-kit #1343](https://github.com/media-kit/media-kit/issues/1343)。

停止操作会保留视频表面，等原生播放器确认停止后才移除；下一次播放等待该操作完成。打开新媒体依照 media_kit 自身的停止流程，不在外部重复停止。加载期间不把旧解码器回调计入新影片的播放记录。

当前构建环境为 Flutter 3.47.7、JDK 17、Android SDK 36、NDK 28.2.13676358。Gradle 与插件版本固定在 wrapper/settings 文件，Flutter依赖固定在上层 `pubspec.lock`。先构建 `native_core` 根包的 Android AAR，再放入 `app/libs`。

正式包采用独立发布签名，不能使用调试签名。设置环境变量 `YINGXIA_ANDROID_KEY_PROPERTIES` 指向保存在源码外的私有 Properties 文件，字段为 `storeFile`、`storePassword`、`keyAlias`、`keyPassword`。不要把签名文件、密码、私钥放入源码包。之后在 `native_client` 运行：

```powershell
flutter build apk --release --target-platform android-arm,android-arm64,android-x64
```

Gradle 正式输出位于 `build/app/outputs/apk/release/app-release.apk`，构建脚本从该位置复制发布 APK，再使用 SDK 的 `apksigner verify --verbose --print-certs` 检查签名。后续升级 APK 应使用同一发布签名，以保留已安装应用和本地数据。

动态网页只返回页面、无法静态解析媒体地址的片源不在原生播放器中打开，用户可选择其他来源。片源配置更新不自动更换 APK。安装和播放验证以本轮保存的实际检查记录为准。

最终发布 APK 已在自建 API 35 x86_64 模拟器上完成 21 项实际运行检查：界面与后台启动、清空取消/确认/重启持久化及收藏保留、Activity 替换，以及相同发布 AOT/播放器的合成 WebM/HLS 解码、暂停、换集、停止重播和正常结束。正式包 Go JNI 经比对确认来自正式 AAR，合成影片仅在独立测试 APK 中。具体证据及验证范围见 `../evidence/android/android_runtime_tests.json`；本轮没有 Android ARM 真机验证。
