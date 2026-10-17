# 从源码构建映匣

以下步骤对应当前 Windows 原生版和 Android 2.3.0。各客户端目录的 README 保留更详细的组件说明与历史记录，版本号和输出文件名以当前脚本为准。SDK、依赖缓存和 Android 签名材料由构建者在仓库外准备。

## 工具与依赖

| 工具 | 本版使用的版本 | 用途 |
|---|---|---|
| Go | 1.27.2；模块声明最低 1.26.0 | Windows 后台、启动器与 Android JNI 后台 |
| Flutter / Dart | 3.47.7 / 3.13.5 | 两端原生界面 |
| MSVC / Windows SDK | 14.44.35207 / 10.0.26100.0 | Windows 客户端与原生插件 |
| CMake / Ninja | 3.31.8 / 外置 Ninja | 提供的 Windows 便携构建脚本 |
| JDK | 17；脚本布局使用 17.0.20.1+1 | Android |
| Android SDK / Build Tools | SDK 36 / 36.0.0 | Android |
| Android NDK | 28.2.13676358 | Go JNI、Android 原生组件 |
| gomobile / gobind | `v0.0.0-20261011034528-197770444867` | 绑定 Android AAR，由后台脚本按固定版本构建 |

Go 依赖保存在各后台的 `vendor/`；Flutter 依赖由 `pubspec.lock` 固定，本地修改的 `media_kit_video` 位于客户端 `vendor/`。首次构建需下载未缓存的 Flutter、Gradle、Maven 和播放器组件；离线构建须预先准备对应缓存。Gradle wrapper 固定为 9.3.1，Android Gradle Plugin 为 9.1.0，Kotlin 为 2.4.0。

提供的便携脚本使用以下仓库外目录布局。下文假设其根目录为 `D:\YingXiaTools`；此路径仅为示例，可替换为自己的绝对路径。

```text
D:\YingXiaTools\
├─ next/go/bin/go.exe
└─ native/
   ├─ flutter_source/bin/flutter.bat
   ├─ msvc/setup_x64.bat
   ├─ msvc_redist/                 Windows 可再发行 DLL
   ├─ cmake/cmake-3.31.8-windows-x86_64/bin/cmake.exe
   ├─ ninja/ninja.exe
   ├─ jdk/jdk-17.0.20.1+1/
   └─ android_sdk/                 含 build-tools/36.0.0 与 ndk/28.2.13676358
```

`msvc/setup_x64.bat` 应初始化已安装或自行准备的 x64 MSVC 与 Windows SDK 环境。编译工具和 Microsoft 运行库须从其官方发行渠道取得，并按相应条款使用。

## Windows 原生版

在仓库根目录的 PowerShell 中执行：

```powershell
$taskTools = 'D:\YingXiaTools'
$taskGo = Join-Path $taskTools 'next/go/bin/go.exe'
$taskFlutter = Join-Path $taskTools 'native/flutter_source/bin/flutter.bat'
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
New-Item -ItemType Directory -Force releases, native_windows_core/build | Out-Null

Push-Location native_windows_core
& $taskGo build -mod=vendor -trimpath '-ldflags=-s -w -H windowsgui' -o build/yingxia_backend.exe ./cmd/native_backend
Pop-Location

Push-Location native_windows_client
& $taskFlutter pub get
./build_windows_native.ps1 -ToolsRoot (Join-Path $taskTools 'native')
./launcher/build_launcher.ps1 -BundleRoot ./build/windows/native_bundle -GoExe $taskGo -OutputExe ../releases/YingXia_v2.3.0_Native_Windows_x64_local.exe
Pop-Location
```

客户端脚本编译 Flutter / C++ 插件，再加入 Go 后台、运行组件和许可；启动器生成包含这些文件的单 EXE，首次启动释放并核对文件哈希。启动器拒绝覆盖已有输出文件，后续构建请使用新文件名。

默认会从当前播放器源码编译。`-ReuseRendererDll` 是用于复用既有固定哈希组件的可选参数，普通源码构建无需使用。只有修改启动器图标或资源时，才需要先运行 `launcher/build_resources.ps1` 重新生成 `.syso`。

