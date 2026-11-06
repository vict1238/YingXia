// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/yingxia_app.dart';

import 'playback_features_test.dart' show FeatureBackend;

void main() {
  final entry = find.byKey(const Key('search-tv-entry'));
  final editor = find.byKey(const Key('search-query'));

  FocusNode entryFocus(WidgetTester tester) =>
      tester.widget<OutlinedButton>(entry).focusNode!;

  Future<FeatureBackend> open(WidgetTester tester) async {
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.5;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    addTearDown(tester.view.resetViewInsets);
    final backend = FeatureBackend()..tv = true;
    await tester.pumpWidget(YingXiaApp(backend: backend));
    await tester.pumpAndSettle();
    entryFocus(tester).requestFocus();
    await tester.pumpAndSettle();
    expect(entryFocus(tester).hasPrimaryFocus, isTrue);
    expect(tester.testTextInput.isVisible, isFalse);
    return backend;
  }

  testWidgets('TV confirm opens and reopens IME; dismissal restores focus', (
    tester,
  ) async {
    await open(tester);
    for (final key in [
      LogicalKeyboardKey.select,
      LogicalKeyboardKey.enter,
      LogicalKeyboardKey.numpadEnter,
      LogicalKeyboardKey.gameButtonA,
    ]) {
      await tester.sendKeyEvent(key);
      await tester.pumpAndSettle();
      expect(editor, findsOneWidget, reason: key.debugName);
      expect(tester.testTextInput.isVisible, isTrue, reason: key.debugName);
      tester.testTextInput.enterText('测试电影');
      tester.view.viewInsets = const FakeViewPadding(bottom: 320);
      await tester.pumpAndSettle();
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
      await tester.pumpAndSettle();
      expect(editor, findsOneWidget, reason: 'IME owns directions while shown');

      tester.testTextInput.hide();
      tester.view.viewInsets = const FakeViewPadding();
      await tester.pumpAndSettle();
      expect(editor, findsNothing);
      expect(entryFocus(tester).hasPrimaryFocus, isTrue);
      expect(find.text('测试电影'), findsWidgets);
    }
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('TV IME without insets can retry or leave via arrows and Back', (
    tester,
  ) async {
    await open(tester);
    await tester.sendKeyEvent(LogicalKeyboardKey.select);
    await tester.pumpAndSettle();
    expect(tester.testTextInput.isVisible, isTrue);
    tester.testTextInput.hide();
    await tester.sendKeyEvent(LogicalKeyboardKey.select);
    await tester.pumpAndSettle();
    expect(tester.testTextInput.isVisible, isTrue);
    tester.testTextInput.hide();
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.pumpAndSettle();
    expect(editor, findsNothing);
    expect(entry, findsOneWidget);

    entryFocus(tester).requestFocus();
    await tester.pumpAndSettle();
    await tester.sendKeyEvent(LogicalKeyboardKey.select);
    await tester.pumpAndSettle();
    expect(editor, findsOneWidget);
    tester.testTextInput.hide();
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(editor, findsNothing);
    expect(entryFocus(tester).hasPrimaryFocus, isTrue);
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('TV system keyboard Search submits once and exits editor', (
    tester,
  ) async {
    final backend = await open(tester);
    await tester.sendKeyEvent(LogicalKeyboardKey.select);
    await tester.pumpAndSettle();
    tester.testTextInput.enterText('测试电影');
    await tester.pump();
    await tester.testTextInput.receiveAction(TextInputAction.search);
    await tester.pumpAndSettle();
    expect(backend.calls.where((call) => call == 'search/start'), hasLength(1));
    expect(editor, findsNothing);
    expect(entry, findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
}
