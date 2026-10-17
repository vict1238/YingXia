# Android 2.2 验证记录范围

最终交付的生产 APK、fixture APK、实际检查项目与结果以源码包顶层 `YingXia_v2.2.0_Android_VALIDATION.json` 为准。最终生产界面/运行报告保存在 `runtime_final/`，必须核对其 APK SHA-256 与发布记录相同。分析、Flutter 测试和构建日志分别为 `flutter_analyze.log`、`flutter_tests.log` 与对应构建日志。

`runtime/`、`runtime_overview.log` 及调试当前画面是首轮开发候选记录，源码打包工具排除这些文件，不能用它们替代最终相同字节的验收。其余阶段日志可能早于最终编译；应用安装或某次构建通过只能证明该次结果。

`immutable_baseline.json` 记录此前 Android 与 Windows 发布件，说明新分支保留历史原件。`release_runtime_classpath_baseline.txt` 是所继承的依赖版本基线，供许可证来源核对，不属于本轮 APK 运行测试。

`gpu_probe_smoke/` 仅为 GPU 采证脚本的只读 smoke 检查。实际 GPU 结果仍需满足 [GPU 采证步骤](../../tool/android_gpu/README.md) 中的 VO、执行阶段和实际输出尺寸条件。模拟器默认使用 MediaCodec 直出，请求超分时回退关闭；Android ARM 真机持续性能、功耗和热表现尚未实测。

后台测试、正式 AAR/JNI 来源与哈希位于 [`../../../native_android_core/evidence`](../../../native_android_core/evidence)。来源表筛选和历史 GLSL 算法检查位于 `../windows_2_2/source_screening` 与 `../fsr_shader`，其记录保留各自原检查时间和适用范围。
