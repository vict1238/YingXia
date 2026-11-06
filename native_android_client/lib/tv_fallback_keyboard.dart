// SPDX-License-Identifier: GPL-3.0-or-later
part of 'yingxia_app.dart';

/// An app-owned search keyboard. Every key is a real focusable button;
/// opening it creates no EditableText or platform input connection.
Future<String?> showTvFallbackKeyboard(
  BuildContext context, {
  required String initialText,
  Future<List<String>> Function(String)? lookup,
}) => showDialog<String>(
  context: context,
  builder:
      (context) => _TvFallbackKeyboard(
        initialText: initialText,
        lookup: lookup ?? lookupTvPinyin,
      ),
);

class _TvFallbackKey {
  const _TvFallbackKey(this.id, this.label, this.action, {this.semanticsLabel});

  final String id;
  final String label;
  final VoidCallback? action;
  final String? semanticsLabel;
}

class _TvFallbackKeyboard extends StatefulWidget {
  const _TvFallbackKeyboard({required this.initialText, required this.lookup});

  final String initialText;
  final Future<List<String>> Function(String) lookup;

  @override
  State<_TvFallbackKeyboard> createState() => _TvFallbackKeyboardState();
}

class _TvFallbackKeyboardState extends State<_TvFallbackKeyboard> {
  static const _limit = 120;
  static const _pageSize = 6;
  static const _letterRows = ['qwertyuiop', 'asdfghjkl', 'zxcvbnm'];
  static const _digits = '1234567890';
  static const _accept = [
    LogicalKeyboardKey.select,
    LogicalKeyboardKey.enter,
    LogicalKeyboardKey.numpadEnter,
    LogicalKeyboardKey.gameButtonA,
    LogicalKeyboardKey.space,
  ];

  final _nodes = <String, FocusNode>{};
  late String _text = _bounded(widget.initialText);
  String _pinyin = '';
  String _lastLetter = 'letter-q';
  bool _chinese = true;
  bool _loading = false;
  bool _lookupFailed = false;
  List<String> _candidates = const [];
  int _page = 0;
  int _lookupGeneration = 0;

  String get _query => '$_text$_pinyin'.trim();
  int get _pageCount => (_candidates.length + _pageSize - 1) ~/ _pageSize;

  static String _bounded(String text) =>
      String.fromCharCodes(text.runes.take(_limit));

  static String _dropLast(String value) =>
      value.isEmpty
          ? value
          : String.fromCharCodes(value.runes.take(value.runes.length - 1));

  FocusNode _node(String id) =>
      _nodes.putIfAbsent(id, () => FocusNode(debugLabel: '映匣遥控键盘 $id'));

  @override
  void dispose() {
    for (final node in _nodes.values) {
      node.dispose();
    }
    super.dispose();
  }

  void _lookupCandidates() {
    final generation = ++_lookupGeneration;
    final query = _pinyin;
    _page = 0;
    _candidates = const [];
    _loading = query.isNotEmpty;
    _lookupFailed = false;
    if (query.isEmpty) return;
    unawaited(() async {
      try {
        final result = await widget.lookup(query);
        if (!mounted || generation != _lookupGeneration) return;
        setState(() {
          _candidates =
              result.where((value) => value.trim().isNotEmpty).toSet().toList();
          _loading = false;
        });
      } catch (_) {
        if (!mounted || generation != _lookupGeneration) return;
        setState(() {
          _loading = false;
          _lookupFailed = true;
        });
      }
    }());
  }

  void _typeLetter(String letter) {
    _lastLetter = 'letter-$letter';
    if ((_text + _pinyin).runes.length >= _limit) return;
    setState(() {
      if (_chinese) {
        _pinyin += letter;
        _lookupCandidates();
      } else {
        _text += letter;
      }
    });
  }

  void _insertLiteral(String value) {
    if ((_text + _pinyin).runes.length >= _limit) return;
    setState(() {
      _text = _bounded('$_text$_pinyin$value');
      _pinyin = '';
      _lookupCandidates();
    });
  }

  void _switchMode() {
    setState(() {
      _text = _bounded('$_text$_pinyin');
      _pinyin = '';
      _chinese = !_chinese;
      _lookupCandidates();
    });
  }

  void _delete() {
    setState(() {
      if (_pinyin.isNotEmpty) {
        _pinyin = _dropLast(_pinyin);
        _lookupCandidates();
      } else {
        _text = _dropLast(_text);
      }
    });
  }

  void _clear() {
    setState(() {
      _text = '';
      _pinyin = '';
      _lookupCandidates();
    });
  }

