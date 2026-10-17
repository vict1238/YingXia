// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

import 'video_quality.dart';

/// Only the video surface owns these shortcuts. Text fields and focused
/// controls keep their normal editing, slider and directional-navigation keys.
double playerControlsScale(Size viewport) =>
    viewport.width >= 1200 && viewport.height >= 700
        ? (viewport.shortestSide / 800).clamp(1.15, 1.6)
        : 1;

/// Small interface keeps controls testable without initializing a native player.
abstract class PlayerControlsAdapter extends ChangeNotifier {
  bool get playing;
  bool get buffering;
  Duration get position;
  Duration get duration;
  Duration get buffer;
  double get volume;
  double get rate;
  BoxFit get fit;
  bool get fullscreen;
  Future<void> playOrPause();
  Future<void> seek(Duration position);
  Future<void> setVolume(double volume);
  Future<void> setRate(double rate);
  Future<void> toggleFullscreen();
  void setFit(BoxFit fit);
}

class _MediaKitControlsAdapter extends PlayerControlsAdapter {
  _MediaKitControlsAdapter(this.player, this.video) {
    for (final stream in <Stream<dynamic>>[
      player.stream.playing,
      player.stream.buffering,
      player.stream.position,
      player.stream.duration,
      player.stream.buffer,
      player.stream.volume,
      player.stream.rate,
    ]) {
      _subscriptions.add(stream.listen((_) => _changed()));
    }
    video.videoViewParametersNotifier.addListener(_changed);
  }

  final Player player;
  final VideoState video;
  final _subscriptions = <StreamSubscription<dynamic>>[];
  bool _closed = false;
  void _changed() {
    if (!_closed) notifyListeners();
  }

  @override
  bool get playing => player.state.playing;
  @override
  bool get buffering => player.state.buffering;
  @override
  Duration get position => player.state.position;
  @override
  Duration get duration => player.state.duration;
  @override
  Duration get buffer => player.state.buffer;
  @override
  double get volume => player.state.volume;
  @override
  double get rate => player.state.rate;
  @override
  BoxFit get fit => video.videoViewParametersNotifier.value.fit;
  @override
  bool get fullscreen => video.isFullscreen();
  @override
  Future<void> playOrPause() => player.playOrPause();
  @override
  Future<void> seek(Duration position) => player.seek(position);
  @override
  Future<void> setVolume(double volume) => player.setVolume(volume);
  @override
  Future<void> setRate(double rate) => player.setRate(rate);
  @override
  Future<void> toggleFullscreen() => video.toggleFullscreen();
  @override
  void setFit(BoxFit fit) => video.update(fit: fit);

  @override
  void dispose() {
    _closed = true;
    video.videoViewParametersNotifier.removeListener(_changed);
    for (final subscription in _subscriptions) {
      unawaited(subscription.cancel());
    }
    super.dispose();
  }
}

/// Serializes a hold/release pair, including release while the native rate
/// change is still pending. An app coordinator can own the actual rate policy.
class TemporaryPlaybackRate {
  TemporaryPlaybackRate({
    required this.readRate,
    required this.writeRate,
    this.onChanged,
  });

  final double Function() readRate;
  final Future<void> Function(double) writeRate;
  final Future<void> Function(bool)? onChanged;
  Future<void> _tail = Future<void>.value();
  bool active = false;
  double _previous = 1;

  Future<void> begin() {
    if (active) return _tail;
    active = true;
    _previous = readRate();
    return _queue(() => onChanged?.call(true) ?? writeRate(2));
  }

  Future<void> end() {
    if (!active) return _tail;
    active = false;
    final previous = _previous;
    return _queue(() => onChanged?.call(false) ?? writeRate(previous));
  }

  Future<void> _queue(Future<void> Function() action) {
    final result = _tail.then((_) => action());
    _tail = result.catchError((Object _) {});
    return result;
  }
}

/// Native controls rendered inside Video.controls, including its fullscreen
/// route. Playback rate callbacks share the app's authoritative rate policy.
class YingXiaPlayerControls extends StatefulWidget {
  const YingXiaPlayerControls({
    super.key,
    required Player this.player,
    required VideoState this.videoState,
    required this.requestedQuality,
    required this.effectiveQuality,
    required this.onQualityChanged,
    this.onPlayOrPause,
    this.onRateChanged,
    this.onTemporaryRateChanged,
    this.cacheDescription,
    this.onCacheSettings,
    this.enableLongPress = false,
    this.autofocus = false,
    this.hideAfter = const Duration(seconds: 3),
  }) : adapter = null;

