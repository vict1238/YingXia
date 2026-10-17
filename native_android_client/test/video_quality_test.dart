// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/video_quality.dart';

class FakeRenderer extends QualityRenderer {
  final Map<String, String> properties = {};
  final List<QualityTarget?> sizes = [];
  int drops = 0;
  int decoderDrops = 0;
  double fps = 24;
  double renderMs = 1;
  bool unavailableMetrics = false;
  bool missingShader = false;
  String backendState = 'hardware';
  void Function()? seeking;
  Completer<void>? failNext;
  Completer<void>? cleanup;

  @override
  Future<String> getProperty(String name) async {
    if (unavailableMetrics) return '';
    switch (name) {
      case 'frame-drop-count':
        return '$drops';
      case 'decoder-frame-drop-count':
        return '$decoderDrops';
      case 'estimated-vf-fps':
        return '$fps';
      case 'container-fps':
        return '$fps';
      case 'gpu-dumb-mode':
        return 'no';
      case 'vo-passes':
        final shader = properties['glsl-shaders'] ?? '';
        final names =
            missingShader
                ? ['scale']
                : shader.contains('enhanced')
                ? ['YingXia FSR1 EASU', 'YingXia FSR1 RCAS Enhanced']
                : shader.contains('light')
                ? ['YingXia FSR1 RCAS Light']
                : ['scale'];
        return jsonEncode({
          'fresh':
              names
                  .map(
                    (name) => {
                      'desc': name,
                      'avg': renderMs * 1000000 / names.length,
                      'count': 32,
                    },
                  )
                  .toList(),
        });
      default:
        return properties[name] ?? '';
    }
  }

  @override
  Future<void> setProperty(String name, String value) async {
    if (name == 'glsl-shaders' && value.isNotEmpty && failNext != null) {
      final pending = failNext!;
      failNext = null;
      await pending.future;
      throw StateError('shader unavailable');
    }
    if (name == 'glsl-shaders' && value.isEmpty && cleanup != null) {
      final pending = cleanup!;
      cleanup = null;
      await pending.future;
    }
    properties[name] = value;
  }

  @override
  Future<void> setSize(QualityTarget? target) async => sizes.add(target);
  @override
  Future<String> shaderPath(VideoQuality mode) async =>
      'fsr1_${mode.name}.glsl';

  @override
  Future<String> backend() async => backendState;
  @override
  Future<void> watchSeeking(void Function() callback) async =>
      seeking = callback;
}

Future<void> configure(
  VideoQualityController controller, {
  VideoQuality mode = VideoQuality.enhanced,
  String gamma = 'bt.1886',
}) async {
  controller.setSource(1280, 720, gamma: gamma);
  controller.setViewport(1920, 1080, fullscreen: true);
  await controller.select(mode);
}

