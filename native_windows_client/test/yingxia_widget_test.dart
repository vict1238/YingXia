// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/backend.dart';
import 'package:yingxia_native/yingxia_app.dart';
import 'package:yingxia_native/video_quality.dart';

class MemoryQualityPreferences extends VideoQualityPreferences {
  MemoryQualityPreferences() : super('');
  VideoQuality mode = VideoQuality.off;
  @override
  Future<VideoQuality> load() async => mode;
  @override
  Future<void> save(VideoQuality value) async => mode = value;
}

class FakeBackend extends BackendConnection {
  bool cleared = false;
  bool failClear = false;
  int clearCalls = 0;
  Completer<Json>? nextLibrary;
  Completer<Json>? nextResolve;
  final List<String> calls = [];
  Json config = {
    'version': '2.0.0-native',
    'network_consent': true,
    'save_history': true,
    'auto_next': true,
    'data_dir': r'C:\映匣\数据',
    'favorite_ids': [],
    'sources': [],
    'platforms': {
      'qq': {'name': '腾讯视频'},
    },
    'source_registry': {
      'auto_update': true,
      'providers': [
        {
          'id': 'test',
          'name': '测试来源',
          'kind': 'cms',
          'enabled': true,
          'origin': 'builtin',
        },
      ],
      'upstreams': {},
      'changes': [],
    },
  };
  final Json history = {
    'id': 'history-1',
    'film_id': 'film-1',
    'title': '测试电影',
    'episode': '第01集',
    'source': '测试来源',
    'mode': 'media',
    'evidence': 'browser_time_advanced',
    'position': 22,
    'updated_at': '2026-10-14T12:00:00Z',
  };
  final Json detail = {
    'id': 'film-1',
    'title': '测试电影',
    'year': '2026',
    'kind': '电影',
    'source': '测试来源',
    'summary': '测试作品信息',
    'groups': [
      {
        'key': 'g1',
        'name': '主线路',
        'episodes': [
          {'key': 'e1', 'name': '第01集'},
          {'key': 'e2', 'name': '第02集'},
        ],
      },
    ],
  };
  @override
  Future<void> start() async {}
  @override
  Future<void> dispose() async {}
  @override
  String get origin => 'http://127.0.0.1:12345';
  @override
  Map<String, String> get mediaHeaders => {};
  @override
  Future<dynamic> get(String path) async {
    calls.add(path);
    if (path == 'state') return config;
    if (path == 'library') {
      final pending = nextLibrary;
      nextLibrary = null;
      if (pending != null) return pending.future;
      return {
        'favorites': [],
        'history': cleared ? [] : [history],
      };
    }
    if (path.startsWith('detail?')) return detail;
    if (path.startsWith('bindings?')) return [];
    if (path.startsWith('search/status?')) {
      return {
        'id': 'job-1',
        'phase': 'done',
        'percent': 100,
        'available_count': 1,
        'successful_sources': 1,
        'source_count': 1,
        'items': [
          {
            ...detail,
            'availability': {
              'state': 'media_ok',
              'label': '媒体可访问',
              'group_key': 'g1',
              'episode_key': 'e2',
            },
          },
        ],
      };
    }
    throw StateError('Unexpected GET $path');
  }

  @override
  Future<dynamic> post(String path, Map<String, dynamic> data) async {
    calls.add(path);
    if (path == 'clear-history') {
      clearCalls++;
      if (failClear) throw Exception('写入失败');
      cleared = true;
      return {};
    }
    if (path == 'search/start') {
      return {'id': 'job-1', 'phase': 'searching', 'percent': 0, 'items': []};
    }
    if (path == 'search/cancel' || path == 'ping') return {};
    if (path == 'settings') {
      config = {...config, ...data};
      return config;
    }
    if (path == 'favorite') return {'saved': true};
    if (path == 'resolve') {
      final pending = nextResolve;
      nextResolve = null;
      if (pending != null) return pending.future;
      return {'kind': 'page', 'url': 'https://example.com/page'};
    }
    throw StateError('Unexpected POST $path');
  }
}

