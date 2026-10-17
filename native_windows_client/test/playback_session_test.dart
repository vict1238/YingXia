// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/playback_session.dart';

void main() {
  test('packet cache has bounded mobile memory and no growing disk cache', () {
    for (final mode in PlaybackCache.values) {
      final props = mode.properties(mobile: true);
      expect(props['cache-on-disk'], 'no');
      expect(mode.mobileMiB, lessThanOrEqualTo(80));
      expect(props['cache-pause-initial'], 'yes');
      expect(mode.restartSeconds, greaterThanOrEqualTo(3));
    }
  });
  test(
    'resume rejects nonfinite, negative, excessive and completed positions',
    () {
      for (final value in [double.nan, double.infinity, -1, 700000]) {
        expect(boundedResume(value, Duration.zero), Duration.zero);
      }
      expect(boundedResume(95, const Duration(seconds: 100)), Duration.zero);
      expect(
        boundedResume(42.75, const Duration(seconds: 100)),
        const Duration(milliseconds: 42750),
      );
    },
  );
  test('continuous buffering prompts once and respects cooldown', () {
    final monitor = PlaybackStallMonitor()..reset(Duration.zero);
    var prompts = 0;
    for (var i = 0; i <= 60; i++) {
      if (monitor.observe(
        now: Duration(seconds: i),
        position: const Duration(seconds: 10),
        duration: const Duration(minutes: 30),
        active: true,
        buffering: true,
      )) {
        prompts++;
      }
    }
    expect(prompts, 1);
  });
  test(
    'deliberate pauses, startup, near end and seek never suggest switch',
    () {
      final monitor = PlaybackStallMonitor()..reset(Duration.zero);
      for (var i = 0; i < 60; i++) {
        expect(
          monitor.observe(
            now: Duration(seconds: i),
            position: Duration(seconds: i * 30),
            duration: const Duration(hours: 2),
            active: true,
            buffering: true,
          ),
          false,
        );
      }
      for (var i = 60; i < 120; i++) {
        expect(
          monitor.observe(
            now: Duration(seconds: i),
            position: const Duration(seconds: 1799),
            duration: const Duration(minutes: 30),
            active: true,
            buffering: true,
          ),
          false,
        );
      }
      for (var i = 120; i < 180; i++) {
        expect(
          monitor.observe(
            now: Duration(seconds: i),
            position: const Duration(seconds: 10),
            duration: const Duration(minutes: 30),
            active: false,
            buffering: true,
          ),
          false,
        );
      }
    },
  );
  test(
    'three short rebuffer events in45seconds prompt but smooth2x does not',
    () {
      final monitor = PlaybackStallMonitor()..reset(Duration.zero);
      var prompts = 0;
      var position = 0;
      for (var i = 0; i < 40; i++) {
        final buffering = [10, 11, 18, 19, 26, 27].contains(i);
        if (!buffering) position += 2;
        if (monitor.observe(
          now: Duration(seconds: i),
          position: Duration(seconds: position),
          duration: const Duration(hours: 1),
          active: true,
          buffering: buffering,
        )) {
          prompts++;
        }
      }
      expect(prompts, 1);
      final smooth = PlaybackStallMonitor()..reset(Duration.zero);
      for (var i = 0; i < 90; i++) {
        expect(
          smooth.observe(
            now: Duration(seconds: i),
            position: Duration(seconds: 2 * i),
            duration: const Duration(hours: 1),
            active: true,
            buffering: false,
          ),
          false,
        );
      }
    },
  );
}
