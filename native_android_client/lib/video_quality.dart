// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';
import 'dart:convert';
import 'dart:ffi';
import 'dart:io';
import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:ffi/ffi.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit/generated/libmpv/bindings.dart' as mpv;
import 'package:media_kit_video/media_kit_video.dart';

import 'mpv_nodes.dart';

enum VideoQuality {
  off('关闭'),
  light('轻量'),
  enhanced('增强');

  const VideoQuality(this.label);
  final String label;

  static VideoQuality parse(Object? value) =>
      values.firstWhere((mode) => mode.name == value, orElse: () => off);
}

/// Native playback preferences are separate from the shared Go library format.
class VideoQualityPreferences {
  VideoQualityPreferences(this.directory);
  final String directory;
  Future<void> _tail = Future.value();

  File get _file =>
      File('$directory${Platform.pathSeparator}video_quality.json');

  Future<VideoQuality> load() async {
    if (directory.isEmpty) return VideoQuality.off;
    try {
      if (!await _file.exists() || await _file.length() > 4096) {
        return VideoQuality.off;
      }
      final data = jsonDecode(await _file.readAsString());
      // Version 1 stored the last temporary playback choice. Version 2 stores
      // only an explicit startup default, so old automatic Light choices do not
      // silently enable upscaling after this upgrade.
      return VideoQuality.parse(
        data is Map && data['version'] == 2 ? data['default_mode'] : null,
      );
    } catch (_) {
      return VideoQuality.off;
    }
  }

  Future<void> save(VideoQuality mode) {
    if (directory.isEmpty) return Future.value();
    final result = _tail.then((_) async {
      await Directory(directory).create(recursive: true);
      final temporary = File('${_file.path}.$pid.tmp');
      try {
        await temporary.writeAsString(
          jsonEncode({'version': 2, 'default_mode': mode.name}),
          flush: true,
        );
        // Only this small platform-local preference file is replaced.
        if (await _file.exists()) await _file.delete();
        await temporary.rename(_file.path);
      } finally {
        if (await temporary.exists()) await temporary.delete();
      }
    });
    _tail = result.catchError((Object _) {});
    return result;
  }
}

@immutable
class QualityTarget {
  const QualityTarget(this.width, this.height);
  final int width;
  final int height;
  Map<String, int> toJson() => {'width': width, 'height': height};
  @override
  bool operator ==(Object other) =>
      other is QualityTarget && width == other.width && height == other.height;
  @override
  int get hashCode => Object.hash(width, height);
}

/// The actual mpv texture, rather than a Flutter-only stretch, is enlarged.
/// The viewport uses physical pixels and keeps the source display aspect.
QualityTarget? qualityTarget({
  required int sourceWidth,
  required int sourceHeight,
  required double viewportWidth,
  required double viewportHeight,
  required VideoQuality mode,
}) {
  if (mode == VideoQuality.off ||
      sourceWidth <= 0 ||
      sourceHeight <= 0 ||
      !viewportWidth.isFinite ||
      !viewportHeight.isFinite ||
      viewportWidth <= 0 ||
      viewportHeight <= 0) {
    return null;
  }
  final aspect = sourceWidth / sourceHeight;
  var width = math.min(viewportWidth, viewportHeight * aspect);
  var height = width / aspect;
  // Large displays must not accidentally quadruple the shader workload.
  final cap = math.min(1.0, math.min(1920 / width, 1080 / height));
  width *= cap;
  height *= cap;
  if (width <= sourceWidth + 1 || height <= sourceHeight + 1) return null;
  return QualityTarget(width.round(), height.round());
}

/// Counts only VO rendering drops; decoder drops and callback frequency are
/// deliberately excluded. Slow/buffered/paused/seek samples reset the window.
class QualityDropPolicy {
  QualityDropPolicy({
    this.warmup = const Duration(seconds: 4),
    this.window = const Duration(seconds: 4),
    this.requiredBadWindows = 2,
  });
  final Duration warmup;
  final Duration window;
  final int requiredBadWindows;
  Duration? _readyAfter;
  Duration? _windowStart;
  Duration? _lastTime;
  double? _lastPosition;
  int? _firstDrops;
  int? _lastDrops;
  int _badWindows = 0;
  int _costSamples = 0;
  int _slowCostSamples = 0;
  String? lastReason;

