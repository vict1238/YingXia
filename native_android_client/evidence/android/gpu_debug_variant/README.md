# Isolated Android GPU debug-context variant

This is a QA-only clone at `native_android_client/build/gpu_debug_client`. The shipping source, manifest, production AAR, and existing APKs are not modified. No device commands are run by this build workflow.

The input list and hashes are in `source_baseline.json`. The only Dart code change is the inserted `gpu-debug: yes` property in the cloned AndroidVideoController.create initialization map, after disabling VO and before enabling the GPU output. `unique_code_difference.patch` records that single-line change. The clone receives the existing fixture AAR; this binary test-input replacement is recorded separately in `clone_prebuild_differences.json`.

The build uses the original fixture package and both original GPU fixture constants, with `--target-platform android-x64`. Flutter/JDK/SDK/NDK/Gradle/Maven/signing paths remain external to the clone under the project's `.tools/native` directory. No signing secret is copied into this evidence directory. `build_configuration.json` records the command arguments and path configuration; `build_variant.ps1` is the exact build script.

This single-variable check addresses context creation before shader execution. The packaged mpv identifies itself as `v0.36.0-549-g78d43740f5-dirty`. Its GLES context helper passes zero context flags by default; the Google emulator guest EGL implementation rejects zero `EGL_CONTEXT_FLAGS_KHR`, while a debug bit is accepted by that validation branch. Enabling gpu-debug requests a nonzero debug-context bit. Whether the owned emulator then creates its context and executes both FSR modes must be measured after installation by the root QA workflow.

Successful output would establish this QA context's GPU/Surface/shader path. It would not establish the unmodified production GPU fixture path, ARM device performance, or a general Impeller cause.

Primary references:

- [Exact mpv context helper](https://github.com/mpv-player/mpv/blob/78d43740f5/video/out/opengl/egl_helpers.c).
- [Google gfxstream guest EGL flags validation](https://github.com/google/gfxstream/blob/main/guest/egl/egl.cpp).
- [Flutter 3.47.7 emulator renderer selection](https://github.com/flutter/flutter/blob/abaf9c523780a608bd46686fd5e53740a07077f8/engine/src/flutter/shell/platform/android/android_context_dynamic_impeller.cc).

Read `artifact.json` for the built APK path and SHA-256, `apk_badging.txt` and `apk_signature.txt` for package/signature evidence, and `preservation_check.json` for post-build source/artifact preservation checks.