  @visibleForTesting
  const YingXiaPlayerControls.withAdapter({
    super.key,
    required PlayerControlsAdapter this.adapter,
    required this.requestedQuality,
    required this.effectiveQuality,
    required this.onQualityChanged,
    this.onPlayOrPause,
    this.onRateChanged,
    this.onTemporaryRateChanged,
    this.cacheDescription,
    this.onCacheSettings,
    this.enableLongPress = false,
    this.autofocus = false,
    this.hideAfter = const Duration(seconds: 3),
  }) : player = null,
       videoState = null;

  final Player? player;
  final VideoState? videoState;
  final PlayerControlsAdapter? adapter;
  final VideoQuality requestedQuality;
  final VideoQuality effectiveQuality;
  final FutureOr<void> Function(VideoQuality) onQualityChanged;
  final Future<void> Function()? onPlayOrPause;
  final Future<void> Function(double)? onRateChanged;
  final Future<void> Function(bool)? onTemporaryRateChanged;
  final String? cacheDescription;
  final FutureOr<void> Function()? onCacheSettings;
  final bool enableLongPress;
  final bool autofocus;
  final Duration hideAfter;

  @override
  State<YingXiaPlayerControls> createState() => _YingXiaPlayerControlsState();
}

class _YingXiaPlayerControlsState extends State<YingXiaPlayerControls>
    with WidgetsBindingObserver {
  late PlayerControlsAdapter _adapter;
  late TemporaryPlaybackRate _temporary;
  final _surfaceFocus = FocusNode(debugLabel: '映匣视频快捷键');
  final _settingsFocus = FocusNode(debugLabel: '映匣播放设置');
  final _volumeFocus = FocusNode(debugLabel: '映匣音量');
  Timer? _hideTimer;
  bool _visible = true;
  bool _panelOpen = false;
  bool _scrubbing = false;
  double? _dragPosition;
  double _lastVolume = 100;
  Offset _doubleTapPosition = Offset.zero;
  Size _viewport = Size.zero;

  @override
  void initState() {
    super.initState();
    _attach();
    WidgetsBinding.instance.addObserver(this);
    _scheduleHide();
  }

  void _attach() {
    _adapter =
        widget.adapter ??
        _MediaKitControlsAdapter(widget.player!, widget.videoState!);
    _adapter.addListener(_playbackChanged);
    final adapter = _adapter;
    _temporary = TemporaryPlaybackRate(
      readRate: () => adapter.rate,
      writeRate: (rate) => adapter.setRate(rate),
      onChanged:
          widget.onTemporaryRateChanged == null
              ? null
              : (active) => widget.onTemporaryRateChanged!(active),
    );
  }

  void _detach() {
    _run(_temporary.end());
    _adapter.removeListener(_playbackChanged);
    if (widget.adapter == null) _adapter.dispose();
  }

  @override
  void didUpdateWidget(YingXiaPlayerControls oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.player != widget.player ||
        oldWidget.videoState != widget.videoState ||
        oldWidget.adapter != widget.adapter) {
      _run(_temporary.end());
      _adapter.removeListener(_playbackChanged);
      if (oldWidget.adapter == null) _adapter.dispose();
      _attach();
    }
    if (!widget.enableLongPress) _endHold();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (ModalRoute.of(context)?.isCurrent == false) _endHold();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed) _endHold();
  }

  @override
  void dispose() {
    _hideTimer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    _detach();
    _surfaceFocus.dispose();
    _settingsFocus.dispose();
    _volumeFocus.dispose();
    super.dispose();
  }

  void _run(Future<void> action) {
    // The app reports media failures. A disposed/closing decoder should not
    // turn a pointer cancellation or lifecycle notification into an exception.
    unawaited(action.catchError((Object _) {}));
  }

  void _playbackChanged() {
    if (!mounted) return;
    if (!_adapter.playing) {
      _endHold();
      _visible = true;
    }
    setState(() {});
    _scheduleHide();
  }

  void _scheduleHide() {
    // Timeline stream updates must not perpetually restart the hide timer.
    if (_hideTimer?.isActive == true) return;
    if (!_adapter.playing || _panelOpen || _scrubbing || _temporary.active) {
      return;
    }
    _hideTimer = Timer(widget.hideAfter, () {
      if (!mounted || _panelOpen || _scrubbing || _temporary.active) return;
      if (_surfaceFocus.hasFocus && !_surfaceFocus.hasPrimaryFocus) return;
      setState(() => _visible = false);
    });
  }

  void _show({bool restart = true}) {
    if (!_visible) setState(() => _visible = true);
    if (restart) _hideTimer?.cancel();
    _scheduleHide();
  }

  void _beginHold() {
    if (!widget.enableLongPress || !_adapter.playing || _panelOpen) return;
    _hideTimer?.cancel();
    _run(_temporary.begin());
    setState(() {});
  }

  void _endHold() {
    if (!_temporary.active) return;
    _run(_temporary.end());
    if (mounted) setState(() {});
    _scheduleHide();
  }

  void _seekBy(int seconds) {
    final maximum = math.max(0, _adapter.duration.inMilliseconds);
    final target = (_adapter.position.inMilliseconds + seconds * 1000).clamp(
      0,
      maximum,
    );
    _run(_adapter.seek(Duration(milliseconds: target)));
    _show();
  }

  void _volume(double value) {
    _run(_adapter.setVolume(value.clamp(0, 100)));
    _show();
  }

  void _playOrPause() {
    _run(widget.onPlayOrPause?.call() ?? _adapter.playOrPause());
    _show();
  }

  void _togglePanel() {
    _endHold();
    setState(() {
      _visible = true;
      _panelOpen = !_panelOpen;
    });
    _hideTimer?.cancel();
    _scheduleHide();
  }

  bool get _editing {
    final focusContext = FocusManager.instance.primaryFocus?.context;
    return focusContext?.widget is EditableText ||
        focusContext?.findAncestorWidgetOfExactType<EditableText>() != null;
  }

  KeyEventResult _key(FocusNode node, KeyEvent event) {
    if (event is! KeyDownEvent && event is! KeyRepeatEvent) {
      return KeyEventResult.ignored;
    }
    if (_editing) return KeyEventResult.ignored;
    final key = event.logicalKey;
    if (key == LogicalKeyboardKey.audioVolumeUp ||
        key == LogicalKeyboardKey.audioVolumeDown) {
      _volume(
        _adapter.volume + (key == LogicalKeyboardKey.audioVolumeUp ? 5 : -5),
      );
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.mediaPlayPause) {
      _playOrPause();
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.escape && _panelOpen) {
      _togglePanel();
      _settingsFocus.requestFocus();
      return KeyEventResult.handled;
    }
    // Let focused buttons, menus and sliders handle their own keys.
    if (!_surfaceFocus.hasPrimaryFocus) return KeyEventResult.ignored;
    if (key == LogicalKeyboardKey.arrowLeft ||
        key == LogicalKeyboardKey.arrowRight) {
      _seekBy(key == LogicalKeyboardKey.arrowLeft ? -10 : 10);
    } else if (key == LogicalKeyboardKey.arrowDown ||
        key == LogicalKeyboardKey.arrowUp) {
      _show();
      _settingsFocus.requestFocus();
    } else if (key == LogicalKeyboardKey.space ||
        key == LogicalKeyboardKey.select ||
        key == LogicalKeyboardKey.enter ||
        key == LogicalKeyboardKey.gameButtonA) {
      _playOrPause();
    } else if (key == LogicalKeyboardKey.keyF) {
      _endHold();
      _run(_adapter.toggleFullscreen());
    } else if (key == LogicalKeyboardKey.escape && _adapter.fullscreen) {
      _endHold();
      _run(_adapter.toggleFullscreen());
    } else {
      return KeyEventResult.ignored;
    }
    return KeyEventResult.handled;
  }

  String _time(Duration value) {
    final seconds = math.max(0, value.inSeconds);
    final minutes = seconds ~/ 60;
    return '${minutes ~/ 60 > 0 ? '${minutes ~/ 60}:' : ''}'
        '${minutes ~/ 60 > 0 ? (minutes % 60).toString().padLeft(2, '0') : minutes}:'
        '${(seconds % 60).toString().padLeft(2, '0')}';
  }

  Widget _button(
    String name,
    IconData icon,
    VoidCallback action,
    double scale, {
    FocusNode? focusNode,
  }) {
    return IconButton(
      key: ValueKey('player-controls-$name'),
      focusNode: focusNode,
      tooltip: switch (name) {
        'play' => _adapter.playing ? '暂停' : '播放',
        'back' => '后退10秒',
        'forward' => '快进10秒',
        'settings' => '播放设置（音量／倍速／FSR）',
        'fullscreen' => _adapter.fullscreen ? '退出全屏' : '全屏',
        _ => name,
      },
      iconSize: 23 * scale,
      style: ButtonStyle(
        minimumSize: WidgetStatePropertyAll(Size.square(40 * scale)),
        padding: const WidgetStatePropertyAll(EdgeInsets.all(6)),
        foregroundColor: const WidgetStatePropertyAll(Colors.white),
        backgroundColor: WidgetStateProperty.resolveWith(
          (states) =>
              states.contains(WidgetState.focused) ? Colors.white24 : null,
        ),
        side: WidgetStateProperty.resolveWith(
          (states) =>
              states.contains(WidgetState.focused)
                  ? const BorderSide(color: Color(0xff89ddbd), width: 2)
                  : BorderSide.none,
        ),
      ),
      onPressed: () {
        _show();
        action();
      },
      icon: Icon(icon),
    );
  }

  Widget _panel(double scale) {
    final rate = _adapter.rate;
    final rates =
        <double>{0.5, 0.75, 1, 1.25, 1.5, 2, 3, rate}.toList()..sort();
    final cached = math.max(0, (_adapter.buffer - _adapter.position).inSeconds);
    return Material(
      key: const ValueKey('player-controls-panel'),
      color: const Color(0xf0182126),
      borderRadius: BorderRadius.circular(10),
      clipBehavior: Clip.antiAlias,
      child: SingleChildScrollView(
        padding: EdgeInsets.all(12 * scale),
        child: DefaultTextStyle.merge(
          style: TextStyle(color: Colors.white, fontSize: 14 * scale),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                children: [
                  IconButton(
                    key: const ValueKey('player-controls-mute'),
                    tooltip: _adapter.volume == 0 ? '取消静音' : '静音',
                    icon: Icon(
                      _adapter.volume == 0
                          ? Icons.volume_off_outlined
                          : Icons.volume_up_outlined,
                    ),
                    color: Colors.white,
                    onPressed: () {
                      if (_adapter.volume > 0) {
                        _lastVolume = _adapter.volume;
                        _volume(0);
                      } else {
                        _volume(_lastVolume);
                      }
                    },
                  ),
                  Expanded(
                    child: Slider(
                      key: const ValueKey('player-controls-volume'),
                      focusNode: _volumeFocus,
                      value: _adapter.volume.clamp(0, 100),
                      min: 0,
                      max: 100,
                      divisions: 20,
                      label: '音量 ${_adapter.volume.round()}%',
                      semanticFormatterCallback:
                          (value) => '音量 ${value.round()}%',
                      onChanged: _volume,
                    ),
                  ),
                  SizedBox(
                    width: 43 * scale,
                    child: Text('${_adapter.volume.round()}%'),
                  ),
                ],
              ),
              _settingRow(
                '倍速',
                DropdownButton<double>(
                  key: const ValueKey('player-controls-rate'),
                  value: rate,
                  isExpanded: true,
                  items:
                      rates
                          .map(
                            (value) => DropdownMenuItem(
                              value: value,
                              child: Text('$value×'),
                            ),
                          )
                          .toList(),
                  onChanged: (value) {
                    if (value == null) return;
                    _endHold();
                    _run(
                      widget.onRateChanged?.call(value) ??
                          _adapter.setRate(value),
                    );
                  },
                ),
                scale,
              ),
              _settingRow(
                'FSR',
                DropdownButton<VideoQuality>(
                  key: const ValueKey('player-controls-quality'),
                  value: widget.requestedQuality,
                  isExpanded: true,
                  items:
                      VideoQuality.values
                          .map(
                            (quality) => DropdownMenuItem(
                              value: quality,
                              child: Text(quality.label),
                            ),
                          )
                          .toList(),
                  onChanged: (value) {
                    if (value != null) {
                      _run(
                        Future<void>.sync(() => widget.onQualityChanged(value)),
                      );
                    }
                  },
                ),
                scale,
              ),
              if (widget.effectiveQuality != widget.requestedQuality)
                Align(
                  alignment: Alignment.centerLeft,
                  child: Text(
                    '当前生效：${widget.effectiveQuality.label}',
                    style: TextStyle(
                      fontSize: 12 * scale,
                      color: Colors.white70,
                    ),
                  ),
                ),
              _settingRow(
                '画面',
                DropdownButton<BoxFit>(
                  key: const ValueKey('player-controls-fit'),
                  value:
                      const [
                            BoxFit.contain,
                            BoxFit.cover,
                            BoxFit.fill,
                          ].contains(_adapter.fit)
                          ? _adapter.fit
                          : BoxFit.contain,
                  isExpanded: true,
                  items: const [
                    DropdownMenuItem(
                      value: BoxFit.contain,
                      child: Text('适应比例'),
                    ),
                    DropdownMenuItem(value: BoxFit.cover, child: Text('填满裁切')),
                    DropdownMenuItem(value: BoxFit.fill, child: Text('拉伸铺满')),
                  ],
                  onChanged: (value) {
                    if (value != null) _adapter.setFit(value);
                  },
                ),
                scale,
              ),
              Align(
                alignment: Alignment.centerLeft,
                child: Text(
                  widget.cacheDescription ?? '已缓冲约 $cached 秒',
                  key: const ValueKey('player-controls-cache'),
                  style: TextStyle(fontSize: 12 * scale, color: Colors.white70),
                ),
              ),
              if (widget.onCacheSettings != null)
                Align(
                  alignment: Alignment.centerRight,
                  child: TextButton(
                    key: const ValueKey('player-controls-cache-settings'),
                    onPressed: () {
                      _endHold();
                      _run(Future<void>.sync(widget.onCacheSettings!));
                    },
                    child: const Text('缓存设置'),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _settingRow(String label, Widget field, double scale) => Row(
    children: [
      SizedBox(width: 54 * scale, child: Text(label)),
      Expanded(child: field),
    ],
  );

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, bounds) {
      _viewport = bounds.biggest;
      final scale = playerControlsScale(_viewport);
      final duration = math.max(1, _adapter.duration.inMilliseconds).toDouble();
      final position = (_dragPosition ??
              _adapter.position.inMilliseconds.toDouble())
          .clamp(0.0, duration);
      final controlsHeight = 82 * scale;
      return FocusTraversalGroup(
        child: Focus(
          focusNode: _surfaceFocus,
          autofocus: widget.autofocus,
          onKeyEvent: _key,
          onFocusChange: (focused) {
            if (focused) _show();
          },
          child: MouseRegion(
            onHover: (_) => _show(),
            child: Material(
              type: MaterialType.transparency,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  GestureDetector(
                    key: const ValueKey('player-controls-surface'),
                    behavior: HitTestBehavior.opaque,
                    onTap: () {
                      _surfaceFocus.requestFocus();
                      if (_panelOpen) {
                        _togglePanel();
                      } else {
                        setState(() => _visible = !_visible);
                        _hideTimer?.cancel();
                        _scheduleHide();
                      }
                    },
                    onDoubleTapDown:
                        (details) => _doubleTapPosition = details.localPosition,
                    onDoubleTap: () {
                      _endHold();
                      if (widget.enableLongPress) {
                        if (_doubleTapPosition.dx < _viewport.width / 3) {
                          _seekBy(-10);
                        } else if (_doubleTapPosition.dx >
                            _viewport.width * 2 / 3) {
                          _seekBy(10);
                        } else {
                          _playOrPause();
                        }
                      } else {
                        _run(_adapter.toggleFullscreen());
                      }
                    },
                    onLongPressStart:
                        widget.enableLongPress ? (_) => _beginHold() : null,
                    onLongPressEnd:
                        widget.enableLongPress ? (_) => _endHold() : null,
                    onLongPressCancel: widget.enableLongPress ? _endHold : null,
                  ),
                  if (_adapter.buffering)
                    const IgnorePointer(
                      child: Center(child: CircularProgressIndicator()),
                    ),
                  if (_temporary.active)
                    Positioned(
                      top: 20 * scale,
                      left: 0,
                      right: 0,
                      child: const IgnorePointer(
                        child: Center(child: Chip(label: Text('2× 快进中'))),
                      ),
                    ),
                  Positioned(
                    left: 0,
                    right: 0,
                    bottom: 0,
                    child: IgnorePointer(
                      ignoring: !_visible,
                      child: ExcludeFocus(
                        excluding: !_visible,
                        child: ExcludeSemantics(
                          excluding: !_visible,
                          child: AnimatedOpacity(
                            opacity: _visible ? 1 : 0,
                            duration: const Duration(milliseconds: 150),
                            child: Container(
                              key: const ValueKey('player-controls-bar'),
                              padding: EdgeInsets.fromLTRB(
                                8,
                                0,
                                8,
                                MediaQuery.paddingOf(context).bottom,
                              ),
                              decoration: const BoxDecoration(
                                gradient: LinearGradient(
                                  begin: Alignment.topCenter,
                                  end: Alignment.bottomCenter,
                                  colors: [
                                    Colors.transparent,
                                    Color(0xd9000000),
                                  ],
                                ),
                              ),
                              child: Column(
                                mainAxisSize: MainAxisSize.min,
                                children: [
                                  SizedBox(
                                    height: 32 * scale,
                                    child: Row(
                                      children: [
                                        Text(
                                          _time(
                                            Duration(
                                              milliseconds: position.round(),
                                            ),
                                          ),
                                          style: TextStyle(
                                            color: Colors.white,
                                            fontSize: 11 * scale,
                                          ),
                                        ),
                                        Expanded(
                                          child: Slider(
                                            key: const ValueKey(
                                              'player-controls-seek',
                                            ),
                                            value: position,
                                            max: duration,
                                            secondaryTrackValue: _adapter
                                                .buffer
                                                .inMilliseconds
                                                .toDouble()
                                                .clamp(position, duration),
                                            label: _time(
                                              Duration(
                                                milliseconds: position.round(),
                                              ),
                                            ),
                                            semanticFormatterCallback:
                                                (value) =>
                                                    '播放位置 ${_time(Duration(milliseconds: value.round()))}',
                                            onChangeStart: (_) {
                                              _endHold();
                                              _scrubbing = true;
                                              _hideTimer?.cancel();
                                            },
                                            onChanged:
                                                _adapter.duration >
                                                        Duration.zero
                                                    ? (value) => setState(
                                                      () =>
                                                          _dragPosition = value,
                                                    )
                                                    : null,
                                            onChangeEnd: (value) {
                                              _scrubbing = false;
                                              _dragPosition = null;
                                              _run(
                                                _adapter.seek(
                                                  Duration(
                                                    milliseconds: value.round(),
                                                  ),
                                                ),
                                              );
                                              _show();
                                            },
                                          ),
                                        ),
                                        Text(
                                          _time(_adapter.duration),
                                          style: TextStyle(
                                            color: Colors.white,
                                            fontSize: 11 * scale,
                                          ),
                                        ),
                                      ],
                                    ),
                                  ),
                                  Row(
                                    children: [
                                      _button(
                                        'play',
                                        _adapter.playing
                                            ? Icons.pause_rounded
                                            : Icons.play_arrow_rounded,
                                        _playOrPause,
                                        scale,
                                      ),
                                      _button(
                                        'back',
                                        Icons.replay_10_rounded,
                                        () => _seekBy(-10),
                                        scale,
                                      ),
                                      _button(
                                        'forward',
                                        Icons.forward_10_rounded,
                                        () => _seekBy(10),
                                        scale,
                                      ),
                                      const Spacer(),
                                      if (bounds.maxWidth >= 540 * scale)
                                        Text(
                                          'FSR ${widget.effectiveQuality.label}  ·  ${_adapter.rate}×',
                                          style: TextStyle(
                                            color: Colors.white70,
                                            fontSize: 12 * scale,
                                          ),
                                        ),
                                      _button(
                                        'settings',
                                        Icons.tune_rounded,
                                        _togglePanel,
                                        scale,
                                        focusNode: _settingsFocus,
                                      ),
                                      _button(
                                        'fullscreen',
                                        _adapter.fullscreen
                                            ? Icons.fullscreen_exit_rounded
                                            : Icons.fullscreen_rounded,
                                        () {
                                          _endHold();
                                          _run(_adapter.toggleFullscreen());
                                        },
                                        scale,
                                      ),
                                    ],
                                  ),
                                ],
                              ),
                            ),
                          ),
                        ),
                      ),
                    ),
                  ),
                  if (_panelOpen)
                    Positioned(
                      right: 8,
                      bottom:
                          controlsHeight + MediaQuery.paddingOf(context).bottom,
                      width: math.min(bounds.maxWidth - 16, 330 * scale),
                      height: math.min(
                        338 * scale,
                        math.max(
                          40,
                          bounds.maxHeight -
                              controlsHeight -
                              MediaQuery.paddingOf(context).bottom -
                              12,
                        ),
                      ),
                      child: Theme(
                        data: Theme.of(context).copyWith(
                          brightness: Brightness.dark,
                          textTheme: Theme.of(context).textTheme.apply(
                            bodyColor: Colors.white,
                            displayColor: Colors.white,
                          ),
                          canvasColor: const Color(0xff202b30),
                        ),
                        child: _panel(scale),
                      ),
                    ),
                ],
              ),
            ),
          ),
        ),
      );
    },
  );
}
