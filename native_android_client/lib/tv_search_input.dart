// SPDX-License-Identifier: GPL-3.0-or-later
part of 'yingxia_app.dart';

/// Moving focus around the TV page never opens the keyboard. Activating this
/// entry mounts a real editor; dismissing it returns focus to the entry.
class _TvSearchInput extends StatefulWidget {
  const _TvSearchInput({
    required this.controller,
    required this.onSubmitted,
    this.focusNode,
  });

  final TextEditingController controller;
  final ValueChanged<String> onSubmitted;
  final FocusNode? focusNode;

  @override
  State<_TvSearchInput> createState() => _TvSearchInputState();
}

class _TvSearchInputState extends State<_TvSearchInput>
    with WidgetsBindingObserver {
  static const _confirmKeys = <LogicalKeyboardKey>[
    LogicalKeyboardKey.select,
    LogicalKeyboardKey.enter,
    LogicalKeyboardKey.numpadEnter,
    LogicalKeyboardKey.gameButtonA,
  ];

  final _localEntry = FocusNode(debugLabel: '电视搜索入口');
  final _editorFocus = FocusNode(debugLabel: '电视搜索编辑');
  final _editorHost = GlobalKey();
  bool _editing = false;
  bool _imeWasVisible = false;
  bool _editorHadFocus = false;
  int _editSession = 0;

  FocusNode get _entry => widget.focusNode ?? _localEntry;
  bool get _imeVisible {
    final insets = View.of(context).viewInsets;
    return insets.bottom > 0 ||
        insets.top > 0 ||
        insets.left > 0 ||
        insets.right > 0;
  }

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _editorFocus.addListener(_editorFocusChanged);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _editorFocus.removeListener(_editorFocusChanged);
    _editorFocus.dispose();
    _localEntry.dispose();
    super.dispose();
  }

  @override
  void didChangeMetrics() {
    if (!_editing) return;
    if (_imeVisible) {
      _imeWasVisible = true;
      return;
    }
    if (!_imeWasVisible) return;
    _imeWasVisible = false;
    final session = _editSession;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _editing && session == _editSession && !_imeVisible) {
        _closeEditor();
      }
    });
  }

  void _editorFocusChanged() {
    if (!_editing) return;
    if (_editorFocus.hasFocus) {
      _editorHadFocus = true;
    } else if (_editorHadFocus) {
      // When a pointer picked another control, keep its focus.
      _closeEditor(restoreFocus: false);
    }
  }

  void _openEditor() {
    if (_editing) return;
    setState(() {
      _editing = true;
      _imeWasVisible = false;
      _editorHadFocus = false;
      _editSession++;
    });
    final session = _editSession;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _editing && session == _editSession) {
        _requestKeyboard();
      }
    });
  }

  void _requestKeyboard() {
    // TextField owns its EditableText. Find that specific descendant so the
    // platform IME request always has an attached, real input client.
    EditableTextState? editor;
    void visit(Element element) {
      if (element is StatefulElement && element.state is EditableTextState) {
        editor = element.state as EditableTextState;
      } else if (editor == null) {
        element.visitChildElements(visit);
      }
    }

    _editorHost.currentContext?.visitChildElements(visit);
    editor?.requestKeyboard();
  }

  void _closeEditor({bool restoreFocus = true, TraversalDirection? direction}) {
    if (!_editing) return;
    setState(() => _editing = false);
    if (!restoreFocus) return;
    final session = _editSession;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || _editing || session != _editSession) return;
      _entry.requestFocus();
      if (direction != null) {
        scheduleMicrotask(() {
          if (mounted && !_editing && _entry.hasPrimaryFocus) {
            _entry.focusInDirection(direction);
          }
        });
      }
    });
  }

  KeyEventResult _handleEditorKey(FocusNode _, KeyEvent event) {
    // Let the OS keyboard handle its own directions, confirmation and text.
    if (event is! KeyDownEvent || _imeVisible) return KeyEventResult.ignored;
    final key = event.logicalKey;
    if (_confirmKeys.contains(key)) {
      _requestKeyboard();
      return KeyEventResult.handled;
    }
    final direction = switch (key) {
      LogicalKeyboardKey.arrowLeft => TraversalDirection.left,
      LogicalKeyboardKey.arrowRight => TraversalDirection.right,
      LogicalKeyboardKey.arrowUp => TraversalDirection.up,
      LogicalKeyboardKey.arrowDown => TraversalDirection.down,
      _ => null,
    };
    if (direction != null) {
      // No IME insets (including OEMs that decline to show it): never strand
      // remote focus inside the cursor's arrow-key actions.
      _closeEditor(direction: direction);
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.escape) {
      _closeEditor();
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  @override
  Widget build(BuildContext context) {
    if (_editing) {
      return PopScope(
        canPop: false,
        onPopInvokedWithResult: (didPop, _) {
          if (!didPop) _closeEditor();
        },
        child: Focus(
          canRequestFocus: false,
          skipTraversal: true,
          onKeyEvent: _handleEditorKey,
          child: KeyedSubtree(
            key: _editorHost,
            child: TextField(
              key: const Key('search-query'),
              controller: widget.controller,
              focusNode: _editorFocus,
              maxLength: 120,
              textInputAction: TextInputAction.search,
              onSubmitted: (value) {
                _closeEditor();
                widget.onSubmitted(value);
              },
              style: const TextStyle(fontSize: 16),
              decoration: const InputDecoration(
                hintText: '片名 / 拼音 / 首字母',
                prefixIcon: Icon(Icons.search_rounded, color: accent),
                counterText: '',
              ),
            ),
          ),
        ),
      );
    }
    return Shortcuts(
      shortcuts: {
        for (final key in _confirmKeys)
          SingleActivator(key, includeRepeats: false): const ActivateIntent(),
      },
      child: OutlinedButton(
        key: const Key('search-tv-entry'),
        focusNode: _entry,
        onPressed: _openEditor,
        style: OutlinedButton.styleFrom(
          minimumSize: const Size(0, 52),
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
          foregroundColor: foreground,
          backgroundColor: surface2,
          textStyle: const TextStyle(fontSize: 16),
        ).copyWith(
          side: WidgetStateProperty.resolveWith(
            (states) =>
                states.contains(WidgetState.focused)
                    ? const BorderSide(color: accent, width: 3)
                    : const BorderSide(color: border),
          ),
        ),
        child: ValueListenableBuilder<TextEditingValue>(
          valueListenable: widget.controller,
          builder:
              (context, value, _) => Row(
                children: [
                  const Icon(Icons.keyboard_outlined, color: accent),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      value.text.isEmpty ? '系统键盘' : value.text,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                ],
              ),
        ),
      ),
    );
  }
}
