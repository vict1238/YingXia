// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:ffi';

import 'package:ffi/ffi.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:media_kit/generated/libmpv/bindings.dart' as mpv;
import 'package:yingxia_native/mpv_nodes.dart';

void main() {
  test(
    'copies nested typed mpv VO map, pass list, names and 64-bit timings',
    () {
      final arena = Arena();
      try {
        final root = arena<mpv.mpv_node>();
        root.ref.format = mpv.mpv_format.MPV_FORMAT_NODE_MAP;
        final map = arena<mpv.mpv_node_list>();
        root.ref.u.list = map;
        map.ref.num = 1;
        map.ref.keys = arena<Pointer<Int8>>(1);
        map.ref.keys[0] = 'fresh'.toNativeUtf8(allocator: arena).cast();
        map.ref.values = arena<mpv.mpv_node>(1);
        map.ref.values[0].format = mpv.mpv_format.MPV_FORMAT_NODE_ARRAY;
        final passes = arena<mpv.mpv_node_list>();
        map.ref.values[0].u.list = passes;
        passes.ref.num = 1;
        passes.ref.values = arena<mpv.mpv_node>(1);
        passes.ref.values[0].format = mpv.mpv_format.MPV_FORMAT_NODE_MAP;
        final pass = arena<mpv.mpv_node_list>();
        passes.ref.values[0].u.list = pass;
        pass.ref.num = 3;
        pass.ref.keys = arena<Pointer<Int8>>(3);
        pass.ref.values = arena<mpv.mpv_node>(3);
        for (var index = 0; index < 3; index++) {
          pass.ref.keys[index] =
              [
                'desc',
                'avg',
                'count',
              ][index].toNativeUtf8(allocator: arena).cast();
        }
        pass.ref.values[0].format = mpv.mpv_format.MPV_FORMAT_STRING;
        pass.ref.values[0].u.string =
            'YingXia FSR1 EASU'.toNativeUtf8(allocator: arena).cast();
        pass.ref.values[1].format = mpv.mpv_format.MPV_FORMAT_INT64;
        pass.ref.values[1].u.int64 = 5000000000;
        pass.ref.values[2].format = mpv.mpv_format.MPV_FORMAT_INT64;
        pass.ref.values[2].u.int64 = 32;
        expect(copyMpvNode(root.ref), {
          'fresh': [
            {'desc': 'YingXia FSR1 EASU', 'avg': 5000000000, 'count': 32},
          ],
        });
      } finally {
        arena.releaseAll();
      }
    },
  );

  test(
    'unavailable, null and invalid typed nodes stay absent instead of success',
    () {
      final arena = Arena();
      try {
        final node = arena<mpv.mpv_node>();
        node.ref.format = mpv.mpv_format.MPV_FORMAT_NONE;
        expect(copyMpvNode(node.ref), isNull);
        node.ref.format = mpv.mpv_format.MPV_FORMAT_NODE_ARRAY;
        expect(copyMpvNode(node.ref), isNull);
        final list = arena<mpv.mpv_node_list>();
        node.ref.u.list = list;
        list.ref.num = 3000;
        expect(copyMpvNode(node.ref), isNull);
        list.ref.num = -1;
        expect(copyMpvNode(node.ref), isNull);
        list.ref.num = 0;
        expect(copyMpvNode(node.ref), isEmpty);
        node.ref.format = mpv.mpv_format.MPV_FORMAT_DOUBLE;
        node.ref.u.double_ = double.nan;
        expect(copyMpvNode(node.ref), isNull);
        node.ref.format = mpv.mpv_format.MPV_FORMAT_FLAG;
        node.ref.u.flag = 1;
        expect(copyMpvNode(node.ref), isTrue);
      } finally {
        arena.releaseAll();
      }
    },
  );
}
