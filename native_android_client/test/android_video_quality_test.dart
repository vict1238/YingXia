// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/video_quality.dart';

import 'video_quality_test.dart' as fixtures;

class AndroidProofRenderer extends fixtures.FakeRenderer {
  @override
  bool get requiresRenderVerification => true;

  QualityTarget? actualOutput = const QualityTarget(1920, 1080);

  @override
  Future<String> getProperty(String name) async {
    if (name == 'current-vo') return 'gpu';
    if (name == 'options/gpu-context') return 'android';
    if (name == 'osd-dimensions') {
      return jsonEncode({'w': actualOutput?.width, 'h': actualOutput?.height});
    }
    return super.getProperty(name);
  }
}

VideoQualityController controller(AndroidProofRenderer renderer) =>
    VideoQualityController(
      renderer: renderer,
      playbackState: () => const QualityPlaybackState(
        active: true,
        buffering: false,
        position: 10,
      ),
      automaticPolling: false,
    );

void main() {
  test('Android uses active VO and keeps absent runtime VO pending', () {
    expect(androidQualityBackend('gpu', 'gpu'), 'android-gpu');
    expect(androidQualityBackend('', 'gpu'), 'pending');
    expect(androidQualityBackend('null', 'gpu'), 'pending');
    expect(
      androidQualityBackend('mediacodec_embed', 'gpu'),
      'unsupported:mediacodec_embed',
    );
    expect(
      androidQualityBackend('', 'mediacodec_embed'),
      'unsupported:mediacodec_embed',
    );
    expect(
      androidQualityBackend('gpu', 'mediacodec_embed'),
      'unsupported:mediacodec_embed',
    );
  });

  test('startup defaults to Off on Android', () async {
    final quality = controller(AndroidProofRenderer());
    expect(quality.requested, VideoQuality.off);
    expect(quality.effective, VideoQuality.off);
    expect(quality.isUpscaling, isFalse);
    await quality.close();
  });

  test(
    'MediaCodec embed reports unsupported and never applies a shader',
    () async {
      final renderer = AndroidProofRenderer()
        ..backendState = 'unsupported:mediacodec_embed';
      final quality = controller(renderer);
      await fixtures.configure(quality);
      expect(quality.requested, VideoQuality.enhanced);
      expect(quality.effective, VideoQuality.off);
      expect(quality.fallbackReason, 'unsupported_video_output');
      expect(quality.status, contains('当前解码输出不支持超分'));
      expect(renderer.sizes, everyElement(isNull));
      expect(renderer.properties['glsl-shaders'], '');
      await quality.close();
    },
  );

  test(
    'Android accepts executing FSR passes at the actual mpv target',
    () async {
      final renderer = AndroidProofRenderer()..backendState = 'android-gpu';
      final quality = controller(renderer);
      await fixtures.configure(quality);
      for (var sample = 0; sample < 8; sample++) {
        await quality.poll();
      }
      expect(quality.effective, VideoQuality.enhanced);
      expect(renderer.properties['glsl-shaders'], 'fsr1_enhanced.glsl');
      expect(renderer.sizes.last, const QualityTarget(1920, 1080));
      await quality.select(VideoQuality.light);
      await quality.poll();
      expect(renderer.properties['glsl-shaders'], 'fsr1_light.glsl');
      await quality.select(VideoQuality.off);
      expect(renderer.properties['glsl-shaders'], '');
      expect(renderer.sizes.last, isNull);
      await quality.close();
    },
  );

  test(
    'Android cannot advertise FSR when VO pass evidence is unavailable',
    () async {
      final renderer = AndroidProofRenderer()
        ..backendState = 'android-gpu'
        ..unavailableMetrics = true;
      final quality = controller(renderer);
      await fixtures.configure(quality);
      for (var sample = 0; sample < 5; sample++) {
        await quality.poll();
      }
      expect(quality.effective, VideoQuality.off);
      expect(quality.fallbackReason, 'shader_unverified');
      expect(renderer.properties['glsl-shaders'], '');
      expect(renderer.sizes.last, isNull);
      await quality.close();
    },
  );

  test('Android restores Off if resize never reaches the mpv output', () async {
    for (final output in [null, const QualityTarget(1280, 720)]) {
      final renderer = AndroidProofRenderer()
        ..backendState = 'android-gpu'
        ..actualOutput = output;
      final quality = controller(renderer);
      await fixtures.configure(quality);
      for (var sample = 0; sample < 5; sample++) {
        await quality.poll();
      }
      expect(quality.effective, VideoQuality.off);
      expect(quality.fallbackReason, 'output_size_mismatch');
      expect(renderer.properties['glsl-shaders'], '');
      expect(renderer.sizes.last, isNull);
      await quality.close();
    }
  });
}
