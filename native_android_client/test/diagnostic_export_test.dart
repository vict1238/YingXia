// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/backend.dart';
import 'package:yingxia_native/yingxia_app.dart';

import 'yingxia_widget_test.dart' as fixtures;

const redactedReport = {
  'app': 'YingXia',
  'version': '2.2.0-native-android',
  'source_registry': {
    'providers': [
      {'id': 'custom', 'endpoint': 'https://example.com/api?token=***'},
    ],
  },
};

class ExportBackend extends fixtures.FakeBackend {
  Completer<String?>? pendingExport;
  String? destination;
  Object? transferredReport;
  int exportCalls = 0;

  @override
  Future<dynamic> get(String path) async {
    if (path == 'report') {
      calls.add(path);
      return redactedReport;
    }
    return super.get(path);
  }

  @override
  Future<String?> exportDiagnostics(
    Object? report, {
    required String directory,
  }) async {
    exportCalls++;
    transferredReport = report;
    return pendingExport?.future ?? Future.value(destination);
  }
}

Future<void> openExportPage(WidgetTester tester, ExportBackend backend) async {
  tester.view.physicalSize = const Size(360, 800);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(YingXiaApp(backend: backend));
  await tester.pumpAndSettle();
  await tester.tap(find.text('来源').last);
  await tester.pumpAndSettle();
  await tester.ensureVisible(find.byKey(const Key('export-diagnostics')));
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('yingxia/core');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  tearDown(() => messenger.setMockMethodCallHandler(channel, null));

  test(
    'Android document export sends only JSON and handles success or cancellation',
    () async {
      final calls = <MethodCall>[];
      bool? result = true;
      messenger.setMockMethodCallHandler(channel, (call) async {
        calls.add(call);
        return result;
      });
      final backend = BackendConnection();
      expect(await backend.saveDiagnosticDocument(redactedReport), isTrue);
      expect(calls.single.method, 'saveDiagnosticReport');
      final arguments = Map<String, dynamic>.from(
        calls.single.arguments as Map,
      );
      expect(arguments.keys, ['json']);
      expect(jsonDecode(arguments['json'] as String), redactedReport);
      expect(arguments.containsKey('path'), isFalse);
      expect(arguments.containsKey('filename'), isFalse);
      result = false;
      expect(await backend.saveDiagnosticDocument(redactedReport), isFalse);
      result = null;
      expect(await backend.saveDiagnosticDocument(redactedReport), isFalse);
    },
  );

  test(
    'oversized diagnostic payload is rejected before platform transfer',
    () async {
      var calls = 0;
      messenger.setMockMethodCallHandler(channel, (_) async {
        calls++;
        return true;
      });
      await expectLater(
        BackendConnection().saveDiagnosticDocument({
          'large': '字' * (1024 * 1024),
        }),
        throwsArgumentError,
      );
      expect(calls, 0);
    },
  );

  testWidgets('phone export awaits save result and reports success once', (
    tester,
  ) async {
    final backend = ExportBackend()..pendingExport = Completer<String?>();
    await openExportPage(tester, backend);
    await tester.tap(find.byKey(const Key('export-diagnostics')));
    await tester.pump();
    expect(find.text('保存中…'), findsOneWidget);
    expect(
      tester
          .widget<OutlinedButton>(find.byKey(const Key('export-diagnostics')))
          .onPressed,
      isNull,
    );
    expect(backend.exportCalls, 1);
    expect(backend.transferredReport, redactedReport);
    backend.pendingExport!.complete('yingxia_diagnostics.json');
    await tester.pumpAndSettle();
    expect(find.text('已保存：yingxia_diagnostics.json'), findsOneWidget);
    expect(find.text('导出诊断'), findsOneWidget);
    expect(backend.calls.where((path) => path == 'report'), hasLength(1));
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('phone document cancellation is quiet and enables retry', (
    tester,
  ) async {
    final backend = ExportBackend();
    await openExportPage(tester, backend);
    await tester.tap(find.byKey(const Key('export-diagnostics')));
    await tester.pumpAndSettle();
    expect(backend.exportCalls, 1);
    expect(find.byType(SnackBar), findsNothing);
    expect(
      tester
          .widget<OutlinedButton>(find.byKey(const Key('export-diagnostics')))
          .onPressed,
      isNotNull,
    );
    backend.destination = 'yingxia_diagnostics.json';
    await tester.tap(find.byKey(const Key('export-diagnostics')));
    await tester.pumpAndSettle();
    expect(backend.exportCalls, 2);
    expect(find.text('已保存：yingxia_diagnostics.json'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });
}
