# Android FSR GPU fixture evidence

This probe targets the separately named fixture APK (`com.yingxia.yingxia_native.fixture`). The production emulator path continues to use `mediacodec_embed`; that output cannot run the FSR GLSL hooks and reports Off when enhancement is requested.

## Build isolation

Use the Android 2.2 build helper's GPU fixture option. Its Flutter invocation must include both:

```text
--dart-define=YINGXIA_PLAYER_FIXTURE=true
--dart-define=YINGXIA_QA_ANDROID_GPU=true
```

Both constants default to false. Together they select Android `vo=gpu`, `hwdec=no`, and console diagnostics. The matching fixture AAR and APK package remain the build helper's responsibility. The normal fixture and production APK use their normal emulator strategy.

Do not run parallel Flutter builds. Keep the production, normal fixture, and GPU fixture APKs separately named. The normal fixture is the baseline for video playback, pause, stop/replay, episode changes, foreground/background, and completed playback.

## Runtime procedure

The owned emulator is `emulator-5560` (API 35, x86_64, SwiftShader). Record a UTC start time before launching the installed GPU fixture:

```powershell
$gpuStartedUtc = [datetime]::UtcNow
```

1. In the fixture app, search for `native`, open the HLS test result, and play its continuous 30-second clip. Keep playback active long enough for five one-second verification samples.
2. Enter fullscreen or use a viewport whose physical size exceeds the source. Select Enhanced, then Light, then Off. A source already at or above the bounded viewport does not require an upscale and cannot prove shader execution.
3. When the GPU path renders, capture samples for each setting. Repeat stop/replay, episode switch, fullscreen exit, and background/foreground to check that Surface recreation preserves playback and that Off restores source output dimensions. The synthetic fixture uses local authenticated relay media, so no external network is needed for playback.
4. Collect metrics without `adb root`, `run-as`, changing protected settings, or reading app-private files:

```powershell
./tool/android_gpu/Collect-FsrEvidence.ps1 `
  -Serial emulator-5560 `
  -SinceUtc $gpuStartedUtc `
  -OutputDirectory ./evidence/android/gpu
```

The collector does not install, launch, stop, or clear the app, and does not clear logcat. It reads selected device properties and the current fixture process log. If the process exited, it reads selected tags and retains only the fixture's structured metrics plus counts of fixed GPU error phrases. It saves `fsr_samples.jsonl` and `summary.json`; it does not save raw URLs, authentication headers, cookies, or an unrestricted device log.

## What counts as a successful GPU check

For each enhancement mode, the same active sample must show:

- `current_vo: gpu` and `renderer: android-gpu`.
- `shader_verified: true`, with RCAS Light for Light, or EASU plus RCAS Enhanced for Enhanced.
- Actual `mpv_output` dimensions matching `target`, capped at 1920×1080. `output` alone is Flutter's Surface size and does not prove mpv rendered at that resolution.
- Source dimensions smaller than the requested upscale target. Console `vo_passes` contains only YingXia shader passes to stay below Android's log entry limit; `vo_pass_count` and `render_ms` describe the complete fresh-frame pass graph.

Off should show an empty target and actual mpv output equal to source display dimensions. The report marks `light_enhanced_off_verified` only when it sees all three states. Dropped-frame policy evidence comes from VO drops and total GPU render time; decoder drops, paused/buffering/seek intervals, and unavailable timing data do not establish GPU overload.

## Unavailable GPU paths

Guest EGL may fail before the first frame with `EGL_BAD_NATIVE_WINDOW`, `Could not create EGL surface`, or another GPU context error. Missing executing shader passes and unconfirmed output dimensions also trigger Off after five active samples. Preserve the resulting report and treat it as an unavailable GPU simulation path. Continue normal fixture playback checks using `mediacodec_embed`.

Successful SwiftShader output verifies this emulator's shader/Surface integration. It does not measure ARM hardware throughput, battery use, heat, or real-phone GPU driver behavior. A real-device run using the same evidence criteria remains the performance acceptance check.

## Implementation references

The vendor plugin retains its MIT license, and FSR shaders retain their AMD MIT attribution. Android resizing follows mpv's existing Surface lifecycle order: disable VO, set `android-surface-size` and `wid`, then recreate the GPU VO. The native Surface acknowledges actual dimensions and JNI reference before `setSize` completes.

Surface mutations and native resource release use one serialized queue. Redraw seeks run after releasing that queue's lock: the native player's disposal holds its own player lock while draining Surface resources, so awaiting a seek while holding the Surface lock would create a lock inversion. Disposal still waits for in-flight Surface mutations. The lifecycle tests reproduce a blocked redraw during native disposal, verify resize acknowledgement precedes resource release, and check that a failed resize leaves disposal drainable.

- [mpv runtime properties](https://mpv.io/manual/stable/#property-list): `current-vo`, `osd-dimensions`, and `vo-passes`.
- [mpv Android EGL context](https://github.com/mpv-player/mpv/blob/v0.37.0/video/out/opengl/context_android.c): Android reconfiguration uses the Surface dimensions for the render target.
- [mpv Android Surface dimensions](https://github.com/mpv-player/mpv/blob/v0.37.0/video/out/android_common.c): configured `android-surface-size` takes precedence over the native window's inferred size.
