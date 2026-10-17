/// This file is a part of media_kit (https://github.com/media-kit/media-kit).
///
/// Copyright © 2021 & onwards, Hitesh Kumar Saini <saini123hitesh@gmail.com>.
/// All rights reserved.
/// Use of this source code is governed by MIT license that can be found in the LICENSE file.
import 'dart:io';
import 'dart:async';
import 'dart:collection';

import 'package:flutter/services.dart';
import 'package:flutter/foundation.dart';

import 'package:media_kit/media_kit.dart';

import 'package:media_kit_video/src/utils/query_decoders.dart';
import 'package:media_kit_video/src/video_controller/platform_video_controller.dart';
import 'android_surface_lifecycle.dart';

/// {@template android_video_controller}
///
/// AndroidVideoController
/// ----------------------
///
/// The [PlatformVideoController] implementation based on native JNI & C/C++ used on Android.
///
/// {@endtemplate}
class AndroidVideoController extends PlatformVideoController {
  /// Whether [AndroidVideoController] is supported on the current platform or not.
  static bool get supported => Platform.isAndroid;

  /// Pointer address to the global object reference of `android.view.Surface` i.e. `(intptr_t)(*android.view.Surface)`.
  final ValueNotifier<int?> wid = ValueNotifier<int?>(null);

  final _surfaceLifecycle = AndroidSurfaceLifecycle();

  // Keep decoded display dimensions separate from the Surface render target.
  // Updating video metadata must not silently undo an active GLSL upscale.
  int _sourceWidth = 0;
  int _sourceHeight = 0;
  int? _outputWidth;
  int? _outputHeight;
  int? _appliedWid;
  int? _appliedWidth;
  int? _appliedHeight;
  bool _disposed = false;

  NativePlayer get platform => player.platform as NativePlayer;

  Future<void> setProperty(String key, String value) async {
    if (_disposed || platform.disposed) return;
    await platform.setProperty(key, value, waitForInitialization: false);
  }

  Future<void> setProperties(Map<String, String> properties) async {
    for (final entry in properties.entries) {
      await setProperty(entry.key, entry.value);
    }
  }

  /// Listener for updating the --wid property.
  Future<void> widListener() {
    return _surfaceLifecycle.update(_updateWidLocked, _redraw);
  }

  Future<bool> _updateWidLocked() async {
    if (_disposed || platform.disposed) return false;
    final width = rect.value?.width.toInt() ?? 1;
    final height = rect.value?.height.toInt() ?? 1;
    final currentWid = wid.value ?? 0;
    if (_appliedWid == currentWid &&
        _appliedWidth == width &&
        _appliedHeight == height) {
      return false;
    }
    final androidSurfaceSizeValue = [width, height].join('x');
    final widValue = currentWid.toString();
    // When --wid is 0, vo=null is required to avoid SIGSEGV.
    final voValue = widValue == '0' ? 'null' : configuration.vo!;
    final vidValue = widValue == '0' ? 'no' : 'auto';
    // It is important to re-initialize --vo after --android-surface-size.
    await setProperty('vo', 'null');
    await setProperties({
      // ORDER IS IMPORTANT.
      'android-surface-size': androidSurfaceSizeValue,
      'wid': widValue,
      'vo': voValue,
      // It is important to re-initialize --vid in-case of --vo=mediacodec_embed.
      // Not doing so causes error "Could not open codec." & video never gets rendered.
      if (configuration.vo == 'mediacodec_embed') 'vid': vidValue,
    });
    _appliedWid = currentWid;
    _appliedWidth = width;
    _appliedHeight = height;
    return true;
  }

  Future<void> _redraw() async {
    if (_disposed || platform.disposed || wid.value == 0) return;
    // Instead of seeking to the start (Duration.zero), seek to the current playback position
    // without jumping the user to the start of the media.
    final currentPosition = player.state.position;
    try {
      await player.seek(currentPosition);
    } catch (exception) {
      if (!_disposed && !platform.disposed) {
        debugPrint('Android Surface redraw failed: $exception');
      }
    }
  }