void main() {
  test(
    'pending renderer waits, software renderer disables shader and restores source',
    () async {
      final renderer = FakeRenderer()..backendState = 'pending';
      final controller = VideoQualityController(
        renderer: renderer,
        playbackState:
            () => const QualityPlaybackState(
              active: true,
              buffering: false,
              position: 1,
            ),
        automaticPolling: false,
      );
      await configure(controller);
      expect(controller.effective, VideoQuality.enhanced);
      renderer.backendState = 'software';
      await controller.poll();
      expect(controller.requested, VideoQuality.enhanced);
      expect(controller.effective, VideoQuality.off);
      expect(controller.fallbackReason, 'software_renderer');
      expect(renderer.properties['glsl-shaders'], '');
      expect(renderer.sizes.last, isNull);
      await controller.close();
    },
  );
  test(
    'native seeking observer resets a short seek even between polling samples',
    () async {
      final renderer = FakeRenderer();
      var now = Duration.zero;
      final controller = VideoQualityController(
        renderer: renderer,
        playbackState:
            () => QualityPlaybackState(
              active: true,
              buffering: false,
              position: now.inSeconds.toDouble(),
            ),
        clock: () => now,
        automaticPolling: false,
      );
      await configure(controller);
      for (var second = 0; second <= 8; second++) {
        now = Duration(seconds: second);
        renderer.drops = second * 5;
        await controller.poll();
      }
      renderer.seeking!();
      for (var second = 9; second <= 16; second++) {
        now = Duration(seconds: second);
        renderer.drops = second * 5;
        await controller.poll();
      }
      expect(controller.effective, VideoQuality.enhanced);
      await controller.close();
    },
  );
  test('physical render target is 1080p, aspect preserving and capped', () {
    QualityTarget? target(int w, int h, double vw, double vh) => qualityTarget(
      sourceWidth: w,
      sourceHeight: h,
      viewportWidth: vw,
      viewportHeight: vh,
      mode: VideoQuality.enhanced,
    );
    expect(target(1280, 720, 1920, 1080), const QualityTarget(1920, 1080));
    expect(target(1280, 720, 3840, 2160), const QualityTarget(1920, 1080));
    expect(target(960, 720, 1920, 1080), const QualityTarget(1440, 1080));
    expect(target(1280, 720, 736, 414), isNull);
    expect(target(1920, 1080, 3840, 2160), isNull);
    expect(
      qualityTarget(
        sourceWidth: 1280,
        sourceHeight: 720,
        viewportWidth: 1920,
        viewportHeight: 1080,
        mode: VideoQuality.off,
      ),
      isNull,
    );
    expect(target(1280, 720, double.infinity, 1080), isNull);
  });

  test('sustained actual VO drops require warmup and two complete windows', () {
    final policy = QualityDropPolicy();
    final decisions = <bool>[];
    for (var second = 0; second <= 12; second++) {
      decisions.add(
        policy.sample(
          now: Duration(seconds: second),
          position: second.toDouble(),
          active: true,
          buffering: false,
          rate: 1,
          voDrops: second * 5,
          sourceFps: 24,
        ),
      );
    }
    expect(decisions.take(12), everyElement(isFalse));
    expect(decisions.last, isTrue);
    expect(policy.lastReason, 'vo_frame_drops');
  });

  test(
    'pause, seek, buffer and counter reset clear accumulated bad windows',
    () {
      for (final interruption in ['pause', 'seek', 'buffer', 'counter']) {
        final policy = QualityDropPolicy();
        for (var second = 0; second <= 8; second++) {
          expect(
            policy.sample(
              now: Duration(seconds: second),
              position: second.toDouble(),
              active: true,
              buffering: false,
              rate: 1,
              voDrops: second * 5,
              sourceFps: 24,
            ),
            isFalse,
          );
        }
        expect(
          policy.sample(
            now: const Duration(seconds: 9),
            position: interruption == 'seek' ? 100 : 9,
            active: interruption != 'pause',
            buffering: interruption == 'buffer',
            rate: 1,
            voDrops: interruption == 'counter' ? 0 : 45,
            sourceFps: 24,
          ),
          isFalse,
        );
        for (var second = 10; second <= 17; second++) {
          expect(
            policy.sample(
              now: Duration(seconds: second),
              position:
                  interruption == 'seek'
                      ? 91 + second.toDouble()
                      : second.toDouble(),
              active: true,
              buffering: false,
              rate: 1,
              voDrops:
                  interruption == 'counter' ? (second - 9) * 5 : second * 5,
              sourceFps: 24,
            ),
            isFalse,
            reason: interruption,
          );
        }
      }
    },
  );

  test(
    'slow fresh-frame GPU passes degrade; sparse and missing metrics do not',
    () {
      final slow = QualityDropPolicy();
      bool lower = false;
      for (var second = 0; second <= 12; second++) {
        lower = slow.sample(
          now: Duration(seconds: second),
          position: second.toDouble(),
          active: true,
          buffering: false,
          rate: 1,
          sourceFps: 30,
          renderMilliseconds: 35,
        );
      }
      expect(lower, isTrue);
      expect(slow.lastReason, 'render_budget');
      for (final fps in [null, 1.0, 24.0, 120.0]) {
        final sparse = QualityDropPolicy();
        for (var second = 0; second <= 60; second++) {
          expect(
            sparse.sample(
              now: Duration(seconds: second),
              position: second.toDouble(),
              active: true,
              buffering: false,
              rate: 1,
              sourceFps: fps,
              voDrops: fps == 120 ? second * 5 : null,
            ),
            isFalse,
          );
        }
      }
    },
  );

  test(
    'controller applies Enhanced, Light and Off to the actual texture',
    () async {
      final renderer = FakeRenderer();
      final controller = VideoQualityController(
        renderer: renderer,
        playbackState:
            () => const QualityPlaybackState(
              active: true,
              buffering: false,
              position: 1,
            ),
        automaticPolling: false,
      );
      await configure(controller);
      expect(renderer.sizes.last, const QualityTarget(1920, 1080));
      expect(renderer.properties['glsl-shaders'], 'fsr1_enhanced.glsl');
      controller.recordOutput(1920, 1080);
      expect(controller.output, const QualityTarget(1920, 1080));
      await controller.select(VideoQuality.light);
      expect(renderer.properties['glsl-shaders'], 'fsr1_light.glsl');
      await controller.select(VideoQuality.off);
      expect(renderer.properties['glsl-shaders'], '');
      expect(renderer.properties['fbo-format'], 'auto');
      expect(renderer.sizes.last, isNull);
      await controller.close();
    },
  );

  test(
    'actual controller automatically lowers Enhanced to Light to Off',
    () async {
      final renderer = FakeRenderer();
      var now = Duration.zero;
      final controller = VideoQualityController(
        renderer: renderer,
        playbackState:
            () => QualityPlaybackState(
              active: true,
              buffering: false,
              position: now.inSeconds.toDouble(),
            ),
        clock: () => now,
        automaticPolling: false,
      );
      await configure(controller);
      for (var second = 0; second <= 12; second++) {
        now = Duration(seconds: second);
        renderer.drops = second * 5;
        await controller.poll();
      }
      expect(controller.requested, VideoQuality.enhanced);
      expect(controller.effective, VideoQuality.light);
      expect(renderer.properties['glsl-shaders'], 'fsr1_light.glsl');
      for (var second = 13; second <= 24; second++) {
        now = Duration(seconds: second);
        renderer.drops = second * 5;
        await controller.poll();
      }
      expect(controller.effective, VideoQuality.off);
      expect(controller.status, '当前关闭 · 已降档');
      expect(renderer.properties['glsl-shaders'], '');
      await controller.close();
    },
  );

  test(
    'decoder drops and unavailable metrics cannot lower controller mode',
    () async {
      final renderer = FakeRenderer();
      var now = Duration.zero;
      final controller = VideoQualityController(
        renderer: renderer,
        playbackState:
            () => QualityPlaybackState(
              active: true,
              buffering: false,
              position: now.inSeconds.toDouble(),
            ),
        clock: () => now,
        automaticPolling: false,
      );
      await configure(controller);
      for (var second = 0; second <= 30; second++) {
        now = Duration(seconds: second);
        renderer.decoderDrops = second * 50;
        await controller.poll();
      }
      expect(controller.effective, VideoQuality.enhanced);
      renderer.unavailableMetrics = true;
      for (var second = 31; second <= 60; second++) {
        now = Duration(seconds: second);
        await controller.poll();
      }
      expect(controller.effective, VideoQuality.enhanced);
      await controller.close();
    },
  );

  test(
    'HDR and missing executing shader passes restore normal renderer',
    () async {
      final hdrRenderer = FakeRenderer();
      final hdr = VideoQualityController(
        renderer: hdrRenderer,
        playbackState:
            () => const QualityPlaybackState(
              active: true,
              buffering: false,
              position: 1,
            ),
        automaticPolling: false,
      );
      await configure(hdr, gamma: 'pq');
      expect(hdr.effective, VideoQuality.off);
      expect(hdr.fallbackReason, 'hdr');
      expect(hdrRenderer.properties['fbo-format'], 'auto');
      expect(hdrRenderer.properties['glsl-shaders'], '');
      await hdr.close();
      final renderer = FakeRenderer()..missingShader = true;
      final controller = VideoQualityController(
        renderer: renderer,
        playbackState:
            () => const QualityPlaybackState(
              active: true,
              buffering: false,
              position: 1,
            ),
        automaticPolling: false,
      );
      await configure(controller);
      for (var poll = 0; poll < 5; poll++) {
        await controller.poll();
      }
      expect(controller.effective, VideoQuality.off);
      expect(controller.fallbackReason, 'shader_not_active');
      expect(renderer.properties['glsl-shaders'], '');
      await controller.close();
    },
  );

  test(
    'fullscreen target ignores stale window layout and returns on exit',
    () async {
      final renderer = FakeRenderer();
      final controller = VideoQualityController(
        renderer: renderer,
        playbackState:
            () => const QualityPlaybackState(
              active: false,
              buffering: false,
              position: 0,
            ),
        automaticPolling: false,
      );
      controller.setSource(1280, 720, gamma: 'bt.1886');
      controller.setViewport(736, 414, fullscreen: false);
      await controller.select(VideoQuality.enhanced);
      expect(controller.target, isNull);
      controller.setFullscreen(true);
      controller.setViewport(1920, 1080, fullscreen: true);
      controller.setViewport(736, 414, fullscreen: false);
      await controller.select(VideoQuality.enhanced);
      expect(controller.target, const QualityTarget(1920, 1080));
      controller.setFullscreen(false);
      await controller.select(VideoQuality.enhanced);
      expect(controller.target, isNull);
      await controller.close();
    },
  );

  test(
    'preference persistence changes only the separate Windows preference',
    () async {
      final directory = await Directory.systemTemp.createTemp(
        'yingxia-quality-',
      );
      try {
        final library = File(
          '${directory.path}${Platform.pathSeparator}library.json',
        );
        await library.writeAsString('{"history":[1],"favorites":[2]}');
        final preferences = VideoQualityPreferences(directory.path);
        expect(await preferences.load(), VideoQuality.off);
        final qualityFile = File(
          '${directory.path}${Platform.pathSeparator}video_quality.json',
        );
        await qualityFile.writeAsString('{"version":1,"mode":"light"}');
        expect(await preferences.load(), VideoQuality.off);
        await qualityFile.writeAsString('{"version":1,"mode":"enhanced"}');
        expect(await preferences.load(), VideoQuality.off);
        await preferences.save(VideoQuality.enhanced);
        expect(
          await VideoQualityPreferences(directory.path).load(),
          VideoQuality.enhanced,
        );
        expect(await library.readAsString(), '{"history":[1],"favorites":[2]}');
        await preferences.save(VideoQuality.off);
        expect(await preferences.load(), VideoQuality.off);
      } finally {
        await directory.delete(recursive: true);
      }
    },
  );
}