  void reset(Duration now) {
    _readyAfter = now + warmup;
    _windowStart = null;
    _lastTime = null;
    _lastPosition = null;
    _firstDrops = null;
    _lastDrops = null;
    _badWindows = 0;
    _costSamples = 0;
    _slowCostSamples = 0;
    lastReason = null;
  }

  bool sample({
    required Duration now,
    required double position,
    required bool active,
    required bool buffering,
    required double rate,
    int? voDrops,
    double? sourceFps,
    double? renderMilliseconds,
  }) {
    if (_readyAfter == null) reset(now);
    final elapsed =
        _lastTime == null ? null : (now - _lastTime!).inMicroseconds / 1000000;
    final advance = _lastPosition == null ? null : position - _lastPosition!;
    // A pause or seek is not evidence that the GPU cannot keep up.
    final discontinuity =
        elapsed != null &&
        advance != null &&
        (elapsed <= 0 ||
            elapsed > 3 ||
            advance < -0.15 ||
            advance > elapsed * math.max(rate, 1) * 1.8 + 0.5);
    if (!active || buffering || discontinuity || !position.isFinite) {
      reset(now);
      return false;
    }
    _lastTime = now;
    _lastPosition = position;
    if (now < _readyAfter!) return false;
    // No timeline progress generally means a stall, not a shader bottleneck.
    if (advance != null && advance <= 0.001) {
      reset(now);
      return false;
    }
    if (_lastDrops != null && voDrops != null && voDrops < _lastDrops!) {
      reset(now);
      return false;
    }
    _lastDrops = voDrops;
    _windowStart ??= now;
    _firstDrops ??= voDrops;
    final validFps =
        sourceFps != null &&
        sourceFps.isFinite &&
        sourceFps >= 5 &&
        sourceFps <= 120 &&
        rate > 0;
    if (validFps &&
        renderMilliseconds != null &&
        renderMilliseconds.isFinite &&
        renderMilliseconds >= 0) {
      _costSamples++;
      if (renderMilliseconds > 1000 / (sourceFps * rate) * 0.9) {
        _slowCostSamples++;
      }
    }
    if (now - _windowStart! < window) return false;
    final seconds = (now - _windowStart!).inMicroseconds / 1000000;
    final delta =
        voDrops == null || _firstDrops == null ? null : voDrops - _firstDrops!;
    // Avoid interpreting expected high-FPS/display mismatch as GPU overload.
    final badDrops =
        validFps &&
        sourceFps * rate <= 60 &&
        delta != null &&
        delta >= 3 &&
        delta / (sourceFps * rate * seconds) >= 0.02;
    final slowRender =
        _costSamples >= 3 && _slowCostSamples / _costSamples >= 0.75;
    _badWindows = badDrops || slowRender ? _badWindows + 1 : 0;
    lastReason =
        slowRender
            ? 'render_budget'
            : badDrops
            ? 'vo_frame_drops'
            : null;
    _windowStart = now;
    _firstDrops = voDrops;
    _costSamples = 0;
    _slowCostSamples = 0;
    if (_badWindows < requiredBadWindows) return false;
    _badWindows = 0;
    return true;
  }
}

@immutable
class QualityPlaybackState {
  const QualityPlaybackState({
    required this.active,
    required this.buffering,
    required this.position,
    this.rate = 1,
  });
  final bool active;
  final bool buffering;
  final double position;
  final double rate;
}

abstract class QualityRenderer {
  /// Android's Surface path is accepted only with executing shader and actual
  /// mpv output-size evidence. Missing metrics must not imply FSR is active.
  bool get requiresRenderVerification => false;
  Future<String> getProperty(String name);
  Future<void> setProperty(String name, String value);
  Future<void> setSize(QualityTarget? target);
  Future<String> shaderPath(VideoQuality mode);
  Future<void> watchSeeking(VoidCallback onSeeking) async {}
  Future<void> close() async {}
  Future<String> backend() async => 'unknown';
}