  /// Called with the Surface lock held. Native acknowledgement includes actual
  /// Surface dimensions and JNI reference, so setSize completes only after
  /// mpv has reconfigured its Android EGL output to that same target.
  Future<bool> _resizeSurfaceLocked() async {
    if (_disposed ||
        platform.disposed ||
        _sourceWidth <= 0 ||
        _sourceHeight <= 0) {
      return false;
    }
    final width = _outputWidth ?? _sourceWidth;
    final height = _outputHeight ?? _sourceHeight;
    if (width == rect.value?.width.toInt() &&
        height == rect.value?.height.toInt()) {
      return _updateWidLocked();
    }
    final state = await _channel.invokeMapMethod<String, dynamic>(
      'VideoOutputManager.SetSurfaceSize',
      {
        'handle': (await player.handle).toString(),
        'width': width.toString(),
        'height': height.toString(),
      },
    );
    if (_disposed || platform.disposed) return false;
    if (state == null ||
        state['width'] != width ||
        state['height'] != height ||
        state['wid'] is! int ||
        state['wid'] == 0) {
      throw StateError('Android Surface resize was not acknowledged');
    }
    rect.value = Rect.fromLTWH(0, 0, width.toDouble(), height.toDouble());
    id.value = state['id'] as int;
    wid.value = state['wid'] as int;
    return _updateWidLocked();
  }

  /// [StreamSubscription] for listening to video [Rect].
  StreamSubscription<VideoParams>? videoParamsSubscription;

  /// {@macro android_video_controller}
  AndroidVideoController._(super.player, super.configuration) {
    wid.addListener(widListener);
    videoParamsSubscription = player.stream.videoParams.listen(
      (event) => _surfaceLifecycle.update(() async {
        final int width;
        final int height;
        if ((event.rotate ?? 0) % 180 == 0) {
          width = event.dw ?? 0;
          height = event.dh ?? 0;
        } else {
          // width & height are swapped for 90 or 270 degrees rotation.
          width = event.dh ?? 0;
          height = event.dw ?? 0;
        }

        final isZero = width <= 0 || height <= 0;
        final isSame = width == _sourceWidth && height == _sourceHeight;
        if (isZero || isSame) {
          return false;
        }
        _sourceWidth = width;
        _sourceHeight = height;
        final changed = await _resizeSurfaceLocked();

        if (!waitUntilFirstFrameRenderedCompleter.isCompleted) {
          waitUntilFirstFrameRenderedCompleter.complete();
        }
        return changed;
      }, _redraw),
    );
  }

  /// {@macro android_video_controller}
  static Future<PlatformVideoController> create(
    Player player,
    VideoControllerConfiguration configuration,
  ) async {
    Future<String> getDefaultHwdec() async {
      // Enforce software rendering in emulators.
      bool hw = configuration.enableHardwareAcceleration;
      final bool isEmulator = await _channel.invokeMethod('Utils.IsEmulator');
      if (isEmulator) {
        hw = false;
        debugPrint('media_kit: Emulator detected.');
        debugPrint('media_kit: Enforcing S/W rendering.');
      }
      return hw ? 'auto-safe' : 'no';
    }

    // Update [configuration] to have default values.
    configuration = configuration.copyWith(
      vo: configuration.vo ?? 'gpu',
      hwdec: configuration.hwdec ?? await getDefaultHwdec(),
    );

    // Retrieve the native handle of the [Player].
    final handle = await player.handle;
    // Return the existing [VideoController] if it's already created.
    if (_controllers.containsKey(handle)) {
      return _controllers[handle]!;
    }

    // In case no video-decoders are found, this means media_kit_libs_***_audio is being used.
    // Thus, --vid=no is required to prevent libmpv from trying to decode video (otherwise bad things may happen).
    //
    // Search for common H264 decoder to check if video support is available.
    final decoders = await queryDecoders(handle);
    if (!decoders.contains('h264')) {
      throw UnsupportedError(
        '[VideoController] is not available.'
        ' '
        'Please use media_kit_libs_***_video instead of media_kit_libs_***_audio.',
      );
    }

    // Creation:
    final controller = AndroidVideoController._(player, configuration);

    // Register [_dispose] for execution upon [Player.dispose].
    player.platform?.release.add(controller._dispose);

    // Store the [VideoController] in the [_controllers].
    _controllers[handle] = controller;

    await _channel.invokeMethod('VideoOutputManager.Create', {
      'handle': handle.toString(),
    });

    await controller.setProperties({
      // It is necessary to set vo=null here to avoid SIGSEGV, --wid must be assigned before vo=gpu is set.
      'vo': 'null',
      'hwdec': configuration.hwdec!,
      'vid': 'auto',
      'opengl-es': 'yes',
      'force-window': 'yes',
      'gpu-context': 'android',
      'sub-use-margins': 'no',
      'sub-font-provider': 'none',
      'sub-scale-with-window': 'yes',
      'hwdec-codecs': 'h264,hevc,mpeg4,mpeg2video,vp8,vp9,av1',
    });

    // Return the [PlatformVideoController].
    return controller;
  }