Future<void> openLibrary(WidgetTester tester, FakeBackend backend) async {
  tester.view.physicalSize = const Size(1340, 900);
  tester.view.devicePixelRatio = 1;
  await tester.pumpWidget(YingXiaApp(backend: backend));
  await tester.pumpAndSettle();
  await tester.tap(find.byKey(const Key('nav-2')));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('clear history confirms in app, persists then refreshes rows', (
    tester,
  ) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final backend = FakeBackend();
    await openLibrary(tester, backend);
    expect(find.text('测试电影'), findsOneWidget);
    await tester.tap(find.byKey(const Key('clear-history')));
    await tester.pumpAndSettle();
    expect(find.text('清空历史记录？'), findsOneWidget);
    expect(backend.clearCalls, 0);
    await tester.tap(find.byKey(const Key('confirm-clear-history')));
    await tester.pumpAndSettle();
    expect(backend.clearCalls, 1);
    expect(find.text('测试电影'), findsNothing);
    expect(find.byKey(const Key('empty-history')), findsOneWidget);
    expect(backend.calls.where((p) => p == 'library').length, 2);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('cancel clear keeps rows and never sends clear API', (
    tester,
  ) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final backend = FakeBackend();
    await openLibrary(tester, backend);
    await tester.tap(find.byKey(const Key('clear-history')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('取消'));
    await tester.pumpAndSettle();
    expect(find.text('测试电影'), findsOneWidget);
    expect(backend.clearCalls, 0);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('late library response cannot restore cleared history', (
    tester,
  ) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final backend = FakeBackend();
    await openLibrary(tester, backend);
    final older = Completer<Json>();
    backend.nextLibrary = older;
    await tester.tap(find.byKey(const Key('nav-2')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('clear-history')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirm-clear-history')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('empty-history')), findsOneWidget);
    older.complete({
      'favorites': [],
      'history': [backend.history],
    });
    await tester.pumpAndSettle();
    expect(find.text('测试电影'), findsNothing);
    expect(find.byKey(const Key('empty-history')), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('failed clear preserves history and reports failure', (
    tester,
  ) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final backend = FakeBackend()..failClear = true;
    await openLibrary(tester, backend);
    await tester.tap(find.byKey(const Key('clear-history')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirm-clear-history')));
    await tester.pumpAndSettle();
    expect(backend.clearCalls, 1);
    expect(find.text('测试电影'), findsOneWidget);
    expect(find.textContaining('写入失败'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('search progress polls service and chooses tested episode', (
    tester,
  ) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    tester.view.physicalSize = const Size(1340, 900);
    tester.view.devicePixelRatio = 1;
    final backend = FakeBackend();
    await tester.pumpWidget(YingXiaApp(backend: backend));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('search-query')), '测试');
    await tester.tap(find.byKey(const Key('search-submit')));
    await tester.pump();
    expect(find.text('正在搜索…'), findsOneWidget);
    await tester.pump(const Duration(milliseconds: 500));
    await tester.pumpAndSettle();
    expect(find.text('检测完成 · 1 个可访问'), findsOneWidget);
    await tester.tap(find.text('测试电影'));
    await tester.pumpAndSettle();
    expect(find.text('已选：第02集'), findsOneWidget);
    expect(find.text('选集之后请点击下方播放所选集'), findsOneWidget);
    await tester.tap(find.byKey(const Key('play-selected')));
    await tester.pumpAndSettle();
    expect(find.text('此来源提供网页播放，请切换来源'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('phone navigation and all four pages avoid layout overflow', (
    tester,
  ) async {
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    tester.view.devicePixelRatio = 1;
    for (final size in [const Size(390, 844), const Size(320, 640)]) {
      tester.view.physicalSize = size;
      final backend = FakeBackend();
      await tester.pumpWidget(YingXiaApp(backend: backend));
      await tester.pumpAndSettle();
      expect(find.byType(NavigationBar), findsOneWidget);
      for (final label in ['播放', '收藏', '来源', '搜索']) {
        await tester.tap(find.text(label).last);
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull, reason: '$label at $size');
      }
      await tester.pumpWidget(const SizedBox());
    }
  });
  testWidgets(
    'selecting another episode invalidates pending playback resolution',
    (tester) async {
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      tester.view.physicalSize = const Size(1340, 900);
      tester.view.devicePixelRatio = 1;
      final backend = FakeBackend();
      await tester.pumpWidget(YingXiaApp(backend: backend));
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const Key('search-query')), '测试');
      await tester.tap(find.byKey(const Key('search-submit')));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 500));
      await tester.pumpAndSettle();
      await tester.tap(find.text('测试电影'));
      await tester.pumpAndSettle();
      final pending = Completer<Json>();
      backend.nextResolve = pending;
      await tester.tap(find.byKey(const Key('play-selected')));
      await tester.pump();
      await tester.tap(find.text('第01集'));
      await tester.pumpAndSettle();
      expect(find.text('已选：第01集'), findsOneWidget);
      pending.complete({
        'kind': 'page',
        'url': 'https://example.com/old-episode',
      });
      await tester.pumpAndSettle();
      expect(find.text('此来源提供网页播放，请切换来源'), findsNothing);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
  testWidgets(
    'startup default is off and temporary playback choices do not overwrite it',
    (tester) async {
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      tester.view.physicalSize = const Size(1340, 900);
      tester.view.devicePixelRatio = 1;
      final preferences = MemoryQualityPreferences();
      Future<void> open() async {
        await tester.pumpWidget(
          YingXiaApp(backend: FakeBackend(), qualityPreferences: preferences),
        );
        await tester.pumpAndSettle();
      }

      VideoQuality? selected(String key) =>
          tester
              .widget<DropdownButton<VideoQuality>>(find.byKey(Key(key)))
              .value;
      Future<void> choose(String key, String label) async {
        await tester.ensureVisible(find.byKey(Key(key)));
        await tester.tap(find.byKey(Key(key)));
        await tester.pumpAndSettle();
        await tester.tap(find.text(label).last);
        await tester.pumpAndSettle();
      }

      await open();
      await tester.tap(find.byKey(const Key('nav-1')));
      await tester.pumpAndSettle();
      expect(selected('video-quality'), VideoQuality.off);
      expect(
        find.descendant(
          of: find.byType(AspectRatio).first,
          matching: find.byIcon(Icons.play_arrow_outlined),
        ),
        findsNothing,
      );
      await choose('video-quality', '增强');
      expect(selected('video-quality'), VideoQuality.enhanced);
      expect(preferences.mode, VideoQuality.off);
      await tester.tap(find.byKey(const Key('nav-3')));
      await tester.pumpAndSettle();
      expect(selected('video-quality-default'), VideoQuality.off);
      await choose('video-quality-default', '轻量');
      expect(preferences.mode, VideoQuality.light);
      await tester.tap(find.byKey(const Key('nav-1')));
      await tester.pumpAndSettle();
      expect(selected('video-quality'), VideoQuality.enhanced);
      await choose('video-quality', '关闭');
      expect(preferences.mode, VideoQuality.light);
      await tester.pumpWidget(const SizedBox());
      await open();
      await tester.tap(find.byKey(const Key('nav-1')));
      await tester.pumpAndSettle();
      expect(selected('video-quality'), VideoQuality.light);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
  testWidgets(
    'developers appear before source settings with exact names and addresses',
    (tester) async {
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      tester.view.physicalSize = const Size(1340, 900);
      tester.view.devicePixelRatio = 1;
      await tester.pumpWidget(YingXiaApp(backend: FakeBackend()));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('nav-3')));
      await tester.pumpAndSettle();
      expect(find.text('翻翻蓄小激光  cqh.vict1238@gmail.com'), findsOneWidget);
      expect(find.text('流风驻尘闲  yuchihaobo@gmail.com'), findsOneWidget);
      expect(
        tester.getTopLeft(find.text('开发人员')).dy,
        lessThan(tester.getTopLeft(find.text('设置')).dy),
      );
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
}
