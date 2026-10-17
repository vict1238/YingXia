// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/yingxia_app.dart';
import 'yingxia_widget_test.dart' show FakeBackend;

class FeatureBackend extends FakeBackend {
  bool tv = false;
  Json? resolution;
  Completer<Json>? pendingResume;
  @override
  bool get isAndroidTv => tv;
  @override
  Future<dynamic> get(String path) async {
    if (path == 'library') {
      return {
        'favorites': [detail],
        'history': cleared ? [] : [history],
      };
    }
    if (path.startsWith('resume?')) {
      calls.add(path);
      if (pendingResume != null) return pendingResume!.future;
      return {
        'found': true,
        'policy': 'resume',
        'group_key': 'g1',
        'episode_key': 'e2',
        'position': 125.5,
        'duration': 1200,
      };
    }
    if (path.startsWith('search/suggest?')) {
      return {
        'items': [
          {'title': '测试电影', 'query': '测试电影', 'match': 'initials'},
        ],
      };
    }
    return super.get(path);
  }

  @override
  Future<dynamic> post(String path, Json data) async {
    if (path == 'resolve') resolution = data;
    return super.post(path, data);
  }
}

void main() {
  Future<void> open(WidgetTester tester, FeatureBackend backend) async {
    tester.view.physicalSize = const Size(1340, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(YingXiaApp(backend: backend));
    await tester.pumpAndSettle();
  }

  testWidgets(
    'clear invalidates a pending history resume before it can autoplay',
    (tester) async {
      final backend = FeatureBackend()..pendingResume = Completer<Json>();
      await open(tester, backend);
      await tester.tap(find.byKey(const Key('nav-2')));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('继续观看'));
      await tester.tap(find.text('继续观看'));
      await tester.pump();
      await tester.tap(find.byKey(const Key('nav-2')));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.byKey(const Key('clear-history')));
      await tester.tap(find.byKey(const Key('clear-history')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('confirm-clear-history')));
      await tester.pumpAndSettle();
      backend.pendingResume!.complete({
        'found': true,
        'group_key': 'g1',
        'episode_key': 'e2',
        'position': 125,
      });
      await tester.pumpAndSettle();
      expect(backend.resolution, isNull);
      expect(find.byKey(const Key('empty-history')), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
    },
  );
  testWidgets(
    'favorite opens latest saved episode and requests playback directly',
    (tester) async {
      final backend = FeatureBackend();
      await open(tester, backend);
      await tester.tap(find.byKey(const Key('nav-2')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('测试电影').first);
      await tester.pumpAndSettle();
      expect(backend.calls.any((path) => path.startsWith('resume?')), true);
      expect(backend.resolution?['episode_key'], 'e2');
      expect(find.text('已选：第02集'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
  testWidgets(
    'history prefers most recent saved episode over stale clicked row',
    (tester) async {
      final backend = FeatureBackend();
      await open(tester, backend);
      await tester.tap(find.byKey(const Key('nav-2')));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('继续观看'));
      await tester.tap(find.text('继续观看'));
      await tester.pumpAndSettle();
      expect(backend.resolution?['episode_key'], 'e2');
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
  testWidgets('pinyin suggestion chip fills actual Chinese search title', (
    tester,
  ) async {
    final backend = FeatureBackend();
    await open(tester, backend);
    await tester.enterText(find.byKey(const Key('search-query')), 'csdy');
    await tester.pump(const Duration(milliseconds: 350));
    await tester.pumpAndSettle();
    expect(find.widgetWithText(ActionChip, '测试电影'), findsOneWidget);
    await tester.tap(find.widgetWithText(ActionChip, '测试电影'));
    await tester.pump(const Duration(milliseconds: 500));
    await tester.pumpAndSettle();
    expect(
      tester
          .widget<TextField>(find.byKey(const Key('search-query')))
          .controller!
          .text,
      '测试电影',
    );
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('TV full HD and4K screens retain readable scale and all pages', (
    tester,
  ) async {
    for (final size in [const Size(1920, 1080), const Size(3840, 2160)]) {
      final backend = FeatureBackend()..tv = true;
      await open(tester, backend);
      tester.view.physicalSize = size;
      await tester.pumpAndSettle();
      expect(
        MediaQuery.textScalerOf(
          tester.element(find.byKey(const Key('search-query'))),
        ).scale(16),
        20,
      );
      for (final i in [1, 2, 3, 0]) {
        await tester.tap(find.byKey(Key('nav-$i')));
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull, reason: 'TV page$i at$size');
      }
      await tester.pumpWidget(const SizedBox());
    }
  });
}
