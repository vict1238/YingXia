// SPDX-License-Identifier: GPL-3.0-or-later
/// Bounded packet caches. Disk caching is deliberately disabled: libmpv's
/// append-only temporary cache otherwise grows for the lifetime of the movie.
enum PlaybackCache {
  balanced('标准', 60, 48, 96, 3),
  generous('提前缓存', 120, 80, 160, 5);

  const PlaybackCache(
    this.label,
    this.seconds,
    this.mobileMiB,
    this.desktopMiB,
    this.restartSeconds,
  );
  final String label;
  final int seconds, mobileMiB, desktopMiB, restartSeconds;
  Map<String, String> properties({required bool mobile}) => {
    'cache': 'yes',
    'cache-on-disk': 'no',
    'cache-secs': '$seconds',
    'demuxer-readahead-secs': '$seconds',
    'demuxer-max-bytes': '${mobile ? mobileMiB : desktopMiB}MiB',
    'demuxer-max-back-bytes': '8MiB',
    'cache-pause': 'yes',
    'cache-pause-initial': 'yes',
    'cache-pause-wait': '$restartSeconds',
    'network-timeout': '20',
  };
}

/// Uses monotonic elapsed time, ignores deliberate pause/seek/startup and EOF,
/// and rate-limits prompts. Rendering drops are handled separately by FSR.
class PlaybackStallMonitor {
  Duration? _lastTime, _lastAdvance, _bufferStart, _lastPrompt;
  Duration _position = Duration.zero;
  Duration _graceUntil = Duration.zero;
  final List<Duration> _stalls = [];

  void reset(Duration now) {
    _lastTime = now;
    _lastAdvance = now;
    _bufferStart = null;
    _lastPrompt = null;
    _position = Duration.zero;
    _graceUntil = now + const Duration(seconds: 8);
    _stalls.clear();
  }

  void suppress(Duration now) {
    _bufferStart = null;
    _lastAdvance = now;
    _graceUntil = now + const Duration(seconds: 8);
  }

  bool observe({
    required Duration now,
    required Duration position,
    required Duration duration,
    required bool active,
    required bool buffering,
  }) {
    final elapsed = _lastTime == null ? Duration.zero : now - _lastTime!;
    final delta = position - _position;
    _lastTime = now;
    _position = position;
    if (!active ||
        (duration > Duration.zero &&
            duration - position < const Duration(seconds: 3))) {
      suppress(now);
      return false;
    }
    // A large jump is a seek, not a network stall (also allows 2x playback).
    if (delta.isNegative || delta > elapsed * 3 + const Duration(seconds: 2)) {
      suppress(now);
      return false;
    }
    if (delta > const Duration(milliseconds: 80)) _lastAdvance = now;
    if (now < _graceUntil) return false;
    _stalls.removeWhere((time) => now - time > const Duration(seconds: 45));
    if (buffering) {
      _bufferStart ??= now;
    } else if (_bufferStart != null) {
      if (now - _bufferStart! >= const Duration(seconds: 1)) _stalls.add(now);
      _bufferStart = null;
    }
    final continuous =
        _bufferStart != null &&
        now - _bufferStart! >= const Duration(seconds: 5);
    final frozen =
        _lastAdvance != null &&
        now - _lastAdvance! >= const Duration(seconds: 9);
    if (!(continuous || frozen || _stalls.length >= 3)) return false;
    if (_lastPrompt != null &&
        now - _lastPrompt! < const Duration(seconds: 90)) {
      return false;
    }
    _lastPrompt = now;
    _stalls.clear();
    return true;
  }
}

Duration boundedResume(num? seconds, Duration duration) {
  final value = seconds?.toDouble() ?? 0;
  if (!value.isFinite || value <= 0 || value > 7 * 24 * 3600) {
    return Duration.zero;
  }
  if (duration > Duration.zero && value >= duration.inMilliseconds / 1000 - 5) {
    return Duration.zero;
  }
  return Duration(milliseconds: (value * 1000).round());
}
