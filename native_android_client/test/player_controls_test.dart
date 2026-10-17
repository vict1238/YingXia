// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/player_controls.dart';
import 'package:yingxia_native/video_quality.dart';

class FakeControlsAdapter extends PlayerControlsAdapter {
  @override
  bool playing = true;
  @override
  bool buffering = false;
  @override
  Duration position = const Duration(seconds: 30);
  @override
  Duration duration = const Duration(minutes: 2);
  @override
  Duration buffer = const Duration(seconds: 55);
  @override
  double volume = 75;
  @override
  double rate = 1.5;
  @override
  BoxFit fit = BoxFit.contain;
  @override
  bool fullscreen = false;
  final rates = <double>[];
  final seeks = <Duration>[];
  int playCalls = 0;

  @override
  Future<void> playOrPause() async {
    playCalls++;
    playing = !playing;
    notifyListeners();
  }

  @override
  Future<void> seek(Duration position) async {
    this.position = position;
    seeks.add(position);
    notifyListeners();
  }

  @override
  Future<void> setVolume(double volume) async {
    this.volume = volume;
    notifyListeners();
  }

  @override
  Future<void> setRate(double rate) async {
    this.rate = rate;
    rates.add(rate);
    notifyListeners();
  }

  @override
  Future<void> toggleFullscreen() async {
    fullscreen = !fullscreen;
    notifyListeners();
  }

  @override
  void setFit(BoxFit fit) {
    this.fit = fit;
    notifyListeners();
  }
}

Future<void> mountControls(
  WidgetTester tester,
  FakeControlsAdapter adapter, {
  Size size = const Size(800, 500),
  GlobalKey<NavigatorState>? navigatorKey,
  Future<void> Function(bool)? onTemporaryRateChanged,
  Future<void> Function(double)? onRateChanged,
  Future<void> Function()? onPlayOrPause,
  ValueChanged<VideoQuality>? onQualityChanged,
  VoidCallback? onCacheSettings,
  Duration hideAfter = const Duration(seconds: 3),
}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    MaterialApp(
      navigatorKey: navigatorKey,
      theme: ThemeData.dark(),
      home: Scaffold(
        body: SizedBox.expand(
          child: YingXiaPlayerControls.withAdapter(
            adapter: adapter,
            requestedQuality: VideoQuality.enhanced,
            effectiveQuality: VideoQuality.light,
            onQualityChanged: onQualityChanged ?? (_) {},
            onRateChanged: onRateChanged,
            onPlayOrPause: onPlayOrPause,
            onTemporaryRateChanged: onTemporaryRateChanged,
            onCacheSettings: onCacheSettings,
            cacheDescription: '内存缓存 · 25 秒',
            enableLongPress: true,
            autofocus: true,
            hideAfter: hideAfter,
          ),
        ),
      ),
    ),
  );
  await tester.pump();
  addTearDown(() async {
    await tester.pumpWidget(const SizedBox.shrink());
    adapter.dispose();
  });
}

Finder control(String name) => find.byKey(ValueKey('player-controls-$name'));

Future<TestGesture> holdSurface(WidgetTester tester) async {
  final surface = tester.getRect(control('surface'));
  final gesture = await tester.startGesture(
    Offset(surface.center.dx, surface.top + 50),
  );
  await tester.pump(const Duration(milliseconds: 650));
  await tester.pump();
  return gesture;
}