  /// Sets both positive Surface output dimensions for vo=gpu. Passing neither
  /// dimension restores the decoded display size. Source metadata remains
  /// separate, including across media changes and rotation updates.
  @override
  Future<void> setSize({int? width, int? height}) =>
      _surfaceLifecycle.update(() async {
        if (_disposed || platform.disposed) return false;
        if ((width == null) != (height == null) ||
            (width != null && width <= 0) ||
            (height != null && height <= 0)) {
          throw ArgumentError('Specify both positive dimensions, or neither');
        }
        if (width != null && configuration.vo != 'gpu') {
          throw UnsupportedError('Android resizing requires vo=gpu');
        }
        _outputWidth = width;
        _outputHeight = height;
        return _resizeSurfaceLocked();
      }, _redraw);

  /// Disposes the instance. Releases allocated resources back to the system.
  Future<void> _dispose() async {
    _disposed = true;
    wid.removeListener(widListener);
    await videoParamsSubscription?.cancel();
    final handle = await player.handle;
    _controllers.remove(handle);
    await _surfaceLifecycle.drain(() async {
      await _channel.invokeMethod('VideoOutputManager.Dispose', {
        'handle': handle.toString(),
      });
    });
    wid.dispose();
    super.dispose();
  }

  /// Currently created [AndroidVideoController]s.
  static final _controllers = HashMap<int, AndroidVideoController>();

  /// [MethodChannel] for invoking platform specific native implementation.
  static final _channel = const MethodChannel(
    'com.alexmercerind/media_kit_video',
  )..setMethodCallHandler((MethodCall call) async {
      try {
        debugPrint(call.method.toString());
        debugPrint(call.arguments.toString());
        switch (call.method) {
          case 'VideoOutput.Resize':
            {
              // Notify about updated texture ID & [Rect].
              final int handle = call.arguments['handle'];
              final Rect rect = Rect.fromLTWH(
                call.arguments['rect']['left'] * 1.0,
                call.arguments['rect']['top'] * 1.0,
                call.arguments['rect']['width'] * 1.0,
                call.arguments['rect']['height'] * 1.0,
              );
              final int id = call.arguments['id'];
              final int wid = call.arguments['wid'];
              _controllers[handle]?.rect.value = rect;
              _controllers[handle]?.id.value = id;
              _controllers[handle]?.wid.value = wid;
              break;
            }
          case 'VideoOutput.WaitUntilFirstFrameRenderedNotify':
            {
              // Notify about updated texture ID & [Rect].
              final int handle = call.arguments['handle'];
              debugPrint(handle.toString());
              // Notify about the first frame being rendered.
              final completer =
                  _controllers[handle]?.waitUntilFirstFrameRenderedCompleter;
              if (!(completer?.isCompleted ?? true)) {
                completer?.complete();
              }
              break;
            }
          default:
            {
              break;
            }
        }
      } catch (exception, stacktrace) {
        debugPrint(exception.toString());
        debugPrint(stacktrace.toString());
      }
    });
}
