// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

import 'backend.dart';
import 'playback_evidence.dart';

typedef Json = Map<String, dynamic>;

const background = Color(0xff101519);
const sidebar = Color(0xff0c1115);
const surface = Color(0xff182126);
const surface2 = Color(0xff202b30);
const border = Color(0xff2b393f);
const foreground = Color(0xffe9f1ef);
const muted = Color(0xff98abaa);
const accent = Color(0xff89ddbd);
const danger = Color(0xfff0aaa3);
const pageNames = ['搜索影片', '播放工作台', '收藏与记录', '来源与诊断'];
const pageIcons = [
  Icons.search_rounded,
  Icons.play_arrow_outlined,
  Icons.view_list_outlined,
  Icons.settings_outlined,
];

Json asJson(dynamic value) =>
    value is Map ? Map<String, dynamic>.from(value) : <String, dynamic>{};
List<Json> asRows(dynamic value) =>
    value is List
        ? value
            .whereType<Map>()
            .map((e) => Map<String, dynamic>.from(e))
            .toList()
        : <Json>[];
String valueText(dynamic value) => value?.toString() ?? '';
String cleanedSource(dynamic value) => valueText(value)
    .replaceAll(RegExp(r'（(?:未实播验证|版本及集数需核对|第三方)）'), '')
    .replaceAll(' / 定位平台：', ' · ');
String episodeName(dynamic value) => valueText(value).replaceAll('（集数未核验）', '');
String joined(Iterable<dynamic> values) =>
    values.map(valueText).where((e) => e.isNotEmpty).join(' · ');
String formattedTime(dynamic raw) {
  final parsed = DateTime.tryParse(valueText(raw))?.toLocal();
  if (parsed == null) return valueText(raw);
  String two(int n) => n.toString().padLeft(2, '0');
  return '${parsed.year}-${two(parsed.month)}-${two(parsed.day)} '
      '${two(parsed.hour)}:${two(parsed.minute)}';
}

/// All screens are Flutter widgets. Video is decoded and rendered by libmpv;
/// this client never creates a browser or WebView.
class YingXiaApp extends StatelessWidget {
  const YingXiaApp({super.key, this.backend});
  final BackendConnection? backend;

  @override
  Widget build(BuildContext context) => MaterialApp(
    title: '映匣 · YingXia',
    debugShowCheckedModeBanner: false,
    locale: const Locale('zh', 'CN'),
    supportedLocales: const [Locale('zh', 'CN'), Locale('en')],
    localizationsDelegates: GlobalMaterialLocalizations.delegates,
    theme: ThemeData(
      brightness: Brightness.dark,
      useMaterial3: true,
      scaffoldBackgroundColor: background,
      colorScheme: const ColorScheme.dark(
        primary: accent,
        secondary: accent,
        surface: surface,
        onSurface: foreground,
        onPrimary: Color(0xff13241e),
        error: danger,
      ),
      fontFamily: Platform.isWindows ? 'Microsoft YaHei UI' : null,
      textTheme: const TextTheme(
        bodyMedium: TextStyle(fontSize: 14, color: foreground, height: 1.55),
        bodySmall: TextStyle(fontSize: 12, color: muted, height: 1.55),
      ),
      dividerColor: border,
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: const Color(0xff151f24),
        isDense: true,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 12,
          vertical: 14,
        ),
        hintStyle: const TextStyle(color: Color(0xff7c9290)),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(7),
          borderSide: const BorderSide(color: border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(7),
          borderSide: const BorderSide(color: border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(7),
          borderSide: const BorderSide(color: accent),
        ),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          backgroundColor: accent,
          foregroundColor: const Color(0xff13241e),
          minimumSize: const Size(72, 42),
          padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 12),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
          textStyle: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          backgroundColor: surface2,
          foregroundColor: foreground,
          side: const BorderSide(color: Color(0xff3d4d53)),
          minimumSize: const Size(64, 40),
          padding: const EdgeInsets.symmetric(horizontal: 17, vertical: 10),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
          textStyle: const TextStyle(fontSize: 12),
        ),
      ),
      dialogTheme: DialogThemeData(
        backgroundColor: const Color(0xff192820),
        shape: RoundedRectangleBorder(
          side: const BorderSide(color: Color(0xff47614f)),
          borderRadius: BorderRadius.circular(16),
        ),
      ),
      snackBarTheme: const SnackBarThemeData(
        backgroundColor: Color(0xff294437),
        contentTextStyle: TextStyle(color: Color(0xffd7f3e5)),
        behavior: SnackBarBehavior.floating,
      ),
    ),
    home: YingXiaHome(backend: backend),
  );
}

class YingXiaHome extends StatefulWidget {
  const YingXiaHome({super.key, this.backend});
  final BackendConnection? backend;
  @override
  State<YingXiaHome> createState() => _YingXiaHomeState();
}

