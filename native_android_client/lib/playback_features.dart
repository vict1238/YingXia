// SPDX-License-Identifier: GPL-3.0-or-later
part of 'yingxia_app.dart';

extension _PlaybackFeatures on _YingXiaHomeState {
  Widget _displayShell(Widget child) {
    if (!_backend.isAndroidTv) return FocusTraversalGroup(child: child);
    final theme = Theme.of(context);
    return MediaQuery(
      data: MediaQuery.of(
        context,
      ).copyWith(textScaler: const TextScaler.linear(1.25)),
      child: Theme(
        data: theme.copyWith(
          filledButtonTheme: FilledButtonThemeData(
            style: theme.filledButtonTheme.style?.copyWith(
              minimumSize: const WidgetStatePropertyAll(Size(88, 52)),
            ),
          ),
          outlinedButtonTheme: OutlinedButtonThemeData(
            style: theme.outlinedButtonTheme.style?.copyWith(
              minimumSize: const WidgetStatePropertyAll(Size(80, 52)),
            ),
          ),
          iconButtonTheme: const IconButtonThemeData(
            style: ButtonStyle(
              minimumSize: WidgetStatePropertyAll(Size(48, 48)),
            ),
          ),
        ),
        child: FocusTraversalGroup(child: child),
      ),
    );
  }

  String _clockLabel(dynamic seconds) {
    final value = seconds is num && seconds.isFinite ? seconds.toInt() : 0;
    final safe = value.clamp(0, 7 * 24 * 3600);
    final minutes = safe ~/ 60;
    return '${minutes.toString().padLeft(2, '0')}:${(safe % 60).toString().padLeft(2, '0')}';
  }

  void _scheduleSuggestions() {
    _suggestDebounce?.cancel();
    final sequence = ++_suggestSequence;
    final query = _query.text.trim();
    if (_config == null || query.isEmpty || _searching) {
      if (_suggestions.isNotEmpty) _changed(() => _suggestions = []);
      return;
    }
    _suggestDebounce = Timer(const Duration(milliseconds: 280), () async {
      try {
        final data = asJson(
          await _backend.get('search/suggest?q=${Uri.encodeComponent(query)}'),
        );
        if (mounted && sequence == _suggestSequence && !_searching) {
          _changed(() => _suggestions = asRows(data['items']).take(8).toList());
        }
      } catch (_) {
        if (mounted && sequence == _suggestSequence) {
          _changed(() => _suggestions = []);
        }
      }
    });
  }

  Future<void> _applyCachePolicy() async {
    final native = _player?.platform;
    if (native is! NativePlayer) return;
    for (final entry
        in _cacheMode.properties(mobile: Platform.isAndroid).entries) {
      try {
        await native.setProperty(entry.key, entry.value);
      } catch (_) {
        // Keep playback available on libmpv builds missing an optional property.
      }
    }
  }

  Future<void> _explicitPlayOrPause() async {
    if (_page != 1 || !_foreground || _playback == null || _playLoading) return;
    if (_player?.state.playing == true) {
      _queueProgress();
      _setTemporaryFastForward(false);
    } else {
      _suspended = false;
      _stallMonitor.suppress(_playClock.elapsed);
    }
    await _player?.playOrPause();
  }

  Future<void> _setPlaybackRate(double value) {
    if (![0.5, 0.75, 1.0, 1.25, 1.5, 2.0, 3.0].contains(value)) {
      return Future.value();
    }
    _changed(() => _rate = value);
    final rate = _temporaryFastForward ? 2.0 : _rate;
    final task = _rateTail.then((_) async => _player?.setRate(rate));
    _rateTail = task.catchError((_) {});
    return _rateTail;
  }

  Future<void> _setTemporaryFastForward(bool active) {
    // Release/cancel must always restore, including a route or lifecycle change.
    if (active && (_suspended || !_foreground || _page != 1)) active = false;
    if (_temporaryFastForward == active) return _rateTail;
    _temporaryFastForward = active;
    final rate = active ? 2.0 : _rate;
    final task = _rateTail.then((_) async => _player?.setRate(rate));
    _rateTail = task.catchError((_) {});
    return _rateTail;
  }

