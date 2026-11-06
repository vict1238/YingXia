// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/yingxia_app.dart';

import 'playback_features_test.dart' show FeatureBackend;

void main() {
  final entry = find.byKey(const Key('search-remote-entry'));
  final dialog = find.byKey(const Key('tv-fallback-dialog'));

  for (final tvFlag in [true, false]) {
    testWidgets('remote input opens without system IME (TV flag $tvFlag)', (
      tester,
    ) async {
      tester.view.physicalSize = const Size(3840, 2160);
      tester.view.devicePixelRatio = 4;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await tester.pumpWidget(
        YingXiaApp(backend: FeatureBackend()..tv = tvFlag),
      );
      await tester.pumpAndSettle();

      expect(
        tester.widget<FilledButton>(entry).focusNode!.hasPrimaryFocus,
        true,
      );
      tester.testTextInput.log.clear();
      await tester.sendKeyEvent(LogicalKeyboardKey.select);
      await tester.pumpAndSettle();
      expect(dialog, findsOneWidget);
      expect(
        find.descendant(of: dialog, matching: find.byType(EditableText)),
        findsNothing,
      );
      expect(
        tester.testTextInput.log.where((m) => m.method == 'TextInput.show'),
        isEmpty,
      );
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      expect(dialog, findsNothing);
      expect(
        tester.widget<FilledButton>(entry).focusNode!.hasPrimaryFocus,
        true,
      );
      await tester.pumpWidget(const SizedBox());
    });
  }

  testWidgets('remote entry is available even without a TV flag in portrait', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 640);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(YingXiaApp(backend: FeatureBackend()));
    await tester.pumpAndSettle();
    expect(entry, findsOneWidget);
    expect(find.byKey(const Key('search-query')), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });
}