void main() {
  test(
    'temporary exact2 rate restores the prior choice after a pending write',
    () async {
      var rate = 1.25;
      final apply = Completer<void>();
      final writes = <double>[];
      final temporary = TemporaryPlaybackRate(
        readRate: () => rate,
        writeRate: (value) async {
          writes.add(value);
          if (value == 2) await apply.future;
          rate = value;
        },
      );
      final start = temporary.begin();
      await Future<void>.delayed(Duration.zero);
      final end = temporary.end();
      expect(temporary.active, isFalse);
      expect(writes, [2]);
      apply.complete();
      await Future.wait([start, end]);
      expect(writes, [2, 1.25]);
      expect(rate, 1.25);
      await temporary.end();
      expect(writes, [2, 1.25]);
    },
  );

  test('temporary callback pairs still restore after a failed start', () async {
    final calls = <bool>[];
    final temporary = TemporaryPlaybackRate(
      readRate: () => 1.5,
      writeRate: (_) async {},
      onChanged: (active) async {
        calls.add(active);
        if (active) throw StateError('decoder is closing');
      },
    );
    await expectLater(temporary.begin(), throwsStateError);
    await temporary.end();
    expect(calls, [true, false]);
  });

  testWidgets('hold uses exactly2 and release restores previous1.5', (
    tester,
  ) async {
    final adapter = FakeControlsAdapter();
    await mountControls(tester, adapter);
    final gesture = await holdSurface(tester);
    expect(adapter.rate, 2);
    expect(find.text('2× 快进中'), findsOneWidget);
    await gesture.up();
    await tester.pump();
    expect(adapter.rate, 1.5);
    expect(adapter.rates, [2, 1.5]);
    expect(find.text('2× 快进中'), findsNothing);
  });

  testWidgets('pointer cancellation restores a3x user rate', (tester) async {
    final adapter = FakeControlsAdapter()..rate = 3;
    await mountControls(tester, adapter);
    final gesture = await holdSurface(tester);
    expect(adapter.rate, 2);
    await gesture.cancel();
    await tester.pump();
    expect(adapter.rate, 3);
  });

  testWidgets(
    'background and navigation release the authoritative hold callback',
    (tester) async {
      final adapter = FakeControlsAdapter();
      final calls = <bool>[];
      final navigator = GlobalKey<NavigatorState>();
      await mountControls(
        tester,
        adapter,
        navigatorKey: navigator,
        onTemporaryRateChanged: (active) async {
          calls.add(active);
          await adapter.setRate(active ? 2 : 1.5);
        },
      );
      var gesture = await holdSurface(tester);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
      await tester.pump();
      expect(calls, [true, false]);
      expect(adapter.rate, 1.5);
      await gesture.cancel();
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pump();
      gesture = await holdSurface(tester);
      unawaited(
        navigator.currentState!.push(
          MaterialPageRoute<void>(
            builder: (_) => const Scaffold(body: Text('另一页面')),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(calls, [true, false, true, false]);
      expect(adapter.rate, 1.5);
      await gesture.cancel();
    },
  );

  testWidgets('removing controls during a hold restores the rate', (
    tester,
  ) async {
    final adapter = FakeControlsAdapter();
    await mountControls(tester, adapter);
    final gesture = await holdSurface(tester);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
    expect(adapter.rate, 1.5);
    await gesture.cancel();
  });

  testWidgets('panel changes volume rate FSR fit and opens cache settings', (
    tester,
  ) async {
    final adapter = FakeControlsAdapter();
    VideoQuality? quality;
    var cacheOpened = false;
    await mountControls(
      tester,
      adapter,
      onQualityChanged: (value) => quality = value,
      onRateChanged: adapter.setRate,
      onCacheSettings: () => cacheOpened = true,
    );
    await tester.tap(control('settings'));
    await tester.pumpAndSettle();
    expect(find.text('当前生效：轻量'), findsOneWidget);
    expect(find.text('内存缓存 · 25 秒'), findsOneWidget);
    await tester.tap(control('mute'));
    await tester.pump();
    expect(adapter.volume, 0);
    await tester.tap(control('mute'));
    await tester.pump();
    expect(adapter.volume, 75);
    await tester.tap(control('rate'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('1.25×').last);
    await tester.pumpAndSettle();
    expect(adapter.rate, 1.25);
    await tester.tap(control('quality'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('关闭').last);
    await tester.pumpAndSettle();
    expect(quality, VideoQuality.off);
    await tester.tap(control('fit'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('填满裁切').last);
    await tester.pumpAndSettle();
    expect(adapter.fit, BoxFit.cover);
    await tester.ensureVisible(control('cache-settings'));
    await tester.tap(control('cache-settings'));
    await tester.pump();
    expect(cacheOpened, isTrue);
  });

  testWidgets('holding a volume slider never starts temporary fast forward', (
    tester,
  ) async {
    final adapter = FakeControlsAdapter();
    await mountControls(tester, adapter);
    await tester.tap(control('settings'));
    await tester.pumpAndSettle();
    final gesture = await tester.startGesture(
      tester.getCenter(control('volume')),
    );
    await tester.pump(const Duration(milliseconds: 650));
    expect(adapter.rates, isEmpty);
    await gesture.up();
  });

  testWidgets(
    'remote seeks clamps toggles playback and navigates to settings',
    (tester) async {
      final adapter = FakeControlsAdapter();
      await mountControls(tester, adapter);
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
      await tester.pump();
      expect(adapter.position, const Duration(seconds: 40));
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowLeft);
      await tester.pump();
      expect(adapter.position, const Duration(seconds: 30));
      await tester.sendKeyEvent(LogicalKeyboardKey.select);
      await tester.pump();
      expect(adapter.playing, isFalse);
      await tester.sendKeyEvent(LogicalKeyboardKey.audioVolumeUp);
      await tester.pump();
      expect(adapter.volume, 80);
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      await tester.pump();
      expect(
        tester.widget<IconButton>(control('settings')).focusNode!.hasFocus,
        isTrue,
      );
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pumpAndSettle();
      expect(control('panel'), findsOneWidget);
      final before = adapter.seeks.length;
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
      await tester.pump();
      expect(adapter.seeks.length, before);
    },
  );

  testWidgets('focused volume slider handles arrows without seeking', (
    tester,
  ) async {
    final adapter = FakeControlsAdapter();
    await mountControls(tester, adapter);
    await tester.tap(control('settings'));
    await tester.pumpAndSettle();
    tester.widget<Slider>(control('volume')).focusNode!.requestFocus();
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.pump();
    expect(adapter.seeks, isEmpty);
  });

  testWidgets('fullscreen button works inside the video bar', (tester) async {
    final adapter = FakeControlsAdapter();
    await mountControls(tester, adapter);
    await tester.tap(control('fullscreen'));
    await tester.pump();
    expect(adapter.fullscreen, isTrue);
  });

  testWidgets(
    'fullscreen explicit play clears app suspension through callback once',
    (tester) async {
      final adapter =
          FakeControlsAdapter()
            ..fullscreen = true
            ..playing = false;
      var suspended = true;
      var callbacks = 0;
      await mountControls(
        tester,
        adapter,
        onPlayOrPause: () async {
          callbacks++;
          suspended = false;
          await adapter.playOrPause();
        },
      );
      await tester.tap(control('play'));
      await tester.pump();
      expect(suspended, isFalse);
      expect(adapter.playing, isTrue);
      expect(callbacks, 1);
      expect(adapter.playCalls, 1);
    },
  );

  testWidgets(
    'TV remote select and media key each use the app play callback once',
    (tester) async {
      final adapter = FakeControlsAdapter()..playing = false;
      var callbacks = 0;
      await mountControls(
        tester,
        adapter,
        onPlayOrPause: () async {
          callbacks++;
          await adapter.playOrPause();
        },
      );
      await tester.sendKeyEvent(LogicalKeyboardKey.select);
      await tester.pump();
      expect(adapter.playing, isTrue);
      expect(callbacks, 1);
      await tester.sendKeyEvent(LogicalKeyboardKey.mediaPlayPause);
      await tester.pump();
      expect(adapter.playing, isFalse);
      expect(callbacks, 2);
      expect(adapter.playCalls, 2);
    },
  );

  testWidgets('timeline updates do not prevent controls auto hiding', (
    tester,
  ) async {
    final adapter = FakeControlsAdapter();
    await mountControls(
      tester,
      adapter,
      hideAfter: const Duration(milliseconds: 500),
    );
    for (var i = 0; i < 6; i++) {
      adapter.position += const Duration(milliseconds: 100);
      adapter.notifyListeners();
      await tester.pump(const Duration(milliseconds: 100));
    }
    final opacity = tester.widget<AnimatedOpacity>(
      find
          .ancestor(of: control('bar'), matching: find.byType(AnimatedOpacity))
          .first,
    );
    expect(opacity.opacity, 0);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.pump();
    expect(
      tester
          .widget<AnimatedOpacity>(
            find
                .ancestor(
                  of: control('bar'),
                  matching: find.byType(AnimatedOpacity),
                )
                .first,
          )
          .opacity,
      1,
    );
  });

  for (final size in const [Size(320, 180), Size(3840, 2160)]) {
    testWidgets(
      'compact controls and scrollable settings fit ${size.width}x${size.height}',
      (tester) async {
        final adapter = FakeControlsAdapter();
        await mountControls(tester, adapter, size: size);
        await tester.tap(control('settings'));
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull);
        expect(tester.getRect(control('panel')).left, greaterThanOrEqualTo(0));
        expect(
          tester.getRect(control('panel')).right,
          lessThanOrEqualTo(size.width),
        );
        expect(tester.getRect(control('panel')).top, greaterThanOrEqualTo(0));
        expect(
          tester.getRect(control('bar')).bottom,
          lessThanOrEqualTo(size.height),
        );
        if (size.width > 1200) {
          expect(
            tester.widget<IconButton>(control('settings')).iconSize,
            greaterThan(30),
          );
        }
      },
    );
  }
}