class _YingXiaHomeState extends State<YingXiaHome> with WidgetsBindingObserver {
  late final BackendConnection _backend = widget.backend ?? BackendConnection();
  final TextEditingController _query = TextEditingController();
  final TextEditingController _bindingUrl = TextEditingController();
  final GlobalKey<VideoState> _videoKey = GlobalKey<VideoState>();
  final List<StreamSubscription<dynamic>> _playerSubscriptions = [];
  Timer? _ping;
  Timer? _refresh;
  Json? _config;
  String? _startupError;
  bool _starting = false;
  bool _consentOpen = false;
  int _page = 0;
  String _searchMode = 'all';
  Json? _searchResult;
  bool _searching = false;
  String _searchError = '';
  String? _searchId;
  int _searchSequence = 0;
  int _detailSequence = 0;
  int _playSequence = 0;
  int _librarySequence = 0;
  Json? _film;
  Json? _group;
  Json? _episode;
  Json? _playback;
  String _workspaceMessage = '';
  String _playStatus = '待播放';
  String _playerSource = 'native';
  String _bindingPlatform = 'qq';
  String _bindingNote = '';
  bool _detailLoading = false;
  bool _playLoading = false;
  bool _suspended = false;
  bool _foreground = true;
  String? _playKey;
  Duration _resumePosition = Duration.zero;
  double _rate = 1;
  Player? _player;
  VideoController? _video;
  final PlaybackEvidenceClock _evidenceClock = PlaybackEvidenceClock();
  int _videoWidth = 0;
  List<Json> _favorites = [];
  List<Json> _history = [];
  Set<String> _favoriteIds = {};
  bool _libraryLoading = false;
  bool _clearingHistory = false;
  bool _updating = false;
  bool _diagnosing = false;
  List<Json>? _diagnostics;
  String _diagnosticError = '';
  Future<void> _settingsTail = Future.value();
  Future<void> _searchStartTail = Future.value();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    HardwareKeyboard.instance.addHandler(_handlePlaybackKey);
    _start();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    HardwareKeyboard.instance.removeHandler(_handlePlaybackKey);
    _searchSequence++;
    _detailSequence++;
    _playSequence++;
    _librarySequence++;
    _ping?.cancel();
    _refresh?.cancel();
    _query.dispose();
    _bindingUrl.dispose();
    for (final subscription in _playerSubscriptions) {
      unawaited(subscription.cancel());
    }
    unawaited(_player?.dispose() ?? Future<void>.value());
    unawaited(_backend.dispose());
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _foreground = state == AppLifecycleState.resumed;
    if (!_foreground) _pausePlayer();
  }

  bool _handlePlaybackKey(KeyEvent event) {
    if (event is KeyDownEvent &&
        _page == 1 &&
        _foreground &&
        _playback != null &&
        (event.logicalKey == LogicalKeyboardKey.space ||
            event.logicalKey == LogicalKeyboardKey.enter ||
            event.logicalKey == LogicalKeyboardKey.keyK)) {
      _suspended = false;
    }
    return false;
  }

  void _changed(VoidCallback action) {
    if (mounted) setState(action);
  }

  void _toast(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(
        SnackBar(content: Text(message), duration: const Duration(seconds: 4)),
      );
  }

  Future<void> _start() async {
    if (_starting) return;
    _changed(() {
      _starting = true;
      _startupError = null;
    });
    try {
      await _backend.start();
      final config = asJson(await _backend.get('state'));
      if (!mounted) return;
      _applyConfig(config);
      _ping?.cancel();
      _refresh?.cancel();
      _ping = Timer.periodic(const Duration(seconds: 20), (_) {
        unawaited(_backend.post('ping', {}).catchError((_) => null));
      });
      _refresh = Timer.periodic(const Duration(seconds: 60), (_) {
        unawaited(_refreshConfig(silent: true));
      });
    } catch (e) {
      _changed(() => _startupError = e.toString());
    } finally {
      _changed(() => _starting = false);
    }
  }

  void _applyConfig(Json config) {
    _changed(() {
      _config = config;
      _favoriteIds =
          (config['favorite_ids'] as List? ?? []).map(valueText).toSet();
      final options = _searchOptions;
      if (!options.containsKey(_searchMode)) _searchMode = 'all';
      if (!_playOptions.containsKey(_playerSource)) _playerSource = 'native';
      final platforms = asJson(config['platforms']);
      if (!platforms.containsKey(_bindingPlatform) && platforms.isNotEmpty) {
        _bindingPlatform = platforms.keys.first;
      }
    });
  }

  Future<void> _refreshConfig({bool silent = false}) async {
    try {
      _applyConfig(asJson(await _backend.get('state')));
    } catch (e) {
      if (!silent) _toast(e.toString());
    }
  }

  Json get _registry => asJson(_config?['source_registry']);
  List<Json> get _providers => asRows(_registry['providers']);
  List<Json> get _parsers => asRows(_config?['sources']);
  List<Json> get _episodes => asRows(_group?['episodes']);
  List<Json> get _groups => asRows(_film?['groups']);
  Map<String, String> get _searchOptions => {
    'all': '全部来源',
    'anime': '动漫',
    for (final p in _providers)
      if (p['enabled'] == true) 'provider:${p['id']}': valueText(p['name']),
    for (final entry in asJson(_config?['platforms']).entries)
      entry.key: valueText(asJson(entry.value)['name']),
  };
  Map<String, String> get _playOptions => {
    'native': '内置播放',
    for (final p in _parsers)
      if (p['enabled'] == true) valueText(p['id']): valueText(p['name']),
  };

  Future<void> _saveSettings(Json patch) {
    final task = _settingsTail.then((_) async {
      _applyConfig(asJson(await _backend.post('settings', patch)));
    });
    _settingsTail = task.catchError((_) {});
    return task;
  }

  Future<bool> _ensureNetwork() async {
    if (_config?['network_consent'] == true) return true;
    if (!mounted || _consentOpen) return false;
    _consentOpen = true;
    final accepted = await showDialog<bool>(
      context: context,
      builder:
          (context) => AlertDialog(
            title: const Text('允许联网？'),
            content: const Text('搜索和播放将连接所选来源。'),
            actions: [
              OutlinedButton(
                onPressed: () => Navigator.pop(context, false),
                child: const Text('取消'),
              ),
              FilledButton(
                onPressed: () => Navigator.pop(context, true),
                child: const Text('允许'),
              ),
            ],
          ),
    );
    _consentOpen = false;
    if (accepted != true) return false;
    try {
      await _saveSettings({'network_consent': true});
      return true;
    } catch (e) {
      _toast(e.toString());
      return false;
    }
  }

  void _navigate(int page) {
    if (_page == 1 && page != 1) _pausePlayer();
    _changed(() => _page = page);
    if (page == 2) unawaited(_loadLibrary());
    if (page == 3) unawaited(_refreshConfig());
  }

  Future<void> _search() async {
    final query = _query.text.trim();
    if (query.isEmpty) {
      _toast('请输入片名');
      return;
    }
    if (!await _ensureNetwork() || !mounted) return;
    final sequence = ++_searchSequence;
    final mode = _searchMode;
    _changed(() {
      _searching = true;
      _searchError = '';
      _searchResult = {'phase': 'searching', 'percent': 0, 'items': []};
    });
    try {
      final pending = _searchStartTail.then<Json?>((_) async {
        if (sequence != _searchSequence || !mounted) return null;
        final result = asJson(
          await _backend.post('search/start', {'query': query, 'mode': mode}),
        );
        if (sequence != _searchSequence || !mounted) {
          try {
            await _backend.post('search/cancel', {'id': result['id']});
          } catch (_) {}
          return null;
        }
        return result;
      });
      _searchStartTail = pending.then<void>((_) {}).catchError((_) {});
      final initial = await pending;
      if (initial == null || sequence != _searchSequence || !mounted) return;
      Json result = initial;
      _searchId = valueText(result['id']);
      while (mounted && sequence == _searchSequence) {
        _changed(() => _searchResult = result);
        final phase = valueText(result['phase']);
        if (phase == 'done') break;
        if (phase == 'cancelled' || phase == 'failed') {
          throw Exception(
            valueText(result['message']).isEmpty ? '搜索已取消' : result['message'],
          );
        }
        await Future<void>.delayed(const Duration(milliseconds: 400));
        if (!mounted || sequence != _searchSequence) return;
        result = asJson(
          await _backend.get(
            'search/status?id=${Uri.encodeComponent(_searchId!)}',
          ),
        );
      }
    } catch (e) {
      if (sequence == _searchSequence) {
        _changed(() {
          _searchError = e.toString();
          _searchResult = null;
        });
      }
    } finally {
      if (sequence == _searchSequence) {
        _changed(() {
          _searching = false;
          _searchId = null;
        });
      }
    }
  }

  void _cancelSearch() {
    final id = _searchId;
    ++_searchSequence;
    _changed(() {
      _searching = false;
      _searchId = null;
      _searchResult = null;
      _searchError = '';
    });
    if (id != null) {
      unawaited(
        _backend.post('search/cancel', {'id': id}).catchError((_) => null),
      );
    }
  }

  Future<void> _openFilm(Json item, {Json? history}) async {
    final sequence = ++_detailSequence;
    _navigate(1);
    _stopPlayer();
    _changed(() {
      _detailLoading = true;
      _workspaceMessage = '';
      _film = null;
      _group = null;
      _episode = null;
    });
    try {
      final id = valueText(history?['film_id'] ?? item['id']);
      final film = asJson(
        await _backend.get('detail?id=${Uri.encodeComponent(id)}'),
      );
      if (!mounted || sequence != _detailSequence) return;
      final groups = asRows(film['groups']);
      Json? group = groups.firstOrNull;
      Json? episode = asRows(group?['episodes']).firstOrNull;
      final availability = asJson(item['availability']);
      if (availability['group_key'] != null) {
        final preferred =
            groups
                .where((g) => g['key'] == availability['group_key'])
                .firstOrNull;
        final preferredEpisode =
            asRows(
              preferred?['episodes'],
            ).where((e) => e['key'] == availability['episode_key']).firstOrNull;
        if (preferred != null && preferredEpisode != null) {
          group = preferred;
          episode = preferredEpisode;
        }
      }
      _resumePosition = Duration.zero;
      if (history != null) {
        bool found = false;
        for (final g in groups) {
          final e =
              asRows(
                g['episodes'],
              ).where((e) => e['name'] == history['episode']).firstOrNull;
          if (e != null) {
            group = g;
            episode = e;
            found = true;
            final seconds = history['position'];
            if (history['mode'] == 'media' && seconds is num && seconds > 0) {
              _resumePosition = Duration(
                milliseconds: (seconds * 1000).round(),
              );
            }
            break;
          }
        }
        if (!found) episode = null;
      }
      _changed(() {
        _film = film;
        _group = group;
        _episode = episode;
        _playerSource = 'native';
        _bindingUrl.clear();
        _bindingNote = '';
        _workspaceMessage =
            groups.isEmpty
                ? '暂无选集'
                : history != null && episode == null
                ? '未找到原记录中的集数，请重新选择'
                : '';
      });
      await _loadBinding();
    } catch (e) {
      if (sequence == _detailSequence) {
        _changed(() => _workspaceMessage = e.toString());
      }
    } finally {
      if (sequence == _detailSequence) _changed(() => _detailLoading = false);
    }
  }

  Future<void> _selectGroup(String key) async {
    final old = _episode;
    _stopPlayer();
    _changed(() {
      _group = _groups.where((e) => e['key'] == key).firstOrNull;
      _episode = null;
      _resumePosition = Duration.zero;
      _workspaceMessage = '';
    });
    if (old != null && _group != null) {
      final selectedGroup = _group;
      final selectedFilm = _film;
      try {
        final matched = asJson(
          await _backend.post('match', {
            'film_id': _film!['id'],
            'group_key': key,
            'match_name': old['name'],
          }),
        );
        if (_group != selectedGroup || _film != selectedFilm) return;
        _changed(
          () =>
              _episode =
                  _episodes
                      .where((e) => e['key'] == matched['key'])
                      .firstOrNull,
        );
      } catch (e) {
        if (_group == selectedGroup && _film == selectedFilm) {
          _changed(() => _workspaceMessage = e.toString());
        }
      }
    }
    await _loadBinding();
  }

  Future<void> _loadBinding() async {
    if (_film == null || _episode == null) return;
    final id = valueText(_film!['id']);
    final episode = valueText(_episode!['name']);
    try {
      final rows = asRows(
        await _backend.get(
          'bindings?id=${Uri.encodeComponent(id)}&episode=${Uri.encodeComponent(episode)}',
        ),
      );
      if (_film?['id'] != id || _episode?['name'] != episode) return;
      final binding =
          rows.where((e) => e['platform'] == _bindingPlatform).firstOrNull ??
          rows.firstOrNull;
      _changed(() {
        _bindingUrl.text = valueText(binding?['url']);
        if (binding != null) _bindingPlatform = valueText(binding['platform']);
        _bindingNote = binding == null ? '' : '已绑定';
      });
    } catch (e) {
      _toast(e.toString());
    }
  }

  Future<void> _saveBinding() async {
    if (_film == null || _episode == null) {
      _toast('请选择影片和集数');
      return;
    }
    try {
      final result = asJson(
        await _backend.post('bind', {
          'film_id': _film!['id'],
          'episode': _episode!['name'],
          'platform': _bindingPlatform,
          'url': _bindingUrl.text.trim(),
        }),
      );
      _changed(() {
        _bindingUrl.text = valueText(result['url']);
        _bindingNote = '已绑定';
      });
      _toast('已保存');
    } catch (e) {
      _toast(e.toString());
    }
  }

  void _createPlayer() {
    if (_player != null) return;
    final player = Player(
      configuration: const PlayerConfiguration(title: '映匣 · YingXia'),
    );
    _player = player;
    _video = VideoController(player);
    _playerSubscriptions.addAll([
      player.stream.playing.listen((playing) {
        if (playing && (_suspended || _page != 1 || !_foreground)) {
          unawaited(player.pause());
          return;
        }
        if (_playback != null) {
          _changed(() => _playStatus = playing ? '缓冲中…' : '已暂停');
        }
      }),
      player.stream.buffering.listen((buffering) {
        if (_playback != null &&
            player.state.playing &&
            !_suspended &&
            _page == 1) {
          _changed(() => _playStatus = buffering ? '缓冲中…' : '播放中');
        }
      }),
      player.stream.width.listen((width) => _videoWidth = width ?? 0),
      player.stream.position.listen((position) {
        final advanced = _evidenceClock.advanced(position);
        if (_playback == null ||
            !player.state.playing ||
            player.state.buffering ||
            _suspended ||
            _page != 1 ||
            !_foreground ||
            _videoWidth <= 0 ||
            !advanced) {
          return;
        }
        _changed(() => _playStatus = '播放中');
        final history = valueText(_playback!['history_id']);
        if (history.isNotEmpty && _evidenceClock.shouldSave(position)) {
          unawaited(
            _backend
                .post('progress', {
                  'history_id': history,
                  'evidence': 'browser_time_advanced',
                  'position': position.inMilliseconds / 1000,
                })
                .catchError((_) => null),
          );
        }
      }),
      player.stream.completed.listen((completed) {
        if (!completed || _playback == null) return;
        _changed(() => _playStatus = '播放结束');
        if (!_suspended &&
            _foreground &&
            _page == 1 &&
            _config?['auto_next'] == true) {
          unawaited(_stepEpisode(1));
        }
      }),
      player.stream.error.listen((error) {
        if (_playback == null) return;
        _changed(() {
          _playKey = null;
          _playStatus = '播放失败';
          _workspaceMessage = '加载失败，请切换来源';
        });
      }),
    ]);
  }

  void _pausePlayer() {
    ++_playSequence;
    _changed(() {
      _suspended = true;
      _playLoading = false;
      if (_playback != null) _playStatus = '已暂停';
    });
    unawaited(_player?.pause() ?? Future<void>.value());
  }

  void _stopPlayer() {
    ++_playSequence;
    _changed(() {
      _playback = null;
      _playKey = null;
      _playLoading = false;
      _playStatus = '已停止';
      _workspaceMessage = '';
    });
    unawaited(_player?.stop() ?? Future<void>.value());
  }

  Future<void> _play() async {
    if (_film == null ||
        _episode == null ||
        _group == null ||
        _page != 1 ||
        !_foreground) {
      return;
    }
    if (_film!['demo'] != true && !await _ensureNetwork()) return;
    if (_page != 1 || !_foreground || !mounted) return;
    final request = <String, dynamic>{
      'film_id': _film!['id'],
      'group_key': _group!['key'],
      'episode_key': _episode!['key'],
      'source': _playerSource,
      'platform_url': _bindingUrl.text.trim(),
      'platform': _bindingUrl.text.trim().isEmpty ? '' : _bindingPlatform,
    };
    final key = jsonEncode(request);
    _suspended = false;
    if (_player != null &&
        _playback?['kind'] == 'media' &&
        _playKey == key &&
        !_player!.state.completed) {
      await _player!.play();
      return;
    }
    final sequence = ++_playSequence;
    _changed(() {
      _playLoading = true;
      _workspaceMessage = '';
      _playStatus = '加载中…';
    });
    try {
      final playback = asJson(await _backend.post('resolve', request));
      if (!mounted ||
          sequence != _playSequence ||
          _suspended ||
          _page != 1 ||
          !_foreground) {
        return;
      }
      if (playback['kind'] != 'media') {
        _changed(() {
          _playback = null;
          _playKey = null;
          _playStatus = '未播放';
          _workspaceMessage = '此来源提供网页播放，请切换来源';
        });
        return;
      }
      _createPlayer();
      await _player!.stop();
      if (!mounted ||
          sequence != _playSequence ||
          _suspended ||
          _page != 1 ||
          !_foreground) {
        return;
      }
      _changed(() {
        _playback = playback;
        _playKey = key;
        _evidenceClock.reset();
        _videoWidth = 0;
      });
      final url =
          Uri.parse(
            _backend.origin,
          ).resolve(valueText(playback['url'])).toString();
      await _player!.open(
        Media(url, httpHeaders: _backend.mediaHeaders),
        play: false,
      );
      if (!mounted ||
          sequence != _playSequence ||
          _suspended ||
          _page != 1 ||
          !_foreground) {
        await _player!.pause();
        return;
      }
      await _player!.setRate(_rate);
      if (_resumePosition > Duration.zero) {
        await _player!.seek(_resumePosition);
        _resumePosition = Duration.zero;
      }
      if (sequence == _playSequence &&
          !_suspended &&
          _page == 1 &&
          _foreground) {
        await _player!.play();
      }
    } catch (e) {
      if (sequence == _playSequence) {
        _changed(() {
          _playKey = null;
          _playStatus = '播放失败';
          _workspaceMessage = e.toString();
        });
      }
    } finally {
      if (sequence == _playSequence) _changed(() => _playLoading = false);
    }
  }

  Future<void> _stepEpisode(int delta) async {
    final episodes = _episodes;
    final index = episodes.indexWhere((e) => e['key'] == _episode?['key']);
    if (index < 0 || index + delta < 0 || index + delta >= episodes.length) {
      return;
    }
    _pausePlayer();
    final sequence = _playSequence;
    _changed(() {
      _episode = episodes[index + delta];
      _resumePosition = Duration.zero;
    });
    await _loadBinding();
    if (sequence != _playSequence || !mounted) return;
    await _play();
  }

  Future<void> _toggleFavorite() async {
    if (_film == null) return;
    final id = valueText(_film!['id']);
    try {
      final result = asJson(await _backend.post('favorite', {'film_id': id}));
      _changed(() {
        if (result['saved'] == true) {
          _favoriteIds.add(id);
        } else {
          _favoriteIds.remove(id);
        }
      });
      _toast(result['saved'] == true ? '已收藏' : '已取消收藏');
    } catch (e) {
      _toast(e.toString());
    }
  }

  Future<void> _loadLibrary({bool throwOnError = false}) async {
    final sequence = ++_librarySequence;
    _changed(() => _libraryLoading = true);
    try {
      final result = asJson(await _backend.get('library'));
      if (sequence != _librarySequence || !mounted) return;
      _changed(() {
        _favorites = asRows(result['favorites']);
        _history = asRows(result['history']);
        _favoriteIds = _favorites.map((e) => valueText(e['id'])).toSet();
      });
    } catch (e) {
      if (sequence == _librarySequence) {
        if (throwOnError) rethrow;
        _toast(e.toString());
      }
    } finally {
      if (sequence == _librarySequence) _changed(() => _libraryLoading = false);
    }
  }

  Future<void> _clearHistory() async {
    if (_clearingHistory) return;
    final accepted = await showDialog<bool>(
      context: context,
      builder:
          (context) => AlertDialog(
            title: const Text('清空历史记录？'),
            actions: [
              OutlinedButton(
                onPressed: () => Navigator.pop(context, false),
                child: const Text('取消'),
              ),
              FilledButton(
                key: const Key('confirm-clear-history'),
                onPressed: () => Navigator.pop(context, true),
                child: const Text('清空'),
              ),
            ],
          ),
    );
    if (accepted != true || !mounted) return;
    ++_librarySequence;
    _changed(() => _clearingHistory = true);
    try {
      await _backend.post('clear-history', {});
      // Clear visible rows only after the persisted write succeeds, then read
      // the service again. Cancellation and failed writes retain existing rows.
      _changed(() => _history = []);
      await _loadLibrary(throwOnError: true);
      _toast('记录已清空');
    } catch (e) {
      _toast(e.toString());
    } finally {
      _changed(() => _clearingHistory = false);
    }
  }

  Future<void> _updateSources() async {
    if (_updating || !await _ensureNetwork()) return;
    _changed(() => _updating = true);
    try {
      await _backend.post('sources/update', {});
      await _refreshConfig();
      _toast('检查完成');
    } catch (e) {
      _toast(e.toString());
    } finally {
      _changed(() => _updating = false);
    }
  }

  Future<void> _diagnose() async {
    if (_diagnosing || !await _ensureNetwork()) return;
    _changed(() {
      _diagnosing = true;
      _diagnosticError = '';
    });
    try {
      final result = asJson(await _backend.post('diagnose', {}));
      _changed(() => _diagnostics = asRows(result['results']));
    } catch (e) {
      _changed(() => _diagnosticError = e.toString());
    } finally {
      _changed(() => _diagnosing = false);
    }
  }

  Future<void> _exportReport() async {
    try {
      final result = await _backend.get('report');
      final directory = valueText(_config?['data_dir']);
      if (directory.isEmpty) throw Exception('未找到数据目录');
      final file = File(
        '$directory${Platform.pathSeparator}yingxia_diagnostics.json',
      );
      await file.writeAsString(
        const JsonEncoder.withIndent('  ').convert(result),
      );
      _toast('已保存：${file.path}');
    } catch (e) {
      _toast(e.toString());
    }
  }

  Future<void> _addSource() async {
    final result = await showDialog<Json>(
      context: context,
      builder: (_) => const _SourceDialog(),
    );
    if (result == null) return;
    try {
      await _backend.post('sources/add', result);
      await _refreshConfig();
      _toast('已添加');
    } catch (e) {
      _toast(e.toString());
    }
  }

  Future<void> _toggleProvider(Json provider, bool enabled) async {
    try {
      final task = _settingsTail.then((_) async {
        final settings = {
          for (final p in _providers) valueText(p['id']): p['enabled'] == true,
        };
        settings[valueText(provider['id'])] = enabled;
        _applyConfig(
          asJson(
            await _backend.post('settings', {'enabled_providers': settings}),
          ),
        );
      });
      _settingsTail = task.catchError((_) {});
      await task;
    } catch (e) {
      _toast(e.toString());
    }
  }

  Future<void> _toggleParser(Json parser, bool enabled) async {
    try {
      final task = _settingsTail.then((_) async {
        final settings = {
          for (final p in _parsers) valueText(p['id']): p['enabled'] == true,
        };
        settings[valueText(parser['id'])] = enabled;
        _applyConfig(
          asJson(
            await _backend.post('settings', {'enabled_sources': settings}),
          ),
        );
      });
      _settingsTail = task.catchError((_) {});
      await task;
    } catch (e) {
      _toast(e.toString());
    }
  }

  Future<void> _removeSource(Json source) async {
    try {
      await _backend.post('sources/remove', {'id': source['id']});
      await _refreshConfig();
    } catch (e) {
      _toast(e.toString());
    }
  }

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final desktop = constraints.maxWidth >= 800;
      return PopScope(
        canPop: _page == 0,
        onPopInvokedWithResult: (didPop, result) {
          if (!didPop) _navigate(0);
        },
        child: Scaffold(
          body: SafeArea(
            child: Row(
              children: [
                if (desktop) _sidebar(constraints.maxWidth > 1100 ? 204 : 170),
                Expanded(
                  child: Column(
                    children: [
                      _topbar(desktop),
                      Expanded(
                        child: _config == null ? _startup() : _content(desktop),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
          bottomNavigationBar:
              desktop
                  ? null
                  : NavigationBar(
                    height: 68,
                    backgroundColor: sidebar,
                    indicatorColor: const Color(0xff1d342c),
                    selectedIndex: _page,
                    onDestinationSelected: _navigate,
                    destinations: [
                      for (var i = 0; i < pageNames.length; i++)
                        NavigationDestination(
                          icon: Icon(pageIcons[i], color: muted),
                          selectedIcon: Icon(pageIcons[i], color: accent),
                          label: ['搜索', '播放', '收藏', '来源'][i],
                        ),
                    ],
                  ),
        ),
      );
    },
  );

  Widget _sidebar(double width) => Container(
    width: width,
    decoration: const BoxDecoration(
      color: sidebar,
      border: Border(right: BorderSide(color: border)),
    ),
    padding: const EdgeInsets.fromLTRB(18, 33, 18, 24),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        InkWell(
          onTap: () => _navigate(0),
          child: const Padding(
            padding: EdgeInsets.symmetric(horizontal: 10),
            child: _Brand(),
          ),
        ),
        const SizedBox(height: 47),
        for (var i = 0; i < pageNames.length; i++)
          Padding(
            padding: const EdgeInsets.only(bottom: 7),
            child: Material(
              color: _page == i ? const Color(0xff1d342c) : Colors.transparent,
              borderRadius: BorderRadius.circular(10),
              child: Semantics(
                label: pageNames[i],
                button: true,
                selected: _page == i,
                excludeSemantics: true,
                onTap: () => _navigate(i),
                child: InkWell(
                  key: Key('nav-$i'),
                  borderRadius: BorderRadius.circular(10),
                  onTap: () => _navigate(i),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 13,
                      vertical: 13,
                    ),
                    child: Row(
                      children: [
                        Icon(
                          pageIcons[i],
                          size: 23,
                          color: _page == i ? accent : muted,
                        ),
                        const SizedBox(width: 12),
                        Flexible(
                          child: Text(
                            pageNames[i],
                            style: TextStyle(
                              fontSize: 13,
                              color: _page == i ? accent : muted,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
          ),
      ],
    ),
  );

  Widget _topbar(bool desktop) => Container(
    height: desktop ? 73 : 58,
    decoration: const BoxDecoration(
      border: Border(bottom: BorderSide(color: border)),
    ),
    padding: EdgeInsets.symmetric(horizontal: desktop ? 38 : 18),
    child: Row(
      children: [
        if (!desktop) ...[
          const Text(
            '映匣',
            style: TextStyle(fontSize: 19, fontWeight: FontWeight.w600),
          ),
          const SizedBox(width: 15),
        ],
        Text(
          pageNames[_page],
          style: const TextStyle(fontSize: 12, color: muted),
        ),
      ],
    ),
  );

  Widget _startup() => Center(
    child: Padding(
      padding: const EdgeInsets.all(30),
      child:
          _startupError == null
              ? const CircularProgressIndicator(strokeWidth: 2)
              : Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(_startupError!, textAlign: TextAlign.center),
                  const SizedBox(height: 18),
                  FilledButton(
                    onPressed: _starting ? null : _start,
                    child: const Text('重试'),
                  ),
                ],
              ),
    ),
  );

  Widget _content(bool desktop) => SingleChildScrollView(
    key: ValueKey('page-$_page'),
    padding: EdgeInsets.fromLTRB(
      desktop ? 38 : 16,
      desktop ? 34 : 24,
      desktop ? 38 : 16,
      36,
    ),
    child: Align(
      alignment: Alignment.topCenter,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 1524),
        child: switch (_page) {
          0 => _searchPage(desktop),
          1 => _workspacePage(desktop),
          2 => _libraryPage(desktop),
          _ => _settingsPage(desktop),
        },
      ),
    ),
  );

  Widget _searchPage(bool desktop) {
    final result = _searchResult;
    final items = asRows(result?['items']);
    final phase = valueText(result?['phase']);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 16, bottom: 30),
          child: Text(
            '想看什么？',
            style: TextStyle(
              fontSize: desktop ? 40 : 32,
              fontWeight: FontWeight.w600,
              letterSpacing: 1,
            ),
          ),
        ),
        Container(
          padding: const EdgeInsets.all(10),
          decoration: BoxDecoration(
            color: const Color(0xff1b252a),
            border: Border.all(color: const Color(0xff3a504c)),
            borderRadius: BorderRadius.circular(12),
          ),
          child: LayoutBuilder(
            builder: (_, constraints) {
              final narrow = constraints.maxWidth < 560;
              final query = Semantics(
                label: '搜索片名',
                child: TextField(
                  key: const Key('search-query'),
                  controller: _query,
                  maxLength: 120,
                  textInputAction: TextInputAction.search,
                  onSubmitted: (_) {
                    if (!_searching) _search();
                  },
                  style: const TextStyle(fontSize: 15),
                  decoration: const InputDecoration(
                    hintText: '搜索片名…',
                    prefixIcon: Icon(
                      Icons.search_rounded,
                      color: accent,
                      size: 25,
                    ),
                    counterText: '',
                    filled: false,
                    border: InputBorder.none,
                    enabledBorder: InputBorder.none,
                    focusedBorder: InputBorder.none,
                  ),
                ),
              );
              final mode = Semantics(
                label: '搜索来源',
                child: _dropdown(
                  _searchMode,
                  _searchOptions,
                  (value) => _changed(() => _searchMode = value),
                ),
              );
              final button = FilledButton(
                key: const Key('search-submit'),
                onPressed: _searching ? null : _search,
                child: Text(_searching ? '搜索中…' : '搜索'),
              );
              if (narrow) {
                return Column(
                  children: [
                    query,
                    const SizedBox(height: 8),
                    Row(
                      children: [
                        Expanded(child: mode),
                        const SizedBox(width: 10),
                        button,
                      ],
                    ),
                  ],
                );
              }
              return Row(
                children: [
                  Expanded(child: query),
                  const SizedBox(width: 12),
                  SizedBox(width: 175, child: mode),
                  const SizedBox(width: 10),
                  button,
                ],
              );
            },
          ),
        ),
        if (_searchError.isNotEmpty) _notice(_searchError),
        if (result != null)
          Padding(
            padding: const EdgeInsets.only(top: 16),
            child: Column(
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        _progressLabel(result),
                        style: const TextStyle(fontSize: 12, color: muted),
                      ),
                    ),
                    if (_searching)
                      TextButton(
                        onPressed: _cancelSearch,
                        child: const Text('取消'),
                      ),
                  ],
                ),
                const SizedBox(height: 8),
                ClipRRect(
                  borderRadius: BorderRadius.circular(4),
                  child: LinearProgressIndicator(
                    value:
                        ((result['percent'] as num? ?? 0) / 100)
                            .clamp(0, 1)
                            .toDouble(),
                    minHeight: 5,
                    color: accent,
                    backgroundColor: surface2,
                  ),
                ),
              ],
            ),
          ),
        if (items.isNotEmpty || phase == 'done') ...[
          _sectionHead(
            '搜索结果',
            Text(
              '${items.length} 个结果',
              style: const TextStyle(fontSize: 12, color: muted),
            ),
          ),
          if (asRows(result?['source_results']).isNotEmpty)
            _sourceSearchResults(result!),
          if (items.isEmpty)
            _empty(
              (result?['successful_sources'] as num? ?? 0) == 0
                  ? '来源暂不可用'
                  : '暂无结果',
            )
          else
            _filmGrid(items, desktop),
        ],
      ],
    );
  }

  String _progressLabel(Json result) {
    if (result['phase'] == 'checking') {
      return '检测访问 · ${result['checked_count'] ?? 0} / ${result['check_total'] ?? 0} 个结果 · ${result['available_count'] ?? 0} 个可访问';
    }
    if (result['phase'] == 'done') {
      return '检测完成 · ${result['available_count'] ?? 0} 个可访问';
    }
    final total = result['source_total'] as num? ?? 0;
    return total > 0
        ? '正在搜索 · ${result['source_completed'] ?? 0} / $total 个来源'
        : '正在搜索…';
  }

  Widget _sourceSearchResults(Json result) => Padding(
    padding: const EdgeInsets.only(bottom: 15),
    child: _expansion(
      '${result['successful_sources'] ?? 0} / ${result['source_count'] ?? 0} 个来源已响应',
      _table(
        ['来源', '结果'],
        asRows(result['source_results'])
            .map(
              (row) => [
                Text(valueText(row['name'])),
                Text(
                  row['ok'] == true
                      ? '${row['count']} 个结果'
                      : valueText(row['error']),
                ),
              ],
            )
            .toList(),
      ),
    ),
  );

  Widget _filmGrid(List<Json> items, bool desktop) => LayoutBuilder(
    builder: (_, constraints) {
      final count =
          desktop ? (constraints.maxWidth / 218).floor().clamp(2, 6) : 2;
      final width = (constraints.maxWidth - (count - 1) * 18) / count;
      return Wrap(
        spacing: 18,
        runSpacing: 18,
        children: [
          for (var i = 0; i < items.length; i++)
            SizedBox(
              width: width,
              child: _FilmCard(
                film: items[i],
                index: i,
                onOpen: () => _openFilm(items[i]),
              ),
            ),
        ],
      );
    },
  );

  Widget _workspacePage(bool desktop) {
    final saved = _favoriteIds.contains(valueText(_film?['id']));
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    valueText(_film?['title']).isEmpty
                        ? '播放工作台'
                        : valueText(_film!['title']),
                    style: const TextStyle(
                      fontSize: 25,
                      fontWeight: FontWeight.w600,
                      height: 1.5,
                    ),
                  ),
                  if (_film != null)
                    Text(
                      joined([
                        _film!['year'],
                        _film!['kind'],
                        _film!['area'],
                        _film!['remarks'],
                        cleanedSource(_film!['source']),
                      ]),
                      style: const TextStyle(fontSize: 12, color: muted),
                    ),
                ],
              ),
            ),
            const SizedBox(width: 12),
            OutlinedButton(
              onPressed: _film == null ? null : _toggleFavorite,
              child: Text(saved ? '✓ 已收藏' : '＋ 收藏'),
            ),
          ],
        ),
        const SizedBox(height: 26),
        if (_detailLoading)
          const Padding(
            padding: EdgeInsets.only(bottom: 15),
            child: LinearProgressIndicator(minHeight: 2),
          ),
        LayoutBuilder(
          builder: (_, constraints) {
            final wide = constraints.maxWidth >= 710;
            if (wide) {
              return Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(child: _playerColumn()),
                  const SizedBox(width: 23),
                  SizedBox(
                    width: constraints.maxWidth >= 900 ? 270 : 220,
                    child: _episodePanel(),
                  ),
                ],
              );
            }
            return Column(
              children: [
                _playerColumn(),
                const SizedBox(height: 20),
                _episodePanel(),
              ],
            );
          },
        ),
      ],
    );
  }

  Widget _playerColumn() {
    final episodes = _episodes;
    final index = episodes.indexWhere((e) => e['key'] == _episode?['key']);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        AspectRatio(
          aspectRatio: 16 / 9,
          child: Container(
            decoration: BoxDecoration(
              color: const Color(0xff060a0c),
              border: Border.all(color: border),
              borderRadius: BorderRadius.circular(12),
            ),
            clipBehavior: Clip.antiAlias,
            child:
                _playback?['kind'] == 'media' && _video != null
                    ? Listener(
                      onPointerDown: (_) {
                        if (_page == 1 && _foreground) _suspended = false;
                      },
                      child: Focus(
                        onKeyEvent: (_, event) {
                          if (_page == 1 && _foreground) _suspended = false;
                          return KeyEventResult.ignored;
                        },
                        child: Video(
                          key: _videoKey,
                          controller: _video!,
                          pauseUponEnteringBackgroundMode: true,
                        ),
                      ),
                    )
                    : Center(
                      child: Padding(
                        padding: const EdgeInsets.all(22),
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Container(
                              width: 64,
                              height: 64,
                              decoration: BoxDecoration(
                                shape: BoxShape.circle,
                                border: Border.all(
                                  color: const Color(0xff416456),
                                ),
                              ),
                              child: const Icon(
                                Icons.play_arrow_outlined,
                                size: 38,
                                color: Color(0xff699b88),
                              ),
                            ),
                            const SizedBox(height: 22),
                            const Text(
                              '选集之后请点击下方播放所选集',
                              textAlign: TextAlign.center,
                              style: TextStyle(
                                fontSize: 14,
                                color: Color(0xffaec7bd),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 11),
          child: Row(
            children: [
              Container(
                width: 6,
                height: 6,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color:
                      _playStatus == '播放中' ? accent : const Color(0xff64776e),
                ),
              ),
              const SizedBox(width: 8),
              Text(
                _playStatus,
                style: const TextStyle(fontSize: 11, color: muted),
              ),
              const Spacer(),
              TextButton(
                onPressed:
                    _videoKey.currentState == null
                        ? null
                        : () => _videoKey.currentState?.toggleFullscreen(),
                child: const Text('全屏 ⛶', style: TextStyle(fontSize: 12)),
              ),
            ],
          ),
        ),
        Container(
          padding: const EdgeInsets.only(bottom: 21),
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: border)),
          ),
          child: Wrap(
            spacing: 8,
            runSpacing: 8,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              OutlinedButton(
                onPressed: index <= 0 ? null : () => _stepEpisode(-1),
                child: const Text('上一集'),
              ),
              FilledButton(
                key: const Key('play-selected'),
                onPressed: _episode == null || _playLoading ? null : _play,
                child: Text(_playLoading ? '加载中…' : '▷ 播放所选集'),
              ),
              OutlinedButton(
                onPressed:
                    index < 0 || index >= episodes.length - 1
                        ? null
                        : () => _stepEpisode(1),
                child: const Text('下一集'),
              ),
              if (_playback != null)
                SizedBox(
                  width: 92,
                  child: _dropdown(
                    _rate.toString(),
                    {
                      '0.5': '0.5×',
                      '1.0': '1×',
                      '1.25': '1.25×',
                      '1.5': '1.5×',
                      '2.0': '2×',
                    },
                    (value) {
                      _changed(() => _rate = double.parse(value));
                      unawaited(
                        _player?.setRate(_rate) ?? Future<void>.value(),
                      );
                    },
                  ),
                ),
              OutlinedButton(onPressed: _stopPlayer, child: const Text('停止')),
            ],
          ),
        ),
        const SizedBox(height: 20),
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    '播放方式 / 解析线路',
                    style: TextStyle(fontSize: 12, color: muted),
                  ),
                  const SizedBox(height: 7),
                  _dropdown(_playerSource, _playOptions, (value) {
                    _stopPlayer();
                    _changed(() => _playerSource = value);
                  }),
                ],
              ),
            ),
            const SizedBox(width: 20),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    '实际播放来源',
                    style: TextStyle(fontSize: 12, color: muted),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    _playback == null
                        ? '尚未选择'
                        : cleanedSource(_playback!['source_label']),
                    style: const TextStyle(
                      fontSize: 12,
                      color: Color(0xffc0d7cb),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
        const SizedBox(height: 14),
        _bindingPanel(),
        if (_workspaceMessage.isNotEmpty) _notice(_workspaceMessage),
      ],
    );
  }

  Widget _bindingPanel() => _expansion(
    '绑定播放页',
    Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        LayoutBuilder(
          builder: (_, constraints) {
            final select = _dropdown(
              _bindingPlatform,
              {
                for (final entry in asJson(_config?['platforms']).entries)
                  entry.key: valueText(asJson(entry.value)['name']),
              },
              (value) => _changed(() {
                _bindingPlatform = value;
                _bindingUrl.clear();
                _bindingNote = '';
              }),
            );
            final input = TextField(
              controller: _bindingUrl,
              decoration: const InputDecoration(hintText: '粘贴本集播放页'),
              style: const TextStyle(fontSize: 12),
            );
            final save = OutlinedButton(
              onPressed: _saveBinding,
              child: const Text('保存'),
            );
            if (constraints.maxWidth < 550) {
              return Column(
                children: [
                  select,
                  const SizedBox(height: 10),
                  input,
                  const SizedBox(height: 10),
                  Align(alignment: Alignment.centerRight, child: save),
                ],
              );
            }
            return Row(
              children: [
                SizedBox(width: 120, child: select),
                const SizedBox(width: 8),
                Expanded(child: input),
                const SizedBox(width: 8),
                save,
              ],
            );
          },
        ),
        if (_bindingNote.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 7),
            child: Text(
              _bindingNote,
              style: const TextStyle(fontSize: 12, color: muted),
            ),
          ),
      ],
    ),
  );

  Widget _episodePanel() => _panel(
    Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            const Text(
              '选集',
              style: TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
            ),
            const Spacer(),
            _chip('${_episodes.length} 集'),
          ],
        ),
        const SizedBox(height: 17),
        const Text('片源', style: TextStyle(fontSize: 12, color: muted)),
        const SizedBox(height: 7),
        _dropdown(
          valueText(_group?['key']),
          {
            for (final group in _groups)
              valueText(group['key']):
                  '${group['name']}（${asRows(group['episodes']).length} 项）',
          },
          _selectGroup,
          placeholder: '暂无来源',
        ),
        const SizedBox(height: 12),
        Text(
          _episode == null ? '请选择集数' : '已选：${episodeName(_episode!['name'])}',
          style: const TextStyle(fontSize: 12, color: muted),
        ),
        const SizedBox(height: 15),
        ConstrainedBox(
          constraints: const BoxConstraints(maxHeight: 290),
          child: SingleChildScrollView(
            child: LayoutBuilder(
              builder: (_, constraints) {
                final columns =
                    constraints.maxWidth > 450
                        ? 6
                        : constraints.maxWidth > 260
                        ? 4
                        : 3;
                final width =
                    (constraints.maxWidth - (columns - 1) * 7) / columns;
                return Wrap(
                  spacing: 7,
                  runSpacing: 7,
                  children: [
                    for (final episode in _episodes)
                      SizedBox(
                        width: width,
                        child: _EpisodeButton(
                          name: episodeName(episode['name']),
                          selected: _episode?['key'] == episode['key'],
                          onPressed: () {
                            _pausePlayer();
                            _changed(() {
                              _episode = episode;
                              _resumePosition = Duration.zero;
                            });
                            _loadBinding();
                          },
                        ),
                      ),
                  ],
                );
              },
            ),
          ),
        ),
        const SizedBox(height: 21),
        const Divider(),
        const SizedBox(height: 16),
        const Text(
          '作品信息',
          style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 9),
        ConstrainedBox(
          constraints: const BoxConstraints(maxHeight: 230),
          child: SingleChildScrollView(
            child: Text(
              valueText(_film?['summary']),
              style: const TextStyle(
                fontSize: 11,
                color: Color(0xff96b49f),
                height: 1.9,
              ),
            ),
          ),
        ),
        if (joined([_film?['director'], _film?['actors']]).isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              [
                if (valueText(_film?['director']).isNotEmpty)
                  '导演：${_film!['director']}',
                if (valueText(_film?['actors']).isNotEmpty)
                  '演员：${_film!['actors']}',
              ].join(' / '),
              style: const TextStyle(fontSize: 11, color: muted),
            ),
          ),
      ],
    ),
    color: const Color(0xff17221f),
    padding: 20,
  );

  Widget _libraryPage(bool desktop) => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      _pageTitle('收藏与记录', desktop),
      _sectionHead(
        '我的收藏',
        Text(
          '${_favorites.length} 个收藏',
          style: const TextStyle(fontSize: 12, color: muted),
        ),
      ),
      if (_libraryLoading && _favorites.isEmpty && _history.isEmpty)
        const LinearProgressIndicator(minHeight: 2),
      if (_favorites.isEmpty)
        _empty('暂无收藏')
      else
        _filmGrid(_favorites, desktop),
      _sectionHead(
        '最近打开',
        OutlinedButton(
          key: const Key('clear-history'),
          onPressed: _clearingHistory ? null : _clearHistory,
          child: Text(_clearingHistory ? '清空中…' : '清空记录'),
        ),
        top: 42,
      ),
      if (_history.isEmpty)
        _empty('暂无记录', key: const Key('empty-history'))
      else
        _table(
          ['作品 / 集数', '实际来源', '状态 / 时间', '操作'],
          _history.map((entry) {
            const evidence = {
              'opened': '已打开',
              'browser_time_advanced': '已播放',
              'native_time_advanced': '已播放',
              'user_confirmed': '手动确认',
            };
            return <Widget>[
              SizedBox(
                width: 240,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      valueText(entry['title']),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                    Text(
                      episodeName(entry['episode']),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 11, color: muted),
                    ),
                  ],
                ),
              ),
              SizedBox(
                width: 250,
                child: Text(
                  cleanedSource(entry['source']),
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    evidence[valueText(entry['evidence'])] ??
                        valueText(entry['evidence']),
                  ),
                  Text(
                    formattedTime(entry['updated_at']),
                    style: const TextStyle(fontSize: 11, color: muted),
                  ),
                ],
              ),
              OutlinedButton(
                onPressed: () => _openFilm({}, history: entry),
                child: const Text('重新打开'),
              ),
            ];
          }).toList(),
        ),
    ],
  );

  Widget _settingsPage(bool desktop) => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      _pageTitle('来源与诊断', desktop),
      LayoutBuilder(
        builder: (_, constraints) {
          final cards = [
            _panel(
              Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const Text(
                    '设置',
                    style: TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 14),
                  _toggle('联网搜索与播放', _config?['network_consent'] == true, (
                    enabled,
                  ) async {
                    if (enabled) {
                      await _ensureNetwork();
                    } else {
                      _stopPlayer();
                      await _saveSettingSafely({'network_consent': false});
                    }
                  }),
                  _toggle(
                    '保存历史记录',
                    _config?['save_history'] == true,
                    (value) => _saveSettingSafely({'save_history': value}),
                  ),
                  _toggle(
                    '自动播放下一集',
                    _config?['auto_next'] == true,
                    (value) => _saveSettingSafely({'auto_next': value}),
                  ),
                ],
              ),
            ),
            _panel(
              Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const Text(
                    '数据目录',
                    style: TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 17),
                  Container(
                    padding: const EdgeInsets.all(11),
                    color: const Color(0xff0e1913),
                    child: SelectableText(
                      valueText(_config?['data_dir']),
                      style: const TextStyle(
                        fontSize: 11,
                        color: Color(0xffb5cdba),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  Align(
                    alignment: Alignment.centerLeft,
                    child: OutlinedButton(
                      onPressed: _exportReport,
                      child: const Text('导出诊断'),
                    ),
                  ),
                ],
              ),
            ),
          ];
          if (constraints.maxWidth < 650) {
            return Column(
              children: [cards[0], const SizedBox(height: 18), cards[1]],
            );
          }
          return Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(child: cards[0]),
              const SizedBox(width: 18),
              Expanded(child: cards[1]),
            ],
          );
        },
      ),
      _sectionHead(
        '来源更新',
        OutlinedButton(
          onPressed: _updating ? null : _updateSources,
          child: Text(_updating ? '检查中…' : '检查更新'),
        ),
      ),
      _panel(
        Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _toggle(
              '自动更新来源',
              _registry['auto_update'] == true,
              (value) => _saveSettingSafely({'auto_update_sources': value}),
            ),
            Text(
              valueText(_registry['last_checked_at']).isEmpty
                  ? '尚未检查'
                  : '上次检查：${formattedTime(_registry['last_checked_at'])}',
              style: const TextStyle(fontSize: 12, color: muted),
            ),
            const SizedBox(height: 10),
            for (final entry in asJson(_registry['upstreams']).entries)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: Text(
                  joined([
                    {
                          '88lin': '88lin',
                          'kazumi_rules': 'Kazumi 规则',
                          'kazumi': 'Kazumi 程序',
                        }[entry.key] ??
                        entry.key,
                    asJson(entry.value)['version'],
                    asJson(entry.value)['status'],
                    asJson(entry.value)['error'],
                  ]),
                  style: const TextStyle(fontSize: 12, color: muted),
                ),
              ),
            if ((_registry['changes'] as List? ?? []).isNotEmpty)
              Text(
                (_registry['changes'] as List).map(valueText).join('；'),
                style: const TextStyle(fontSize: 12, color: muted),
              ),
          ],
        ),
      ),
      _sectionHead(
        '影视来源（${_providers.length}）',
        OutlinedButton(onPressed: _addSource, child: const Text('＋ 添加来源')),
      ),
      _table(
        ['来源', '类型', '启用', ''],
        _providers
            .map(
              (source) => <Widget>[
                SizedBox(
                  width: 220,
                  child: Text(
                    valueText(source['name']),
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                Text(
                  source['kind'] == 'kazumi'
                      ? '动漫'
                      : source['kind'] == 'cms'
                      ? 'CMS'
                      : '影视',
                ),
                Checkbox(
                  value: source['enabled'] == true,
                  onChanged: (value) => _toggleProvider(source, value == true),
                ),
                if (source['origin'] == 'custom')
                  OutlinedButton(
                    onPressed: () => _removeSource(source),
                    child: const Text('移除'),
                  )
                else
                  const SizedBox.shrink(),
              ],
            )
            .toList(),
      ),
      const SizedBox(height: 24),
      _expansion(
        '解析线路',
        _table(
          ['来源', '用途', '启用'],
          _parsers
              .map(
                (source) => <Widget>[
                  Text(valueText(source['name'])),
                  const Text('平台播放页'),
                  Checkbox(
                    value: source['enabled'] == true,
                    onChanged: (value) => _toggleParser(source, value == true),
                  ),
                ],
              )
              .toList(),
        ),
      ),
      _sectionHead(
        '连接诊断',
        FilledButton(
          onPressed: _diagnosing ? null : _diagnose,
          child: Text(_diagnosing ? '检测中…' : '检测连接'),
        ),
        top: 33,
      ),
      if (_diagnosing)
        _empty('检测中…')
      else if (_diagnosticError.isNotEmpty)
        _notice(_diagnosticError)
      else if (_diagnostics == null)
        _empty('尚未检测')
      else
        _table(
          ['来源', 'HTTP', '结果'],
          _diagnostics!
              .map(
                (result) => <Widget>[
                  Text(valueText(result['name'])),
                  Text(
                    valueText(result['http']).isEmpty
                        ? '—'
                        : valueText(result['http']),
                  ),
                  SizedBox(width: 360, child: Text(valueText(result['state']))),
                ],
              )
              .toList(),
        ),
    ],
  );

  Future<void> _saveSettingSafely(Json settings) async {
    try {
      await _saveSettings(settings);
    } catch (e) {
      _toast(e.toString());
    }
  }

  Widget _pageTitle(String title, bool desktop) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: Text(
      title,
      style: TextStyle(
        fontSize: desktop ? 32 : 28,
        fontWeight: FontWeight.w600,
      ),
    ),
  );

  Widget _sectionHead(String title, Widget action, {double top = 25}) =>
      Padding(
        padding: EdgeInsets.only(top: top, bottom: 17),
        child: Row(
          children: [
            Expanded(
              child: Text(
                title,
                style: const TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            const SizedBox(width: 12),
            action,
          ],
        ),
      );

  Widget _toggle(String title, bool value, ValueChanged<bool> onChanged) =>
      Padding(
        padding: const EdgeInsets.only(bottom: 7),
        child: Row(
          children: [
            SizedBox(
              width: 26,
              height: 32,
              child: Checkbox(
                value: value,
                onChanged: (value) => onChanged(value == true),
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                title,
                style: const TextStyle(fontSize: 12, color: Color(0xffc2d5c8)),
              ),
            ),
          ],
        ),
      );

  Widget _dropdown(
    String value,
    Map<String, String> options,
    ValueChanged<String> onChanged, {
    String placeholder = '',
  }) => DropdownButtonFormField<String>(
    initialValue: options.containsKey(value) ? value : null,
    key: ValueKey('${options.keys.join('|')}-$value'),
    isExpanded: true,
    hint: Text(placeholder, style: const TextStyle(fontSize: 12)),
    style: const TextStyle(color: foreground, fontSize: 12),
    dropdownColor: surface,
    menuMaxHeight: 420,
    items:
        options.entries
            .map(
              (entry) => DropdownMenuItem(
                value: entry.key,
                child: Text(entry.value, overflow: TextOverflow.ellipsis),
              ),
            )
            .toList(),
    onChanged:
        options.isEmpty
            ? null
            : (value) {
              if (value != null) onChanged(value);
            },
  );

  Widget _chip(String text) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 4),
    decoration: BoxDecoration(
      color: const Color(0xff1b3028),
      border: Border.all(color: const Color(0xff385447)),
      borderRadius: BorderRadius.circular(6),
    ),
    child: Text(
      text,
      style: const TextStyle(fontSize: 11, color: Color(0xffafd4c0)),
    ),
  );

  Widget _panel(
    Widget child, {
    Color color = const Color(0xff17241f),
    double padding = 22,
  }) => Container(
    width: double.infinity,
    padding: EdgeInsets.all(padding),
    decoration: BoxDecoration(
      color: color,
      border: Border.all(color: const Color(0xff2b3f36)),
      borderRadius: BorderRadius.circular(12),
    ),
    child: child,
  );

  Widget _notice(String message) => Container(
    margin: const EdgeInsets.symmetric(vertical: 18),
    padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 13),
    decoration: BoxDecoration(
      color: const Color(0xff322321),
      border: Border.all(color: const Color(0xff6b4440)),
      borderRadius: BorderRadius.circular(8),
    ),
    child: Text(
      message.replaceFirst('Exception: ', ''),
      style: const TextStyle(fontSize: 12, color: danger),
    ),
  );

  Widget _empty(String message, {Key? key}) => Container(
    key: key,
    width: double.infinity,
    padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 30),
    decoration: BoxDecoration(
      border: Border.all(color: const Color(0xff3b514d)),
      borderRadius: BorderRadius.circular(10),
    ),
    child: Text(
      message,
      style: const TextStyle(fontSize: 13, color: Color(0xff92aaa4)),
    ),
  );

  Widget _expansion(String title, Widget child) => Material(
    color: const Color(0xff16231f),
    shape: RoundedRectangleBorder(
      side: const BorderSide(color: Color(0xff2c4238)),
      borderRadius: BorderRadius.circular(10),
    ),
    clipBehavior: Clip.antiAlias,
    child: Theme(
      data: Theme.of(context).copyWith(dividerColor: Colors.transparent),
      child: ExpansionTile(
        tilePadding: const EdgeInsets.symmetric(horizontal: 15),
        childrenPadding: const EdgeInsets.fromLTRB(15, 0, 15, 15),
        title: Text(
          title,
          style: const TextStyle(fontSize: 12, color: Color(0xffb9d7c7)),
        ),
        children: [child],
      ),
    ),
  );

  Widget _table(List<String> headers, List<List<Widget>> rows) => LayoutBuilder(
    builder:
        (_, constraints) => Container(
          decoration: BoxDecoration(
            color: const Color(0xff17201f),
            border: Border.all(color: const Color(0xff2f403d)),
            borderRadius: BorderRadius.circular(10),
          ),
          clipBehavior: Clip.antiAlias,
          child: SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: ConstrainedBox(
              constraints: BoxConstraints(minWidth: constraints.maxWidth),
              child: DataTable(
                headingRowColor: const WidgetStatePropertyAll(
                  Color(0xff1e3029),
                ),
                headingTextStyle: const TextStyle(
                  fontSize: 11,
                  color: Color(0xff94b09c),
                ),
                dataTextStyle: const TextStyle(
                  fontSize: 12,
                  color: Color(0xffc1d4c7),
                ),
                horizontalMargin: 16,
                columnSpacing: 26,
                headingRowHeight: 46,
                dataRowMinHeight: 58,
                dataRowMaxHeight: 74,
                dividerThickness: 1,
                columns:
                    headers
                        .map((header) => DataColumn(label: Text(header)))
                        .toList(),
                rows:
                    rows
                        .map(
                          (cells) => DataRow(
                            cells: cells.map((cell) => DataCell(cell)).toList(),
                          ),
                        )
                        .toList(),
              ),
            ),
          ),
        ),
  );
}