  void _queueProgress({bool completed = false}) {
    final history = valueText(_playback?['history_id']);
    final player = _player;
    if (player == null ||
        history.isEmpty ||
        history != _confirmedHistory ||
        _confirmedPosition <= Duration.zero) {
      return;
    }
    final duration = player.state.duration.inMilliseconds / 1000;
    final position =
        completed && duration > 0
            ? duration
            : _confirmedPosition.inMilliseconds / 1000;
    final payload = <String, dynamic>{
      'history_id': history,
      'evidence': 'native_time_advanced',
      'position': position,
      'duration': duration,
      'completed': completed,
    };
    // Preserve last-write order on pause/stop/episode changes. The backend rejects
    // history IDs invalidated by Clear; no flush may recreate a cleared record.
    _progressTail = _progressTail
        .then((_) async {
          await _backend.post('progress', payload);
        })
        .catchError((_) {});
  }

  void _watchPlayback() {
    final player = _player;
    if (player == null) return;
    final show = _stallMonitor.observe(
      now: _playClock.elapsed,
      position: player.state.position,
      duration: player.state.duration,
      active:
          _playback != null &&
          !_playLoading &&
          !_suspended &&
          _foreground &&
          _page == 1 &&
          player.state.playing &&
          !player.state.completed,
      buffering: player.state.buffering,
    );
    if (!show || _showStallPrompt || _config?['smart_source_switch'] == false) {
      return;
    }
    // Rebuffer more before resuming, avoiding the repeated one-second cadence.
    if (_cacheMode == PlaybackCache.balanced) {
      _cacheMode = PlaybackCache.generous;
      unawaited(_applyCachePolicy());
    }
    _changed(() {
      _showStallPrompt = true;
      _alternativeNote = '';
      _alternative = null;
    });
  }

  void _cancelAlternative() {
    ++_alternativeSequence;
    final id = _alternativeJob;
    _alternativeJob = null;
    _checkingAlternative = false;
    _alternative = null;
    if (id != null) {
      unawaited(
        _backend
            .post('playback/alternatives/cancel', {'id': id})
            .catchError((_) => null),
      );
    }
  }

  Future<void> _findAlternative() async {
    if (_checkingAlternative ||
        _film == null ||
        _episode == null ||
        _group == null) {
      return;
    }
    final sequence = ++_alternativeSequence;
    _changed(() {
      _checkingAlternative = true;
      _alternativeNote = '正在检查同集来源…';
    });
    String? job;
    try {
      var result = asJson(
        await _backend.post('playback/alternatives', {
          'film_id': _film!['id'],
          'group_key': _group!['key'],
          'episode_key': _episode!['key'],
          'position': (_player?.state.position.inMilliseconds ?? 0) / 1000,
        }),
      );
      job = valueText(result['id']);
      if (!mounted || sequence != _alternativeSequence) return;
      _alternativeJob = job;
      final deadline = DateTime.now().add(const Duration(seconds: 55));
      while (mounted && sequence == _alternativeSequence) {
        final candidates = asRows(result['candidates'] ?? result['items']);
        if (candidates.isNotEmpty) {
          _changed(() {
            _alternative = candidates.first;
            _alternativeNote =
                '可切换至 ${cleanedSource(candidates.first['source'])}';
          });
          return;
        }
        final phase = valueText(result['phase']);
        if (['done', 'failed', 'cancelled'].contains(phase) ||
            DateTime.now().isAfter(deadline)) {
          break;
        }
        await Future<void>.delayed(const Duration(milliseconds: 500));
        if (!mounted || sequence != _alternativeSequence) return;
        result = asJson(
          await _backend.get(
            'playback/alternatives/status?id=${Uri.encodeComponent(job)}',
          ),
        );
      }
      if (mounted && sequence == _alternativeSequence) {
        _changed(() => _alternativeNote = '暂未找到可访问的同集来源');
      }
    } catch (_) {
      if (mounted && sequence == _alternativeSequence) {
        _changed(() => _alternativeNote = '来源检查失败，可稍后重试');
      }
    } finally {
      if (job != null && job.isNotEmpty) {
        unawaited(
          _backend
              .post('playback/alternatives/cancel', {'id': job})
              .catchError((_) => null),
        );
      }
      if (mounted && sequence == _alternativeSequence) {
        _changed(() {
          _checkingAlternative = false;
          _alternativeJob = null;
        });
      }
    }
  }

