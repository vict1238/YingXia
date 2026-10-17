// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/backend.dart';
import 'package:yingxia_native/source_manager.dart';

class SourceBackend extends BackendConnection {
  final List<(String, SourceJson)> calls = [];
  SourceJson registry = {
    'auto_update': true,
    'providers': List.generate(
      32,
      (i) => {
        'id': 'source-$i',
        'name': '来源 $i',
        'kind': 'cms',
        'enabled': i != 1,
        'custom': i == 0,
        'editable': i == 0,
        'url': 'https://example.com/api?token=***',
        'health': {
          'status': i == 2 ? 'unavailable' : 'unknown',
          'latency_ms': 0,
        },
      },
    ),
    'upstreams': {},
    'changes': [],
  };
  int polls = 0;
  bool finish = false;

  SourceJson job(String state, int completed) => {
    'id': 'speed-1',
    'state': state,
    'completed': completed,
    'total': 32,
    'sources': registry,
  };

  @override
  Future<dynamic> post(String path, Map<String, dynamic> data) async {
    calls.add((path, data));
    switch (path) {
      case 'sources/speed/start':
        return job('running', 0);
      case 'sources/speed/cancel':
        return job('cancelled', polls);
      case 'sources/edit':
        final rows = List<SourceJson>.from(registry['providers'] as List);
        rows[0] = {...rows[0], 'name': data['name']};
        registry = {...registry, 'providers': rows};
        return registry;
      case 'sources/add':
      case 'sources/remove':
      case 'sources/update':
        return registry;
    }
    throw StateError(path);
  }

  @override
  Future<dynamic> get(String path) async {
    expect(path, 'sources/speed/status?id=speed-1');
    polls++;
    final rows = List<SourceJson>.from(registry['providers'] as List);
    rows[0] = {
      ...rows[0],
      'health': {'status': 'available', 'latency_ms': 123},
    };
    registry = {...registry, 'providers': rows};
    return job(finish ? 'done' : 'running', finish ? 32 : polls);
  }
}

Future<void> openManager(
  WidgetTester tester,
  SourceBackend backend, {
  List<(String, bool)>? toggles,
}) async {
  tester.view.physicalSize = const Size(1050, 2100);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  var registry = backend.registry;
  await tester.pumpWidget(
    MaterialApp(
      theme: ThemeData.dark(),
      home: Scaffold(
        body: SingleChildScrollView(
          child: StatefulBuilder(
            builder:
                (context, setState) => SourceManager(
                  backend: backend,
                  registry: registry,
                  ensureNetwork: () async => true,
                  refresh:
                      () async => setState(() => registry = backend.registry),
                  onRegistry: (value) => setState(() => registry = value),
                  toggleProvider: (source, enabled) async {
                    toggles?.add((source['id'] as String, enabled));
                  },
                  saveAutoUpdate: (enabled) async {
                    setState(
                      () => registry = {...registry, 'auto_update': enabled},
                    );
                  },
                ),
          ),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('large source list is bounded, searchable and filterable', (
    tester,
  ) async {
    final backend = SourceBackend();
    await openManager(tester, backend);
    expect(find.byKey(const Key('source-row-source-14')), findsOneWidget);
    expect(find.byKey(const Key('source-row-source-15')), findsNothing);
    await tester.ensureVisible(find.byKey(const Key('source-page-next')));
    await tester.tap(find.byKey(const Key('source-page-next')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('source-row-source-15')), findsOneWidget);
    expect(find.byKey(const Key('source-row-source-0')), findsNothing);
    await tester.ensureVisible(find.byKey(const Key('source-filter-query')));
    await tester.enterText(
      find.byKey(const Key('source-filter-query')),
      '来源 31',
    );
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('source-row-source-31')), findsOneWidget);
    expect(find.byKey(const Key('source-row-source-30')), findsNothing);
    await tester.enterText(find.byKey(const Key('source-filter-query')), '');
    await tester.tap(find.byKey(const Key('source-filter')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('未响应').last);
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('source-row-source-2')), findsOneWidget);
    expect(find.byKey(const Key('source-row-source-0')), findsNothing);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets(
    'all-source and single-source latency jobs show progress and can cancel',
    (tester) async {
      final backend = SourceBackend();
      await openManager(tester, backend);
      await tester.tap(find.byKey(const Key('source-measure-all')));
      await tester.pump();
      expect(backend.calls.last.$1, 'sources/speed/start');
      expect(backend.calls.last.$2, <String, dynamic>{});
      expect(find.text('测速中 · 0 / 32'), findsOneWidget);
      await tester.pump(const Duration(milliseconds: 510));
      await tester.pump();
      expect(find.text('测速中 · 1 / 32'), findsOneWidget);
      expect(find.text('CMS · 已响应 · 123 ms'), findsOneWidget);
      await tester.tap(find.byKey(const Key('source-speed-cancel')));
      await tester.pumpAndSettle();
      expect(find.text('已取消 · 1 / 32'), findsOneWidget);
      expect(backend.calls.last.$1, 'sources/speed/cancel');
      expect(backend.calls.last.$2, {'id': 'speed-1'});
      backend.finish = true;
      await tester.tap(find.byKey(const Key('source-measure-source-1')));
      await tester.pump();
      expect(backend.calls.last.$1, 'sources/speed/start');
      expect(backend.calls.last.$2, {
        'ids': ['source-1'],
      });
      await tester.pump(const Duration(milliseconds: 510));
      await tester.pumpAndSettle();
      expect(find.text('测速完成 · 32 / 32'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    'custom source edit preserves redacted URL and switches remain manageable',
    (tester) async {
      final backend = SourceBackend();
      final toggles = <(String, bool)>[];
      await openManager(tester, backend, toggles: toggles);
      await tester.tap(find.byKey(const Key('source-enabled-source-1')));
      await tester.pumpAndSettle();
      expect(toggles, [('source-1', true)]);
      await tester.tap(find.byKey(const Key('source-auto-update')));
      await tester.pumpAndSettle();
      expect(
        tester
            .widget<Switch>(find.byKey(const Key('source-auto-update')))
            .value,
        false,
      );
      await tester.tap(find.byKey(const Key('source-actions-source-0')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('编辑'));
      await tester.pumpAndSettle();
      expect(
        tester
            .widget<TextField>(find.byKey(const Key('source-dialog-url')))
            .controller!
            .text,
        '',
      );
      await tester.enterText(
        find.byKey(const Key('source-dialog-name')),
        '新名称',
      );
      await tester.tap(find.byKey(const Key('source-dialog-submit')));
      await tester.pumpAndSettle();
      expect(backend.calls.last.$1, 'sources/edit');
      expect(backend.calls.last.$2, {
        'id': 'source-0',
        'name': '新名称',
        'url': '',
      });
      expect(find.text('新名称'), findsOneWidget);
      await tester.tap(find.byKey(const Key('source-update')));
      await tester.pumpAndSettle();
      expect(backend.calls.last.$1, 'sources/update');
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
}
