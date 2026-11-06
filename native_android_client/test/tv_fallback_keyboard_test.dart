// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/yingxia_app.dart';

const left = LogicalKeyboardKey.arrowLeft;
const right = LogicalKeyboardKey.arrowRight;
const up = LogicalKeyboardKey.arrowUp;
const down = LogicalKeyboardKey.arrowDown;
const ok = LogicalKeyboardKey.select;

Finder key(String name) => find.byKey(ValueKey('tv-fallback-$name'));

void focused(WidgetTester tester, String name) {
  expect(
    tester.widget<OutlinedButton>(key(name)).focusNode!.hasPrimaryFocus,
    true,
  );
}

Future<void> keys(
  WidgetTester tester,
  List<LogicalKeyboardKey> sequence,
) async {
  for (final item in sequence) {
    await tester.sendKeyEvent(item);
    await tester.pumpAndSettle();
  }
}

Future<ValueNotifier<String>> open(
  WidgetTester tester, {
  String initial = '',
  required Future<List<String>> Function(String) lookup,
  Size size = const Size(3840, 2160),
  double dpr = 4,
}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = dpr;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  final result = ValueNotifier('not submitted');
  addTearDown(result.dispose);
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: Builder(
          builder:
              (context) => TextButton(
                child: const Text('launch'),
                onPressed: () async {
                  final value = await showTvFallbackKeyboard(
                    context,
                    initialText: initial,
                    lookup: lookup,
                  );
                  if (value != null) result.value = value;
                },
              ),
        ),
      ),
    ),
  );
  await tester.tap(find.text('launch'));
  await tester.pumpAndSettle();
  focused(tester, 'letter-q');
  return result;
}

void main() {
  testWidgets(
    'D-pad pinyin composes and submits Chinese without platform IME',
    (tester) async {
      final result = await open(
        tester,
        lookup: (text) async => text == 'ni' ? ['你'] : [],
      );
      expect(find.byType(EditableText), findsNothing);
      expect(tester.testTextInput.isVisible, false);

      await keys(tester, [down, down, right, right, right, right, right]);
      focused(tester, 'letter-n');
      await keys(tester, [ok, up, up, left]);
      focused(tester, 'letter-i');
      await keys(tester, [ok, up]);
      focused(tester, 'candidate-0');
      expect(find.text('你'), findsOneWidget);
      await keys(tester, [ok]);
      focused(tester, 'letter-i');
      await keys(tester, [down, down, down, down]);
      focused(tester, 'search');
      await keys(tester, [ok]);
      expect(result.value, '你');
      expect(key('dialog'), findsNothing);
      expect(tester.testTextInput.isVisible, false);
    },
  );

  testWidgets('last candidate page can be selected by remote', (tester) async {
    final result = await open(
      tester,
      lookup: (text) async => ['甲', '乙', '丙', '丁', '戊', '己', '庚', '辛'],
    );
    await keys(tester, [ok, up, right, right, right, right, right, right]);
    focused(tester, 'next');
    await keys(tester, [ok]);
    focused(tester, 'candidate-0');
    expect(find.text('庚'), findsOneWidget);
    await keys(tester, [ok]);
    focused(tester, 'letter-q');
    await keys(tester, [
      down,
      down,
      down,
      down,
      right,
      right,
      right,
      right,
      ok,
    ]);
    expect(result.value, '庚');
  });

  testWidgets(
    'mode, space, delete and digits remain reachable on narrow view',
    (tester) async {
      final result = await open(
        tester,
        initial: 'a',
        lookup: (_) async => [],
        size: const Size(360, 640),
        dpr: 1,
      );
      await keys(tester, [down, down, down, down]);
      focused(tester, 'mode');
      await keys(tester, [ok, right, ok, right, ok, up]);
      focused(tester, 'digit-5');
      await keys(tester, [ok, down, right, right]);
      focused(tester, 'search');
      await keys(tester, [ok]);
      expect(result.value, 'a5');
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets('raw pinyin still submits when dictionary lookup fails', (
    tester,
  ) async {
    final result = await open(
      tester,
      lookup: (_) async => throw StateError('test'),
    );
    await keys(tester, [ok]);
    expect(find.text('汉字候选暂不可用，可直接按拼音搜索'), findsOneWidget);
    await keys(tester, [
      down,
      down,
      down,
      down,
      right,
      right,
      right,
      right,
      ok,
    ]);
    expect(result.value, 'q');
  });

  testWidgets('Android logical GoBack does not duplicate platform route Back', (
    tester,
  ) async {
    final result = await open(tester, initial: '保留', lookup: (_) async => []);
    await tester.sendKeyDownEvent(
      LogicalKeyboardKey.goBack,
      physicalKey: PhysicalKeyboardKey.escape,
    );
    await tester.sendKeyUpEvent(
      LogicalKeyboardKey.goBack,
      physicalKey: PhysicalKeyboardKey.escape,
    );
    await tester.pumpAndSettle();
    expect(key('dialog'), findsOneWidget);
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(key('dialog'), findsNothing);
    expect(result.value, 'not submitted');
  });
}