/// current-vo is an mpv runtime property; gpu-context is only an option and
/// cannot establish that a GPU VO was actually created. A temporarily absent
/// current-vo stays pending until active-frame pass verification settles it.
String androidQualityBackend(String currentVo, String configuredVo) {
  final actual = currentVo.trim();
  if (configuredVo != 'gpu') return 'unsupported:$configuredVo';
  if (actual.isEmpty || actual == 'null') return 'pending';
  return actual == 'gpu' ? 'android-gpu' : 'unsupported:$actual';
}

class MpvQualityRenderer implements QualityRenderer {
  MpvQualityRenderer(this.player, this.video, this.dataDirectory);
  static const _channel = MethodChannel('com.alexmercerind/media_kit_video');
  final Player player;
  final VideoController video;
  final String dataDirectory;
  final Map<VideoQuality, String> _paths = {};
  Future<void>? _watching;
  bool _observedSeeking = false;
  bool _closing = false;

  @override
  bool get requiresRenderVerification => Platform.isAndroid;

  NativePlayer get _native {
    final platform = player.platform;
    if (platform is! NativePlayer) throw StateError('不支持当前播放器');
    return platform;
  }

  @override
  Future<String> getProperty(String name) async {
    final native = _native;
    if (name != 'vo-passes' && name != 'osd-dimensions') {
      return native.getProperty(name);
    }
    await native.waitForPlayerInitialization;
    await native.waitForVideoControllerInitializationIfAttached;
    if (native.disposed) throw StateError('播放器已关闭');
    final property = name.toNativeUtf8();
    final node = calloc<mpv.mpv_node>();
    var result = -1;
    try {
      result = native.mpv.mpv_get_property(
        native.ctx,
        property.cast(),
        mpv.mpv_format.MPV_FORMAT_NODE,
        node.cast<Void>(),
      );
      if (result < 0) {
        return jsonEncode({'status': 'unavailable', 'error_code': result});
      }
      return jsonEncode(copyMpvNode(node.ref));
    } finally {
      if (result >= 0) native.mpv.mpv_free_node_contents(node);
      calloc.free(node);
      calloc.free(property);
    }
  }

  @override
  Future<void> setProperty(String name, String value) =>
      _native.setProperty(name, value);
  @override
  Future<void> setSize(QualityTarget? target) =>
      video.setSize(width: target?.width, height: target?.height);
  @override
  Future<String> shaderPath(VideoQuality mode) async {
    if (_paths.containsKey(mode)) return _paths[mode]!;
    final name =
        mode == VideoQuality.enhanced
            ? 'fsr1_enhanced.glsl'
            : 'fsr1_light.glsl';
    final content = await rootBundle.load('assets/shaders/$name');
    final directory = Directory(
      '$dataDirectory${Platform.pathSeparator}video-shaders-v2.1.0',
    );
    await directory.create(recursive: true);
    final file = File('${directory.path}${Platform.pathSeparator}$name');
    await file.writeAsBytes(
      content.buffer.asUint8List(content.offsetInBytes, content.lengthInBytes),
      flush: true,
    );
    return _paths[mode] = file.path;
  }

  @override
  Future<void> watchSeeking(VoidCallback onSeeking) =>
      _watching ??= () async {
        try {
          final native = _native;
          await native.waitForPlayerInitialization;
          await native.waitForVideoControllerInitializationIfAttached;
          if (_closing || native.disposed) return;
          await native.observeProperty('seeking', (value) async {
            if (!_closing && value == 'yes') onSeeking();
          });
          _observedSeeking = true;
        } catch (_) {
          // Older mpv builds retain timeline discontinuity detection as fallback.
        }
      }();

  @override
  Future<void> close() async {
    _closing = true;
    await _watching;
    if (_observedSeeking && !_native.disposed) {
      try {
        await _native.unobserveProperty('seeking');
      } catch (_) {}
    }
  }

  @override
  Future<String> backend() async {
    if (_closing) return 'pending';
    try {
      if (Platform.isAndroid) {
        final platform = await video.platform.future;
        String currentVo;
        try {
          currentVo = await _native.getProperty('current-vo');
        } catch (_) {
          currentVo = '';
        }
        return androidQualityBackend(
          currentVo,
          platform.configuration.vo ?? 'gpu',
        );
      }
      return await _channel.invokeMethod<String>(
            'VideoOutputManager.GetRendererBackend',
            {'handle': (await player.handle).toString()},
          ) ??
          'pending';
    } catch (_) {
      return 'unknown';
    }
  }
}