  void _changePage(int delta) {
    final next = _page + delta;
    if (next < 0 || next >= _pageCount) return;
    setState(() => _page = next);
    // The pager just pressed may become disabled at either boundary.
    if ((delta < 0 && next == 0) || (delta > 0 && next == _pageCount - 1)) {
      _node('candidate-0').requestFocus();
    }
  }

  void _commit(String value) {
    setState(() {
      _text = _bounded('$_text$value');
      _pinyin = '';
      _lookupCandidates();
    });
    _node(_lastLetter).requestFocus();
  }

  void _submit() {
    if (_query.isNotEmpty) Navigator.pop(context, _query);
  }

  List<List<_TvFallbackKey>> _rows() => [
    [
      _TvFallbackKey(
        'previous',
        '‹',
        _page > 0 ? () => _changePage(-1) : null,
        semanticsLabel: '上一页候选',
      ),
      for (var slot = 0; slot < _pageSize; slot++)
        if (_page * _pageSize + slot < _candidates.length)
          _TvFallbackKey(
            'candidate-$slot',
            _candidates[_page * _pageSize + slot],
            () => _commit(_candidates[_page * _pageSize + slot]),
          )
        else
          _TvFallbackKey('candidate-$slot', '—', null),
      _TvFallbackKey(
        'next',
        '›',
        _page + 1 < _pageCount ? () => _changePage(1) : null,
        semanticsLabel: '下一页候选',
      ),
    ],
    for (final row in _letterRows)
      [
        for (final letter in row.split(''))
          _TvFallbackKey('letter-$letter', letter, () => _typeLetter(letter)),
      ],
    [
      for (final digit in _digits.split(''))
        _TvFallbackKey('digit-$digit', digit, () => _insertLiteral(digit)),
    ],
    [
      _TvFallbackKey(
        'mode',
        _chinese ? '中／英' : '英／中',
        _switchMode,
        semanticsLabel: _chinese ? '当前拼音，切换英文' : '当前英文，切换拼音',
      ),
      _TvFallbackKey('space', '空格', () => _insertLiteral(' ')),
      _TvFallbackKey('delete', '退格', _delete),
      _TvFallbackKey('clear', '清空', _clear),
      _TvFallbackKey('search', '搜索', _query.isEmpty ? null : _submit),
      _TvFallbackKey('close', '取消', () => Navigator.pop(context)),
    ],
  ];

  KeyEventResult _handleKey(FocusNode _, KeyEvent event) {
    if (event is! KeyDownEvent && event is! KeyRepeatEvent) {
      return KeyEventResult.ignored;
    }
    final key = event.logicalKey;
    // Android handles KEYCODE_BACK with a platform route-pop. Do not pop the
    // dialog on the duplicate logical GoBack event.
    if (key == LogicalKeyboardKey.goBack) return KeyEventResult.ignored;
    if (key == LogicalKeyboardKey.escape ||
        key == LogicalKeyboardKey.gameButtonB) {
      if (event is KeyDownEvent) Navigator.pop(context);
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.backspace ||
        key == LogicalKeyboardKey.delete) {
      _delete();
      return KeyEventResult.handled;
    }
    final horizontal =
        key == LogicalKeyboardKey.arrowLeft
            ? -1
            : key == LogicalKeyboardKey.arrowRight
            ? 1
            : 0;
    final vertical =
        key == LogicalKeyboardKey.arrowUp
            ? -1
            : key == LogicalKeyboardKey.arrowDown
            ? 1
            : 0;
    if (horizontal == 0 && vertical == 0 && !_accept.contains(key)) {
      return KeyEventResult.ignored;
    }

    final rows = _rows();
    for (var y = 0; y < rows.length; y++) {
      final row = rows[y];
      final x = row.indexWhere(
        (item) => _nodes[item.id]?.hasPrimaryFocus == true,
      );
      if (x < 0) continue;
      if (_accept.contains(key)) {
        if (event is KeyDownEvent) row[x].action?.call();
      } else if (horizontal != 0) {
        for (
          var i = x + horizontal;
          i >= 0 && i < row.length;
          i += horizontal
        ) {
          if (row[i].action == null) continue;
          _moveFocus(row[i]);
          break;
        }
      } else {
        final center = (x + .5) / row.length;
        for (var i = y + vertical; i >= 0 && i < rows.length; i += vertical) {
          final next = rows[i];
          _TvFallbackKey? target;
          var nearest = double.infinity;
          for (var j = 0; j < next.length; j++) {
            if (next[j].action == null) continue;
            final distance = ((j + .5) / next.length - center).abs();
            if (distance < nearest) {
              nearest = distance;
              target = next[j];
            }
          }
          if (target != null) {
            _moveFocus(target);
            break;
          }
        }
      }
      return KeyEventResult.handled;
    }
    _node('letter-q').requestFocus();
    return KeyEventResult.handled;
  }