## Android：正式 AAR 与自有签名

先在仓库根目录构建生产后台，`-ToolsRoot` 接收上表的工具根目录：

```powershell
$taskTools = 'D:\YingXiaTools'
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
Push-Location native_android_core
./build_android_aar.ps1 -ToolsRoot $taskTools
Pop-Location
```

输出为 `native_android_core/build/yingxia_core.aar`，包含 API 26 的 `arm64-v8a`、`armeabi-v7a`、`x86_64` JNI。脚本按 `go.mod` 固定版本构建 gomobile 与 gobind；正式构建不传 `-Fixture` 或 `native_fixture` 标签。

创建自己的签名密钥并在提示时设置密码。密钥、密码和 properties 文件保存在仓库外：

```powershell
New-Item -ItemType Directory -Force 'D:\YingXiaSigning' | Out-Null
& (Join-Path $taskTools 'native/jdk/jdk-17.0.20.1+1/bin/keytool.exe') -genkeypair -v -keystore 'D:/YingXiaSigning/yingxia-release.jks' -alias yingxia -keyalg RSA -keysize 2048 -validity 10000
```

在 `D:\YingXiaSigning\android-key.properties` 中填写以下字段，示例中的占位文字应替换为自己设置的值：

```properties
storeFile=D:/YingXiaSigning/yingxia-release.jks
storePassword=<自己的密钥库密码>
keyAlias=yingxia
keyPassword=<自己的密钥密码>
```

自己的签名 APK 无法覆盖使用官方签名的已安装 APK；保管同一密钥用于自己后续版本的升级。不要将签名文件、密码、`local.properties` 或工具缓存提交到 Git。

设置工具路径后构建：

```powershell
$env:YINGXIA_FLUTTER = Join-Path $taskTools 'native/flutter_source/bin/flutter.bat'
$env:JAVA_HOME = Join-Path $taskTools 'native/jdk/jdk-17.0.20.1+1'
$env:ANDROID_HOME = Join-Path $taskTools 'native/android_sdk'
$env:GRADLE_USER_HOME = Join-Path $taskTools 'native/gradle_home'
$env:YINGXIA_ANDROID_KEY_PROPERTIES = 'D:\YingXiaSigning\android-key.properties'

Push-Location native_android_client
& $env:YINGXIA_FLUTTER pub get
./tools/build_android.ps1 -Online
Pop-Location
```

脚本复制正式 AAR，并调用 `apksigner` 验证签名。候选输出为 `native_android_client/build/candidates/YingXia_v2.3.0_Android.apk`。示例采用脚本的 `-Online` 入口；首次构建需网络，离线构建须另行准备完整缓存并设置对应工具的离线选项。`-Fixture` 与 `-QaGpu` 供独立测试包使用，不用于正式交付。

## 检查与历史版本

在相应 Go 后台目录运行 `go test -mod=vendor ./...` 和 `go vet -mod=vendor ./...`；在相应 Flutter 客户端运行 `flutter analyze` 和 `flutter test`。使用隔离测试目录，避免改动日常收藏、历史和设置。构建与单元测试完成后，还应对最终相同字节进行目标平台的安装、启动和播放检查；现有发布验证记录位于 [`../releases/`](../releases/)。

历史 Windows WebView 版仅需 Go 构建，运行时需要系统提供 Microsoft WebView2 Runtime。在 `current_source/` 中：

```powershell
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -mod=vendor -trimpath '-ldflags=-H windowsgui -s -w' -o YingXia_v1.3.2_Windows_x64_local.exe .
```

Python 原型使用 Python 3.10+ 标准库及电脑已安装的 Edge / Chrome，在 `legacy_python/` 中运行 `py -3 main.py`，或使用 `START.bat`。历史原生 2.0 的步骤见 [`native_client/README_zh.md`](../native_client/README_zh.md) 与 [`native_core/README_zh.md`](../native_core/README_zh.md)。

原生播放器、FSR、字体、运行库与依赖的来源和许可详见各客户端的 `THIRD_PARTY_NOTICES.md`；源码归档还保留相应构建配方与源码记录。