/// All renderer mutations run in order; stale viewport callbacks cannot apply
/// a previous mode after a user selection, stop, or source change.
class VideoQualityController extends ChangeNotifier {
  VideoQualityController({
    required this.renderer,
    required this.playbackState,
    VideoQuality initial = VideoQuality.off,
    QualityDropPolicy? policy,
    Duration Function()? clock,
    this.diagnosticFile,
    this.diagnosticToLog = false,
    bool automaticPolling = true,
  }) : requested = initial,
       effective = initial,
       _policy = policy ?? QualityDropPolicy(),
       _clock = clock ?? (() => _watch.elapsed) {
    _watch.start();
    unawaited(renderer.watchSeeking(suspend));
    if (automaticPolling) {
      _timer = Timer.periodic(
        const Duration(seconds: 1),
        (_) => unawaited(poll()),
      );
    }
  }
  final QualityRenderer renderer;
  final QualityPlaybackState Function() playbackState;
  final File? diagnosticFile;

  /// Explicit fixture-only console evidence; excludes media URLs and headers.
  final bool diagnosticToLog;
  final QualityDropPolicy _policy;
  final Duration Function() _clock;
  static final Stopwatch _watch = Stopwatch();
  VideoQuality requested;
  VideoQuality effective;
  String fallbackReason = '';
  QualityTarget? target;
  QualityTarget? output;
  int _sourceWidth = 0;
  int _sourceHeight = 0;
  String _gamma = '';
  double _viewportWidth = 0;
  double _viewportHeight = 0;
  double _normalWidth = 0;
  double _normalHeight = 0;
  bool _fullscreen = false;
  bool _disposed = false;
  bool _polling = false;
  bool _applied = false;
  bool _shaderVerified = false;
  int _missingPasses = 0;
  int _wrongOutputSamples = 0;
  int _generation = 0;
  int? _previousDrops;
  Timer? _timer;
  Timer? _debounce;
  Future<void> _tail = Future.value();
  Future<void> _logTail = Future.value();

  bool get isUpscaling => target != null && effective != VideoQuality.off;
  String get status {
    if (fallbackReason.isEmpty) return '';
    final reason = switch (fallbackReason) {
      'unsupported_video_output' => '当前解码输出不支持超分',
      'renderer_unavailable' => '渲染器不可用',
      'shader_unverified' => '无法验证着色器',
      'shader_not_active' => '着色器未运行',
      'output_size_mismatch' => '输出尺寸未生效',
      'hdr' => 'HDR 视频',
      'software_renderer' || 'dumb_renderer' => '当前渲染器不支持超分',
      _ => '',
    };
    return '当前${effective.label} · 已降档${reason.isEmpty ? '' : ' · $reason'}';
  }

  Future<void> select(VideoQuality mode) {
    requested = mode;
    effective = mode;
    fallbackReason = '';
    _shaderVerified = false;
    _missingPasses = 0;
    _wrongOutputSamples = 0;
    _policy.reset(_clock());
    notifyListeners();
    return _apply('selection');
  }

  void beginMedia() {
    _generation++;
    _sourceWidth = 0;
    _sourceHeight = 0;
    _gamma = '';
    effective = requested;
    fallbackReason = '';
    _shaderVerified = false;
    _missingPasses = 0;
    _wrongOutputSamples = 0;
    _previousDrops = null;
    output = null;
    target = null;
    _policy.reset(_clock());
    _applied = false;
    unawaited(_apply('media_reset'));
    notifyListeners();
  }

  void suspend() {
    if (_disposed) return;
    _policy.reset(_clock());
    _previousDrops = null;
    _log('suspended');
  }

