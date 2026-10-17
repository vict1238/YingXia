// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
// Regression-test the vendored Android resource queue used by the controller.
// ignore: implementation_imports
import 'package:media_kit_video/src/video_controller/android_video_controller/android_surface_lifecycle.dart';

void main() {
  test(
    'native disposal can drain Surface while redraw waits for player lock',
    () async {
      final surface = AndroidSurfaceLifecycle();
      final redrawStarted = Completer<void>();
      final nativeDisposalFinished = Completer<void>();
      var surfaceReleased = false;

      // NativePlayer.dispose owns its player lock and calls Surface disposal.
      // A concurrent redraw seek cannot acquire that player lock until disposal
      // returns. Holding the Surface lock while awaiting this seek deadlocks.
      final update = surface.update(() async => true, () async {
        redrawStarted.complete();
        await nativeDisposalFinished.future;
      });
      await redrawStarted.future;
      final disposal = surface.drain(() async {
        surfaceReleased = true;
        nativeDisposalFinished.complete();
      });

      await Future.wait([update, disposal]).timeout(const Duration(seconds: 1));
      expect(surfaceReleased, isTrue);
    },
  );

  test(
    'disposal still waits for acknowledged in-flight Surface mutation',
    () async {
      final surface = AndroidSurfaceLifecycle();
      final mutationStarted = Completer<void>();
      final nativeResizeAcknowledged = Completer<void>();
      final events = <String>[];
      final update = surface.update(
        () async {
          mutationStarted.complete();
          await nativeResizeAcknowledged.future;
          events.add('resize_acknowledged');
          return true;
        },
        () async {
          events.add('redraw');
        },
      );
      await mutationStarted.future;
      final disposal = surface.drain(() async {
        events.add('surface_released');
      });

      await Future<void>.delayed(Duration.zero);
      expect(events, isEmpty);
      nativeResizeAcknowledged.complete();
      await Future.wait([update, disposal]);
      expect(
        events.indexOf('surface_released'),
        greaterThan(events.indexOf('resize_acknowledged')),
      );
    },
  );

  test(
    'failed Surface mutation skips seek and leaves disposal drainable',
    () async {
      final surface = AndroidSurfaceLifecycle();
      var seekRequested = false;
      await expectLater(
        surface.update(
          () async {
            throw StateError('native resize failed');
          },
          () async {
            seekRequested = true;
          },
        ),
        throwsStateError,
      );

      var released = false;
      await surface
          .drain(() async {
            released = true;
          })
          .timeout(const Duration(seconds: 1));
      expect(seekRequested, isFalse);
      expect(released, isTrue);
    },
  );
}
