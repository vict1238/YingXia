// SPDX-License-Identifier: GPL-3.0-or-later

/// Native players can emit a position event on every frame. Keep an observation
/// anchor until enough motion accumulates, rather than requiring a 100 ms jump
/// between adjacent callbacks. A seek establishes a new anchor without itself
/// being treated as evidence that frames played.
class PlaybackEvidenceClock {
  Duration _anchor = Duration.zero;
  Duration _saved = Duration.zero;
  bool _firstSave = true;

  void reset() {
    _anchor = Duration.zero;
    _saved = Duration.zero;
    _firstSave = true;
  }

  bool advanced(Duration position) {
    final delta = position - _anchor;
    if (delta.isNegative || delta > const Duration(seconds: 3)) {
      _anchor = position;
      _saved = Duration.zero;
      _firstSave = true;
      return false;
    }
    if (delta <= const Duration(milliseconds: 100)) return false;
    _anchor = position;
    return true;
  }

  bool shouldSave(Duration position) {
    if (!_firstSave && position - _saved <= const Duration(seconds: 3)) {
      return false;
    }
    _saved = position;
    _firstSave = false;
    return true;
  }
}