  void setSource(int width, int height, {String? gamma}) {
    if (_disposed) return;
    final updatedGamma = gamma ?? '';
    final changed =
        width != _sourceWidth ||
        height != _sourceHeight ||
        updatedGamma != _gamma;
    _sourceWidth = width;
    _sourceHeight = height;
    _gamma = updatedGamma;
    if (!changed) return;
    _policy.reset(_clock());
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 150), () {
      unawaited(_apply('source_metadata'));
    });
  }

  void setViewport(double width, double height, {required bool fullscreen}) {
    if (_disposed || !width.isFinite || !height.isFinite) return;
    if (!fullscreen) {
      _normalWidth = width;
      _normalHeight = height;
      if (_fullscreen) return;
    }
    if ((width - _viewportWidth).abs() < 1 &&
        (height - _viewportHeight).abs() < 1) {
      return;
    }
    _viewportWidth = width;
    _viewportHeight = height;
    _policy.reset(_clock());
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 150), () {
      unawaited(_apply('viewport'));
    });
  }

  void setFullscreen(bool value) {
    _fullscreen = value;
    if (!value) setViewport(_normalWidth, _normalHeight, fullscreen: false);
  }

  void recordOutput(int width, int height) {
    if (_disposed || width <= 0 || height <= 0) return;
    final updated = QualityTarget(width, height);
    if (output == updated) return;
    output = updated;
    _log('output');
  }

  void shaderError(String prefix, String message) {
    if (_disposed || !isUpscaling) return;
    final text = '$prefix $message'.toLowerCase();
    final shaderFailure =
        (text.contains('shader') || text.contains('glsl')) &&
        (text.contains('error') ||
            text.contains('failed') ||
            text.contains('cannot'));
    final software =
        text.contains('software rendering') ||
        text.contains('dumb-mode') && text.contains('forcing');
    if (shaderFailure || software) {
      unawaited(
        _fallback(
          VideoQuality.off,
          shaderFailure ? 'shader_error' : 'software_renderer',
        ),
      );
    }
  }

  Future<void> _fallback(VideoQuality mode, String reason) async {
    if (_disposed || effective == mode) return;
    effective = mode;
    fallbackReason = reason;
    _policy.reset(_clock());
    _previousDrops = null;
    notifyListeners();
    await _apply(reason);
  }

  Future<void> _apply(String trigger) {
    final generation = ++_generation;
    final result = _tail
        .then((_) async {
          if (_disposed || generation != _generation) return;
          final hdr = _gamma == 'pq' || _gamma == 'hlg' || _gamma == 'st2084';
          if (hdr && effective != VideoQuality.off) {
            effective = VideoQuality.off;
            fallbackReason = 'hdr';
            notifyListeners();
          }
          var next = qualityTarget(
            sourceWidth: _sourceWidth,
            sourceHeight: _sourceHeight,
            viewportWidth: _viewportWidth,
            viewportHeight: _viewportHeight,
            mode: effective,
          );
          String? backend;
          if (next != null) {
            backend = await renderer.backend();
            if (_disposed || generation != _generation) return;
            if (backend == 'software' || backend.startsWith('unsupported:')) {
              effective = VideoQuality.off;
              fallbackReason =
                  backend == 'software'
                      ? 'software_renderer'
                      : 'unsupported_video_output';
              next = null;
              notifyListeners();
            }
          }
          // Shader options must be set on the same libmpv render context as Video.
          await renderer.setProperty('glsl-shaders', '');
          if (_disposed || generation != _generation) return;
          if (next != null) {
            await renderer.setProperty('fbo-format', 'rgba8');
            await renderer.setProperty('gpu-dumb-mode', 'no');
            await renderer.setProperty('scale', 'bilinear');
            await renderer.setProperty('cscale', 'bilinear');
            await renderer.setProperty('interpolation', 'no');
          } else {
            // A previous SDR enhancement must not force an 8-bit intermediate
            // texture for a later HDR stream or for the normal/off setting.
            await renderer.setProperty('fbo-format', 'auto');
            await renderer.setProperty('gpu-dumb-mode', 'auto');
          }
          await renderer.setSize(next);
          if (_disposed || generation != _generation) return;
          if (next != null) {
            final path = await renderer.shaderPath(effective);
            if (_disposed || generation != _generation) return;
            await renderer.setProperty('glsl-shaders', path);
          }
          target = next;
          _applied = true;
          _shaderVerified = false;
          _missingPasses = 0;
          _wrongOutputSamples = 0;
          _policy.reset(_clock());
          _log('applied', {'trigger': trigger, 'renderer': backend});
        })
        .catchError((Object error) async {
          // Silent setProperty is not treated as proof that a shader executed.
          if (!_disposed && generation == _generation) {
            effective = VideoQuality.off;
            fallbackReason = 'renderer_unavailable';
            target = null;
            notifyListeners();
            _log('fallback', {'trigger': 'renderer_unavailable'});
            // Cleanup belongs to this serialized operation. A new selection must
            // not be cleared later by a failed older operation.
            try {
              await renderer.setProperty('glsl-shaders', '');
            } catch (_) {}
            try {
              await renderer.setProperty('fbo-format', 'auto');
              await renderer.setProperty('gpu-dumb-mode', 'auto');
            } catch (_) {}
            try {
              await renderer.setSize(null);
            } catch (_) {}
          }
        });
    _tail = result;
    return result;
  }

  Future<void> poll() async {
    if (_disposed || _polling || !_applied || _sourceWidth <= 0) return;
    _polling = true;
    final generation = _generation;
    try {
      final state = playbackState();
      if (!state.active || state.buffering) {
        _policy.reset(_clock());
        _previousDrops = null;
        _log('inactive', {
          'active': state.active,
          'buffering': state.buffering,
        });
        return;
      }
      Future<String> read(String name) async {
        try {
          return await renderer.getProperty(name);
        } catch (_) {
          return '';
        }
      }

      final values = await Future.wait([
        read('frame-drop-count'),
        read('decoder-frame-drop-count'),
        read('estimated-vf-fps'),
        read('container-fps'),
        read('vo-passes'),
        read('gpu-dumb-mode'),
        read('hwdec-current'),
        renderer.backend().catchError((Object _) => 'unknown'),
        read('current-vo'),
        read('options/gpu-context'),
        read('osd-dimensions'),
      ]);
      if (_disposed || generation != _generation) return;
      final drops = int.tryParse(values[0]);
      final fps = double.tryParse(values[2]) ?? double.tryParse(values[3]);
      List<Map<String, dynamic>> passes = [];
      String passStatus = 'missing';
      try {
        final parsed = values[4].length > 262144 ? null : jsonDecode(values[4]);
        final fresh = parsed is Map ? parsed['fresh'] : null;
        if (parsed is Map) {
          passStatus =
              parsed['status']?.toString() ??
              (fresh is List ? 'available' : 'unexpected_node');
          if (parsed['error_code'] != null) {
            passStatus = '$passStatus:${parsed['error_code']}';
          }
        }
        if (fresh is List) {
          passes =
              fresh
                  .take(64)
                  .whereType<Map>()
                  .map(
                    (entry) => {
                      'desc': entry['desc']?.toString() ?? '',
                      'avg': entry['avg'],
                      'last': entry['last'],
                      'count': entry['count'],
                    },
                  )
                  .toList();
        }
      } catch (_) {}
      final renderNanoseconds = passes.fold<double>(
        0,
        (sum, entry) =>
            sum + (entry['avg'] is num ? (entry['avg'] as num).toDouble() : 0),
      );
      final delta =
          drops == null || _previousDrops == null || drops < _previousDrops!
              ? null
              : drops - _previousDrops!;
      _previousDrops = drops;
      QualityTarget? renderOutput;
      try {
        final dimensions =
            values[10].length > 4096 ? null : jsonDecode(values[10]);
        if (dimensions is Map &&
            dimensions['w'] is num &&
            dimensions['h'] is num) {
          final width = (dimensions['w'] as num).toInt();
          final height = (dimensions['h'] as num).toInt();
          if (width > 0 && height > 0) {
            renderOutput = QualityTarget(width, height);
          }
        }
      } catch (_) {}
      if (isUpscaling && values[7].startsWith('unsupported:')) {
        await _fallback(VideoQuality.off, 'unsupported_video_output');
        return;
      }
      if (isUpscaling && values[7] == 'software') {
        await _fallback(VideoQuality.off, 'software_renderer');
        return;
      }
      if (isUpscaling && values[5] == 'yes') {
        await _fallback(VideoQuality.off, 'dumb_renderer');
        return;
      }
      if (isUpscaling &&
          (passes.isNotEmpty || renderer.requiresRenderVerification)) {
        final names = passes.map((entry) => entry['desc'].toString()).toList();
        final expected =
            effective == VideoQuality.enhanced
                ? ['YingXia FSR1 EASU', 'YingXia FSR1 RCAS Enhanced']
                : ['YingXia FSR1 RCAS Light'];
        _shaderVerified = expected.every(
          (name) => names.any((n) => n.contains(name)),
        );
        _missingPasses = _shaderVerified ? 0 : _missingPasses + 1;
        if (_missingPasses >= 5) {
          _log('verification_failed', {
            'vo_passes_status': passStatus,
            'vo_pass_count': passes.length,
            'current_vo': values[8],
            'gpu_context_option': values[9],
            'mpv_output': renderOutput?.toJson(),
          });
          await _fallback(
            VideoQuality.off,
            passes.isEmpty ? 'shader_unverified' : 'shader_not_active',
          );
          return;
        }
      }
      if (isUpscaling && renderer.requiresRenderVerification) {
        final matches = renderOutput == target;
        _wrongOutputSamples = matches ? 0 : _wrongOutputSamples + 1;
        if (_wrongOutputSamples >= 5) {
          _log('verification_failed', {
            'current_vo': values[8],
            'mpv_output': renderOutput?.toJson(),
            'vo_pass_count': passes.length,
          });
          await _fallback(VideoQuality.off, 'output_size_mismatch');
          return;
        }
      }
      final lower = _policy.sample(
        now: _clock(),
        position: state.position,
        active: state.active && isUpscaling,
        buffering: state.buffering,
        rate: state.rate,
        voDrops: drops,
        sourceFps: fps,
        renderMilliseconds: passes.isEmpty ? null : renderNanoseconds / 1000000,
      );
      _log('sample', {
        'active': state.active,
        'buffering': state.buffering,
        'position': state.position,
        'vo_drops': drops,
        'vo_drop_delta': delta,
        'decoder_drops': int.tryParse(values[1]),
        'fps': fps,
        'rate': state.rate,
        'render_ms': passes.isEmpty ? null : renderNanoseconds / 1000000,
        'vo_passes': passes,
        'vo_pass_count': passes.length,
        'vo_passes_status': passStatus,
        'shader_verified': _shaderVerified,
        'hwdec': values[6],
        'renderer': values[7],
        'current_vo': values[8],
        'gpu_context_option': values[9],
        'mpv_output': renderOutput?.toJson(),
        'gamma': _gamma,
      });
      if (lower && effective != VideoQuality.off) {
        await _fallback(
          effective == VideoQuality.enhanced
              ? VideoQuality.light
              : VideoQuality.off,
          _policy.lastReason ?? 'render_budget',
        );
      }
    } finally {
      _polling = false;
    }
  }

  void _log(String event, [Map<String, dynamic> extra = const {}]) {
    if (diagnosticFile == null && !diagnosticToLog) return;
    final recordData = <String, dynamic>{
      'time': DateTime.now().toUtc().toIso8601String(),
      'generation': _generation,
      'event': event,
      'requested': requested.name,
      'effective': effective.name,
      'source': {'width': _sourceWidth, 'height': _sourceHeight},
      'viewport': {'width': _viewportWidth, 'height': _viewportHeight},
      'target': target?.toJson(),
      'output': output?.toJson(),
      'fallback': fallbackReason,
      ...extra,
    };
    final record = jsonEncode(recordData);
    if (diagnosticToLog) {
      // Keep each Android log entry below logcat's per-entry size limit.
      // File diagnostics retain the full pass graph; console evidence carries
      // the FSR passes plus the complete graph's count and total render cost.
      final console = <String, dynamic>{...recordData};
      final passes = console['vo_passes'];
      if (passes is List) {
        console['vo_passes'] =
            passes
                .where(
                  (pass) =>
                      pass is Map &&
                      (pass['desc']?.toString().contains('YingXia') ?? false),
                )
                .toList();
        console['vo_passes_scope'] = 'yingxia_fsr';
      }
      debugPrint('YINGXIA_FSR_QA ${jsonEncode(console)}');
    }
    if (diagnosticFile == null) return;
    _logTail = _logTail
        .then((_) async {
          await diagnosticFile!.parent.create(recursive: true);
          await diagnosticFile!.writeAsString(
            '$record\n',
            mode: FileMode.append,
          );
        })
        .catchError((Object _) {});
  }

  Future<void> close() async {
    _disposed = true;
    _generation++;
    _timer?.cancel();
    _debounce?.cancel();
    await _tail;
    await renderer.close();
    await _logTail;
    super.dispose();
  }
}
