// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/yingxia_app.dart';

import 'yingxia_widget_test.dart' show FakeBackend, MemoryQualityPreferences;

class _TvBackend extends FakeBackend {
  @override
  bool get isAndroidTv => true;

  String? resolvedEpisode;

  @override
  Future<dynamic> post(String path, Json data) {
    if (path == 'resolve') resolvedEpisode = data['episode_key'] as String?;
    return super.post(path, data);
  }
}

Future<void> _open(WidgetTester tester, _TvBackend backend, double dpr) async {
  tester.view.devicePixelRatio = dpr;
  tester.view.physicalSize = Size(960 * dpr, 540 * dpr);
  addTearDown(tester.view.resetDevicePixelRatio);
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetViewInsets);
  await tester.pumpWidget(
    YingXiaApp(
      backend: backend,
      qualityPreferences: MemoryQualityPreferences(),
    ),
  );
  await tester.pumpAndSettle();
}

Finder _anchor(String id) => find.byKey(ValueKey('tv-anchor:$id'));

InkWell _anchoredInkWell(WidgetTester tester, String id) => tester.widget(
  find.descendant(of: _anchor(id), matching: find.byType(InkWell)).first,
);

void main() {
  for (final dpr in [2.0, 4.0]) {
    testWidgets(
      'TV ${960 * dpr} keeps rail and editor through keyboard insets',
      (tester) async {
        await _open(tester, _TvBackend(), dpr);
        final entry = find.byKey(const Key('search-tv-entry'));
        expect(
          tester.widget<OutlinedButton>(entry).focusNode!.hasPrimaryFocus,
          true,
        );
        final shell = find.byKey(const Key('tv-safe-content'));
        expect(
          tester.widget<Padding>(shell).padding,
          const EdgeInsets.symmetric(horizontal: 48, vertical: 27),
        );

        await tester.sendKeyEvent(LogicalKeyboardKey.select);
        await tester.pumpAndSettle();
        final editor = find.byKey(const Key('search-query'));
        final editFocus = tester.widget<TextField>(editor).focusNode!;
        expect(editFocus.hasPrimaryFocus, true);

        tester.view.viewInsets = FakeViewPadding(bottom: 280 * dpr);
        await tester.pumpAndSettle();
        expect(find.byKey(const Key('nav-0')), findsOneWidget);
        expect(find.byType(NavigationBar), findsNothing);
        expect(tester.widget<TextField>(editor).focusNode, same(editFocus));
        expect(editFocus.hasPrimaryFocus, true);
        expect(tester.takeException(), isNull);

        tester.view.resetViewInsets();
        await tester.pumpAndSettle();
        expect(
          tester.widget<OutlinedButton>(entry).focusNode!.hasPrimaryFocus,
          true,
        );
        await tester.pumpWidget(const SizedBox());
      },
    );
  }

  testWidgets(
    'remote search, select episode, play and Back restore the result',
    (tester) async {
      final backend = _TvBackend();
      await _open(tester, backend, 4);
      await tester.sendKeyEvent(LogicalKeyboardKey.select);
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const Key('search-query')), '测试电影');
      await tester.testTextInput.receiveAction(TextInputAction.search);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 500));
      await tester.pumpAndSettle();
      expect(
        _anchoredInkWell(tester, 'result:film-1').focusNode!.hasPrimaryFocus,
        true,
      );

      await tester.sendKeyEvent(LogicalKeyboardKey.select);
      await tester.pumpAndSettle();
      expect(
        _anchoredInkWell(tester, 'episode:e2').focusNode!.hasPrimaryFocus,
        true,
      );
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowLeft);
      await tester.pumpAndSettle();
      expect(
        _anchoredInkWell(tester, 'episode:e1').focusNode!.hasPrimaryFocus,
        true,
      );
      final workbench = find.byKey(const PageStorageKey('tv-page-1'));
      final workbenchScroll = tester.state<ScrollableState>(
        find.descendant(of: workbench, matching: find.byType(Scrollable)).first,
      );
      final play = find.byKey(const Key('play-selected'));
      final scrollBeforeSelection = workbenchScroll.position.pixels;
      expect(tester.getRect(workbench).contains(tester.getCenter(play)), true);
      await tester.sendKeyEvent(LogicalKeyboardKey.select);
      await tester.pumpAndSettle();
      expect(
        tester.widget<FilledButton>(play).focusNode!.hasPrimaryFocus,
        true,
      );
      expect(workbenchScroll.position.pixels, scrollBeforeSelection);
      await tester.sendKeyEvent(LogicalKeyboardKey.select);
      await tester.pumpAndSettle();
      expect(backend.resolvedEpisode, 'e1');

      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      expect(
        _anchoredInkWell(tester, 'result:film-1').focusNode!.hasPrimaryFocus,
        true,
      );
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
}
