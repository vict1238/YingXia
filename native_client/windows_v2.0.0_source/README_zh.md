# Windows 2.0.0 对应的 Flutter 源码

本目录保留已验证的 `YingXia_v2.0.0_Native_Windows_x64.exe` 使用的 Flutter 源码和依赖锁定文件。Windows EXE 的 SHA-256 为 `dd2062a005e12626033ab5f4281be7a540c4616a2f9d0edae16654cbb6889f3b`，各源文件的哈希见 `MANIFEST.json`。

Android 后续渲染配置的调整不覆盖这个快照。若要重新构建该 Windows 版本，在项目副本中使用本目录的 `lib`、`pubspec.yaml`、`pubspec.lock` 替换 `native_client` 对应文件，然后按上层 README 的 Windows 构建步骤执行。Windows runner、后台和其他资源继续使用源码包内相应文件。
