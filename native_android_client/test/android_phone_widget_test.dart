// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/video_quality.dart';
import 'package:yingxia_native/yingxia_app.dart';

import 'yingxia_widget_test.dart' as fixtures;

class PhoneBackend extends fixtures.FakeBackend {
  bool saved = false;
  int checks = 0;

  PhoneBackend() {
    config = {
      ...config,
      'source_registry': {
        ...asJson(config['source_registry']),
        'providers': List.generate(
          47,
          (i) => {
            'id': 'phone-$i',
            'name': '手机来源 $i 用于检验很长的来源名称',
            'kind': 'cms',
            'enabled': true,
            'origin': 'builtin',
          },
        ),
      },
    };
  }

  @override
  Future<dynamic> get(String path) async {
    if (path == 'library') {
      calls.add(path);
      return {
        'favorites': saved ? [detail] : [],
        'history': cleared ? [] : [history],
      };
    }
    if (path.startsWith('search/status?') && checks++ == 0) {
      calls.add(path);
      return {
        'id': 'job-1',
        'phase': 'checking',
        'percent': 65,
        'checked_count': 1,
        'check_total': 3,
        'available_count': 1,
        'items': [],
      };
    }
    return super.get(path);
  }

  @override
  Future<dynamic> post(String path, Map<String, dynamic> data) async {
    if (path == 'favorite') {
      calls.add(path);
      saved = !saved;
      config = {
        ...config,
        'favorite_ids': saved ? [detail['id']] : [],
      };
      return {'saved': saved};
    }
    return super.post(path, data);
  }
}

Future<void> phoneSize(WidgetTester tester, Size size) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}

Future<void> navigate(WidgetTester tester, String label) async {
  await tester.tap(find.text(label).last);
  await tester.pumpAndSettle();
}

Future<void> chooseQuality(
  WidgetTester tester,
  String key,
  String label,
) async {
  await tester.ensureVisible(find.byKey(Key(key)));
  await tester.tap(find.byKey(Key(key)));
  await tester.pumpAndSettle();
  await tester.tap(find.text(label).last);
  await tester.pumpAndSettle();
}

VideoQuality? selectedQuality(WidgetTester tester, String key) =>
    tester.widget<DropdownButton<VideoQuality>>(find.byKey(Key(key))).value;

void main() {
  testWidgets(
    '320px default stays independent from temporary quality after restart',
    (tester) async {
      await phoneSize(tester, const Size(320, 640));
      final preferences = fixtures.MemoryQualityPreferences();
      Future<void> open() async {
        await tester.pumpWidget(
          YingXiaApp(backend: PhoneBackend(), qualityPreferences: preferences),
        );
        await tester.pumpAndSettle();
      }

      await open();
      await navigate(tester, '播放');
      expect(selectedQuality(tester, 'video-quality'), VideoQuality.off);
      expect(
        find.descendant(
          of: find.byType(AspectRatio).first,
          matching: find.byIcon(Icons.play_arrow_outlined),
        ),
        findsNothing,
      );
      await chooseQuality(tester, 'video-quality', '增强');
      expect(preferences.mode, VideoQuality.off);
      await navigate(tester, '来源');
      expect(
        selectedQuality(tester, 'video-quality-default'),
        VideoQuality.off,
      );
      await chooseQuality(tester, 'video-quality-default', '轻量');
      expect(preferences.mode, VideoQuality.light);
      await navigate(tester, '播放');
      expect(selectedQuality(tester, 'video-quality'), VideoQuality.enhanced);
      await tester.pumpWidget(const SizedBox());
      await open();
      await navigate(tester, '播放');
      expect(selectedQuality(tester, 'video-quality'), VideoQuality.light);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    '360px availability progress selects the checked episode across rotation',
    (tester) async {
      await phoneSize(tester, const Size(360, 800));
      final backend = PhoneBackend();
      await tester.pumpWidget(YingXiaApp(backend: backend));
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const Key('search-query')), '测试');
      await tester.tap(find.byKey(const Key('search-submit')));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 500));
      await tester.pump();
      expect(find.text('检测访问 · 1 / 3 个结果 · 1 个可访问'), findsOneWidget);
      expect(find.byType(LinearProgressIndicator), findsOneWidget);
      await tester.pump(const Duration(milliseconds: 500));
      await tester.pumpAndSettle();
      expect(find.text('检测完成 · 1 个可访问'), findsOneWidget);
      await tester.ensureVisible(find.text('测试电影'));
      await tester.tap(find.text('测试电影'));
      await tester.pumpAndSettle();
      expect(find.text('已选：第02集'), findsOneWidget);
      tester.view.physicalSize = const Size(640, 360);
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('第01集'));
      await tester.tap(find.text('第01集'));
      await tester.pumpAndSettle();
      expect(find.text('已选：第01集'), findsOneWidget);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await tester.pump();
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pump();
      expect(backend.calls.where((path) => path == 'resolve'), isEmpty);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    'phone clear confirmation persists after restart and keeps favorites',
    (tester) async {
      await phoneSize(tester, const Size(360, 800));
      final backend = PhoneBackend()..saved = true;
      Future<void> open() async {
        await tester.pumpWidget(YingXiaApp(backend: backend));
        await tester.pumpAndSettle();
        await navigate(tester, '收藏');
      }

      await open();
      expect(find.text('1 个收藏'), findsOneWidget);
      await tester.ensureVisible(find.byKey(const Key('clear-history')));
      await tester.tap(find.byKey(const Key('clear-history')));
      await tester.pumpAndSettle();
      expect(backend.clearCalls, 0);
      await tester.tap(find.text('取消'));
      await tester.pumpAndSettle();
      expect(backend.clearCalls, 0);
      await tester.tap(find.byKey(const Key('clear-history')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('confirm-clear-history')));
      await tester.pumpAndSettle();
      expect(backend.clearCalls, 1);
      expect(find.byKey(const Key('empty-history')), findsOneWidget);
      expect(find.text('1 个收藏'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      await open();
      expect(find.byKey(const Key('empty-history')), findsOneWidget);
      expect(find.text('1 个收藏'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
}