class _Brand extends StatelessWidget {
  const _Brand();
  @override
  Widget build(BuildContext context) => Row(
    children: [
      Container(
        width: 43,
        height: 43,
        decoration: BoxDecoration(
          border: Border.all(color: accent),
          borderRadius: BorderRadius.circular(11),
          boxShadow: const [
            BoxShadow(color: Color(0xff233b32), offset: Offset(4, -4)),
          ],
        ),
        child: const Center(
          child: Text(
            '映',
            style: TextStyle(
              fontSize: 22,
              color: accent,
              fontWeight: FontWeight.w600,
            ),
          ),
        ),
      ),
      const SizedBox(width: 12),
      const Expanded(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              '映匣',
              style: TextStyle(
                fontSize: 25,
                fontWeight: FontWeight.w600,
                letterSpacing: 2,
                height: 1.2,
              ),
            ),
            Text(
              'YINGXIA',
              style: TextStyle(fontSize: 9, color: muted, letterSpacing: 1.5),
            ),
          ],
        ),
      ),
    ],
  );
}

class _FilmCard extends StatelessWidget {
  const _FilmCard({
    required this.film,
    required this.index,
    required this.onOpen,
  });
  final Json film;
  final int index;
  final VoidCallback onOpen;
  @override
  Widget build(BuildContext context) {
    final availability = asJson(film['availability']);
    final unavailable = availability['state'] == 'failed';
    final pending = availability['state'] == 'pending';
    final title = valueText(film['title']);
    const gradients = [
      [Color(0xff244a42), Color(0xff19282f)],
      [Color(0xff374354), Color(0xff17232f)],
      [Color(0xff48403b), Color(0xff25272a)],
    ];
    return Opacity(
      opacity: unavailable || pending ? .62 : 1,
      child: Material(
        color: const Color(0xff182328),
        shape: RoundedRectangleBorder(
          side: const BorderSide(color: border),
          borderRadius: BorderRadius.circular(12),
        ),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: pending ? null : onOpen,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Container(
                height: 145,
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topLeft,
                    end: Alignment.bottomRight,
                    colors: gradients[index % 3],
                  ),
                ),
                child: Stack(
                  children: [
                    Positioned(
                      right: -25,
                      top: -93,
                      child: Container(
                        width: 235,
                        height: 235,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          border: Border.all(color: const Color(0x29a6e1c6)),
                        ),
                      ),
                    ),
                    Positioned(
                      left: 17,
                      bottom: 10,
                      child: Text(
                        title.isEmpty
                            ? '映'
                            : String.fromCharCode(title.runes.first),
                        style: const TextStyle(
                          fontSize: 58,
                          fontWeight: FontWeight.w300,
                          color: Color(0x55b1d6c9),
                          height: 1.1,
                        ),
                      ),
                    ),
                    Positioned(
                      left: 13,
                      top: 13,
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 8,
                          vertical: 3,
                        ),
                        decoration: BoxDecoration(
                          color: const Color(0xbf0d171d),
                          borderRadius: BorderRadius.circular(5),
                        ),
                        child: Text(
                          valueText(film['kind']).isEmpty
                              ? '影片'
                              : valueText(film['kind']),
                          style: const TextStyle(
                            fontSize: 10,
                            color: Color(0xffb7d4c8),
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(15),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 7),
                    Text(
                      joined([film['year'], film['area'], film['remarks']]),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 11,
                        color: Color(0xff95aaa6),
                      ),
                    ),
                    const SizedBox(height: 12),
                    const Divider(height: 1),
                    const SizedBox(height: 10),
                    Text(
                      cleanedSource(film['source']),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 11,
                        color: Color(0xff86b79f),
                      ),
                    ),
                    if (availability.isNotEmpty)
                      Align(
                        alignment: Alignment.centerLeft,
                        child: Container(
                          margin: const EdgeInsets.only(top: 7),
                          padding: const EdgeInsets.symmetric(
                            horizontal: 7,
                            vertical: 3,
                          ),
                          decoration: BoxDecoration(
                            borderRadius: BorderRadius.circular(5),
                            border: Border.all(
                              color:
                                  availability['state'] == 'media_ok'
                                      ? const Color(0xff426a59)
                                      : border,
                            ),
                          ),
                          child: Text(
                            valueText(availability['label']),
                            style: TextStyle(
                              fontSize: 10,
                              color:
                                  availability['state'] == 'media_ok'
                                      ? accent
                                      : unavailable
                                      ? danger
                                      : muted,
                            ),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _EpisodeButton extends StatelessWidget {
  const _EpisodeButton({
    required this.name,
    required this.selected,
    required this.onPressed,
  });
  final String name;
  final bool selected;
  final VoidCallback onPressed;
  @override
  Widget build(BuildContext context) => Tooltip(
    message: name,
    child: Material(
      color: selected ? const Color(0xff83d7b5) : const Color(0xff203127),
      shape: RoundedRectangleBorder(
        side: BorderSide(
          color: selected ? const Color(0xffa3efcf) : const Color(0xff3c5146),
        ),
        borderRadius: BorderRadius.circular(6),
      ),
      child: InkWell(
        onTap: onPressed,
        borderRadius: BorderRadius.circular(6),
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 42),
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 9, horizontal: 3),
            child: Center(
              child: Text(
                name,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 11,
                  color:
                      selected
                          ? const Color(0xff133526)
                          : const Color(0xffadccba),
                ),
              ),
            ),
          ),
        ),
      ),
    ),
  );
}

class _SourceDialog extends StatefulWidget {
  const _SourceDialog();
  @override
  State<_SourceDialog> createState() => _SourceDialogState();
}

class _SourceDialogState extends State<_SourceDialog> {
  final _name = TextEditingController();
  final _url = TextEditingController();
  final _json = TextEditingController();
  String _error = '';
  @override
  void dispose() {
    _name.dispose();
    _url.dispose();
    _json.dispose();
    super.dispose();
  }

