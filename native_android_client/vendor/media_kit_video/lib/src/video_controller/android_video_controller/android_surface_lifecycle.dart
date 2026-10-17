/// This file is a part of media_kit (https://github.com/media-kit/media-kit).
/// Copyright © 2021 & onwards, Hitesh Kumar Saini.
/// Use of this source code is governed by the MIT license in LICENSE.
import 'package:synchronized/synchronized.dart';

/// Serializes Surface/VO mutation and disposal. Redraw can acquire the native
/// player lock, whose disposal path itself drains this queue, so redraw must
/// run only after releasing the Surface lock.
class AndroidSurfaceLifecycle {
  final _lock = Lock();

  Future<void> update(
    Future<bool> Function() mutate,
    Future<void> Function() redraw,
  ) async {
    final changed = await _lock.synchronized(mutate);
    if (changed) await redraw();
  }

  /// Release native Surface resources after all in-flight mutations complete.
  Future<void> drain(Future<void> Function() release) =>
      _lock.synchronized(release);
}