  void _moveFocus(_TvFallbackKey item) {
    final node = _node(item.id);
    node.requestFocus();
    if (node.context case final context?) {
      unawaited(
        Scrollable.ensureVisible(
          context,
          alignmentPolicy: ScrollPositionAlignmentPolicy.keepVisibleAtEnd,
        ),
      );
      unawaited(
        Scrollable.ensureVisible(
          context,
          alignmentPolicy: ScrollPositionAlignmentPolicy.keepVisibleAtStart,
        ),
      );
    }
  }

  Widget _buildKey(_TvFallbackKey item, double height) {
    final search = item.id == 'search';
    final label = Text(
      item.label,
      maxLines: 1,
      textAlign: TextAlign.center,
      style: TextStyle(
        fontSize: 16,
        fontWeight: search ? FontWeight.w700 : FontWeight.w500,
      ),
    );
    return Expanded(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 2),
        child: SizedBox(
          height: height,
          child: OutlinedButton(
            key: ValueKey('tv-fallback-${item.id}'),
            autofocus: item.id == 'letter-q',
            focusNode: _node(item.id),
            onPressed: item.action,
            style: ButtonStyle(
              padding: const WidgetStatePropertyAll(
                EdgeInsets.symmetric(horizontal: 2),
              ),
              minimumSize: const WidgetStatePropertyAll(Size.zero),
              tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              visualDensity: VisualDensity.standard,
              backgroundColor: WidgetStateProperty.resolveWith((states) {
                if (states.contains(WidgetState.disabled)) return surface;
                if (states.contains(WidgetState.focused)) {
                  return const Color(0xffc9ffeb);
                }
                return search ? accent : surface2;
              }),
              foregroundColor: WidgetStateProperty.resolveWith((states) {
                if (states.contains(WidgetState.disabled)) return muted;
                if (search || states.contains(WidgetState.focused)) {
                  return background;
                }
                return foreground;
              }),
              side: WidgetStateProperty.resolveWith(
                (states) => BorderSide(
                  color: states.contains(WidgetState.focused) ? accent : border,
                  width: states.contains(WidgetState.focused) ? 2 : 1,
                ),
              ),
              shape: WidgetStatePropertyAll(
                RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
              ),
            ),
            child: FittedBox(
              fit: BoxFit.scaleDown,
              child:
                  item.semanticsLabel == null
                      ? label
                      : Semantics(
                        label: item.semanticsLabel,
                        excludeSemantics: true,
                        child: label,
                      ),
            ),
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final rows = _rows();
    final status =
        _lookupFailed
            ? '汉字候选暂不可用，可直接按拼音搜索'
            : _loading
            ? '正在读取离线候选…'
            : _pageCount > 0
            ? '汉字候选 ${_page + 1}/$_pageCount · 按 ↑ 选择'
            : _chinese
            ? '输入拼音后按 ↑ 选择汉字，也可直接搜索拼音'
            : '英文输入 · 可输入片名或首字母';
    return Dialog(
      key: const ValueKey('tv-fallback-dialog'),
      backgroundColor: background,
      insetPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 10),
      child: Focus(
        canRequestFocus: false,
        skipTraversal: true,
        onKeyEvent: _handleKey,
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 760),
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(16),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Text(
                  '遥控器输入',
                  style: TextStyle(fontSize: 22, fontWeight: FontWeight.w600),
                ),
                const SizedBox(height: 8),
                Container(
                  key: const ValueKey('tv-fallback-preview'),
                  decoration: BoxDecoration(
                    color: surface,
                    border: Border.all(color: border),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 7,
                  ),
                  child: Text.rich(
                    TextSpan(
                      children: [
                        if (_text.isEmpty && _pinyin.isEmpty)
                          const TextSpan(
                            text: '输入片名',
                            style: TextStyle(color: muted),
                          ),
                        TextSpan(text: _text),
                        if (_pinyin.isNotEmpty)
                          TextSpan(
                            text: _pinyin,
                            style: const TextStyle(
                              color: accent,
                              decoration: TextDecoration.underline,
                            ),
                          ),
                      ],
                    ),
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(fontSize: 20, color: foreground),
                  ),
                ),
                const SizedBox(height: 5),
                Text(
                  status,
                  maxLines: 2,
                  style: const TextStyle(fontSize: 13, color: muted),
                ),
                const SizedBox(height: 5),
                for (var i = 0; i < rows.length; i++) ...[
                  if (i != 0) const SizedBox(height: 5),
                  Row(
                    children: [for (final item in rows[i]) _buildKey(item, 38)],
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}