  void _submit() {
    try {
      Json data;
      if (_json.text.trim().isNotEmpty) {
        final parsed = jsonDecode(_json.text);
        final items = parsed is List ? parsed : asJson(parsed)['sources'];
        if (items is! List || items.isEmpty) {
          throw const FormatException('请输入来源 JSON 数组');
        }
        data = {'items': items};
      } else {
        if (_name.text.trim().isEmpty) throw const FormatException('请输入名称');
        final uri = Uri.tryParse(_url.text.trim());
        if (uri == null ||
            !uri.hasAuthority ||
            !['http', 'https'].contains(uri.scheme)) {
          throw const FormatException('请输入正确的接口地址');
        }
        data = {'name': _name.text.trim(), 'url': _url.text.trim()};
      }
      Navigator.pop(context, data);
    } catch (e) {
      setState(
        () => _error = e.toString().replaceFirst('FormatException: ', ''),
      );
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: const Text('添加来源'),
    content: SizedBox(
      width: 480,
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text('名称', style: TextStyle(fontSize: 12, color: muted)),
            const SizedBox(height: 7),
            TextField(
              controller: _name,
              maxLength: 80,
              decoration: const InputDecoration(
                hintText: '来源名称',
                counterText: '',
              ),
            ),
            const SizedBox(height: 14),
            const Text(
              'CMS 接口地址',
              style: TextStyle(fontSize: 12, color: muted),
            ),
            const SizedBox(height: 7),
            TextField(
              controller: _url,
              decoration: const InputDecoration(
                hintText: 'https://…/api.php/provide/vod/',
              ),
            ),
            const SizedBox(height: 12),
            ExpansionTile(
              tilePadding: EdgeInsets.zero,
              title: const Text('批量导入 JSON', style: TextStyle(fontSize: 12)),
              children: [
                TextField(
                  controller: _json,
                  minLines: 4,
                  maxLines: 8,
                  decoration: const InputDecoration(
                    hintText: '{"sources":[{"name":"来源名称","url":"https://…"}]}',
                  ),
                ),
              ],
            ),
            if (_error.isNotEmpty)
              Text(_error, style: const TextStyle(color: danger, fontSize: 12)),
          ],
        ),
      ),
    ),
    actions: [
      OutlinedButton(
        onPressed: () => Navigator.pop(context),
        child: const Text('取消'),
      ),
      FilledButton(onPressed: _submit, child: const Text('添加')),
    ],
  );
}
