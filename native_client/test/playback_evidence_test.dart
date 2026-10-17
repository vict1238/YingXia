// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/playback_evidence.dart';

void main() {
  test('16 ms native callbacks accumulate real playback evidence', () {
    final clock = PlaybackEvidenceClock();
    final advanced = <int>[];
    final saved = <int>[];
    for (var milliseconds = 16; milliseconds <= 4000; milliseconds += 16) {
      final position = Duration(milliseconds: milliseconds);
      if (clock.advanced(position)) {
        advanced.add(milliseconds);
        if (clock.shouldSave(position)) saved.add(milliseconds);
      }
    }
    expect(advanced.first, 112);
    expect(advanced.length, greaterThan(30));
    expect(saved, [112, 3136]);
  });
  test('stationary position never counts as playback', () {
    final clock = PlaybackEvidenceClock();
    for (var count = 0; count < 100; count++) {
      expect(clock.advanced(Duration.zero), isFalse);
    }
  });
  test('forward seek itself is not playback and resumes saving afterward', () {
    final clock = PlaybackEvidenceClock();
    expect(clock.advanced(const Duration(milliseconds: 120)), isTrue);
    expect(clock.shouldSave(const Duration(milliseconds: 120)), isTrue);
    expect(clock.advanced(const Duration(seconds: 30)), isFalse);
    expect(clock.advanced(const Duration(milliseconds: 30016)), isFalse);
    expect(clock.advanced(const Duration(milliseconds: 30112)), isTrue);
    expect(clock.shouldSave(const Duration(milliseconds: 30112)), isTrue);
  });
  test(
    'backward seek resets the save interval and new media resets anchor',
    () {
      final clock = PlaybackEvidenceClock();
      clock.advanced(const Duration(seconds: 30));
      expect(clock.advanced(const Duration(milliseconds: 30112)), isTrue);
      expect(clock.shouldSave(const Duration(milliseconds: 30112)), isTrue);
      expect(clock.advanced(const Duration(seconds: 2)), isFalse);
      expect(clock.advanced(const Duration(milliseconds: 2112)), isTrue);
      expect(clock.shouldSave(const Duration(milliseconds: 2112)), isTrue);
      clock.reset();
      expect(clock.advanced(const Duration(milliseconds: 112)), isTrue);
      expect(clock.shouldSave(const Duration(milliseconds: 112)), isTrue);
    },
  );
}