  Future<void> _switchAlternative() async {
    final candidate = _alternative;
    if (candidate == null || _page != 1 || !_foreground) return;
    final sequence = ++_detailSequence;
    try {
      final film = asJson(
        await _backend.get(
          'detail?id=${Uri.encodeComponent(valueText(candidate['film_id']))}',
        ),
      );
      if (!mounted ||
          sequence != _detailSequence ||
          _page != 1 ||
          !_foreground) {
        return;
      }
      final group =
          asRows(
            film['groups'],
          ).where((g) => g['key'] == candidate['group_key']).firstOrNull;
      final episode =
          asRows(
            group?['episodes'],
          ).where((e) => e['key'] == candidate['episode_key']).firstOrNull;
      if (group == null || episode == null) throw StateError('同集来源已变化，请重新检查');
      final position = _player?.state.position ?? _confirmedPosition;
      await _stopPlayer();
      if (!mounted ||
          sequence != _detailSequence ||
          _page != 1 ||
          !_foreground) {
        return;
      }
      _changed(() {
        _film = film;
        _group = group;
        _episode = episode;
        _resumePosition = position;
        _playerSource = 'native';
        _bindingUrl.clear();
        _bindingNote = '';
        _showStallPrompt = false;
      });
      await _play();
    } catch (e) {
      _toast(e.toString());
    }
  }

  Widget _stallBanner() => Material(
    color: const Color(0xf2203230),
    borderRadius: BorderRadius.circular(8),
    child: Padding(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      child: Wrap(
        spacing: 8,
        runSpacing: 2,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          Text(
            _alternativeNote.isEmpty ? '播放卡顿，换一个来源？' : _alternativeNote,
            style: const TextStyle(fontSize: 12),
          ),
          TextButton(
            key: const Key('stall-switch'),
            onPressed:
                _checkingAlternative
                    ? null
                    : _alternative == null
                    ? _findAlternative
                    : _switchAlternative,
            child: Text(
              _checkingAlternative
                  ? '检查中…'
                  : _alternative == null
                  ? '查找来源'
                  : '切换并续播',
            ),
          ),
          TextButton(
            onPressed: () {
              _cancelAlternative();
              _changed(() => _showStallPrompt = false);
            },
            child: const Text('暂不'),
          ),
        ],
      ),
    ),
  );

  Future<void> _showPlaybackSettings() async {
    _setTemporaryFastForward(false);
    final player = _player;
    await showDialog<void>(
      context: context,
      builder:
          (dialogContext) => StatefulBuilder(
            builder:
                (context, change) => AlertDialog(
                  title: const Text('播放设置'),
                  content: SizedBox(
                    width: 360,
                    child: SingleChildScrollView(
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          const Text('提前缓存'),
                          for (final mode in PlaybackCache.values)
                            ListTile(
                              title: Text(mode.label),
                              subtitle: Text(
                                '${mode.seconds} 秒 · 最多 ${Platform.isAndroid ? mode.mobileMiB : mode.desktopMiB} MB',
                              ),
                              leading: Icon(
                                _cacheMode == mode
                                    ? Icons.radio_button_checked
                                    : Icons.radio_button_off,
                              ),
                              selected: _cacheMode == mode,
                              onTap: () async {
                                _changed(() => _cacheMode = mode);
                                change(() {});
                                await _applyCachePolicy();
                              },
                            ),
                          if (player != null)
                            Text(
                              '已缓存约 ${((player.state.buffer - player.state.position).inSeconds).clamp(0, 9999)} 秒',
                              style: const TextStyle(color: muted),
                            ),
                          SwitchListTile(
                            contentPadding: EdgeInsets.zero,
                            title: const Text('跳过标记广告'),
                            value: _config?['skip_marked_ads'] != false,
                            onChanged: (value) async {
                              await _saveSettingSafely({
                                'skip_marked_ads': value,
                              });
                              if (context.mounted) change(() {});
                            },
                          ),
                          SwitchListTile(
                            contentPadding: EdgeInsets.zero,
                            title: const Text('卡顿时提示换源'),
                            value: _config?['smart_source_switch'] != false,
                            onChanged: (value) async {
                              await _saveSettingSafely({
                                'smart_source_switch': value,
                              });
                              if (context.mounted) change(() {});
                            },
                          ),
                        ],
                      ),
                    ),
                  ),
                  actions: [
                    TextButton(
                      onPressed: () => Navigator.pop(dialogContext),
                      child: const Text('完成'),
                    ),
                  ],
                ),
          ),
    );
  }
}
