# Android 运行组件许可补充

本目录补充 Android Java/Kotlin 运行层的原始许可证和来源记录，与 `../flutter`、`../pub`、`../native`、`../go` 中的 Flutter/Dart、插件、libmpv/FFmpeg 和 Go 通知一起交付。

继承基线的正式 `releaseRuntimeClasspath` 已成功解析，原始报告复制为 [release_runtime_classpath_baseline.txt](../../evidence/android/release_runtime_classpath_baseline.txt)，SHA-256 `dd401c2c92fee8f7a55c98ff5877286d5bbf0383f19d39c561b1cd961f5cfce9`。客户端沿用相同锁文件及 Maven 版本配置。`runtime_dependencies.json` 记录该基线选定的 49 个 Maven 坐标、请求版本与最终版本、POM 许可声明、原始 POM 哈希及对应许可文件。其中有 33 个 AndroidX 坐标，并包含 coroutines BOM 等元数据项；此数量不代表 APK 编入 49 个独立代码库。对应原始 POM 及继承许可的父 POM 保存于 `poms/`。

Kotlin stdlib / stdlib-common 实际版本为 2.4.0，stdlib-jdk7 / jdk8 为 1.8.20，均声明 Apache-2.0。两个版本的 LICENSE / NOTICE 原文已保存。上游说明列出的 JVM 标准库来源包括 GWT、ThreeTenBP、Guava 和 Boost；其完整原始文本一起保留，具体 R8 裁剪结果由最终 APK 决定。Kotlin NOTICE 原文覆盖其编译器发行包，保存此文本不表示 APK 打包了 Kotlin 编译器。

`flutter_embedding_release.pom` 来自本次 Flutter engine `deb287481e3ce9468f3434937ced4240a70539ca` 的官方发布仓库。正式解析实际选择 AndroidX lifecycle 2.7.0、fragment 1.7.1、annotation / annotation-jvm 1.8.1、activity 1.8.1、core / core-ktx 1.13.1、window / window-java 1.2.0、exifinterface 1.4.1、profileinstaller 1.3.1 等。完整逐模块版本表在 `runtime_dependencies.json`。各 AndroidX POM 的 Apache-2.0 声明已核对；完整原文为 `LICENSE-AndroidX-reference.txt`，它来自上游主分支，是许可参考文本，不冒充具体模块源码快照。

其他实际坐标包括 ReLinker 1.4.5、kotlinx.coroutines android / core / core-jvm 1.7.1、JetBrains annotations 23.0.0、JSpecify 1.0.0、Guava listenablefuture 1.0，均为 Apache-2.0；各自版本锁定许可、POM 声明和原始版权文本已保留。ReLinker 文本含 Copyright 2015 KeepSafe Software Inc.；Guava 原始版权头从 listenablefuture 1.0 的正式 source JAR 提取，其许可从父 POM guava-parent 26.0-android 继承。早期下载的 coroutines 1.9.0 文本仅保留为参考，实际运行映射使用 1.7.1 原文。

下载 URL、文件大小和 SHA-256 见 `license-reference-manifest.json`，目录全部文件哈希见 `file_manifest.json`。依赖报告成功及本次许可核对属于构建依赖证据；APK 安装、JNI 启动、界面、清空记录和解码验证仍以各自实际运行报告为准。
