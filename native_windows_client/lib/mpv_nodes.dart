// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:convert';
import 'dart:ffi';

import 'package:media_kit/generated/libmpv/bindings.dart' as mpv;

/// Copies an mpv-owned node before mpv_free_node_contents. Reading a property
/// as a string does not preserve the node structure of VO performance data.
/// Bounds protect diagnostics from unexpectedly large maps/sample arrays.
Object? copyMpvNode(mpv.mpv_node node) {
  var remaining = 8192;
  String text(Pointer<Int8> pointer) {
    if (pointer == nullptr) return '';
    final bytes = pointer.cast<Uint8>();
    var length = 0;
    while (length < 4096 && bytes[length] != 0) {
      length++;
    }
    return utf8.decode(bytes.asTypedList(length), allowMalformed: true);
  }

  Object? copy(mpv.mpv_node value, int depth) {
    if (depth > 8 || --remaining < 0) return null;
    switch (value.format) {
      case mpv.mpv_format.MPV_FORMAT_STRING:
        return text(value.u.string);
      case mpv.mpv_format.MPV_FORMAT_FLAG:
        return value.u.flag != 0;
      case mpv.mpv_format.MPV_FORMAT_INT64:
        return value.u.int64;
      case mpv.mpv_format.MPV_FORMAT_DOUBLE:
        return value.u.double_.isFinite ? value.u.double_ : null;
      case mpv.mpv_format.MPV_FORMAT_NODE_ARRAY:
      case mpv.mpv_format.MPV_FORMAT_NODE_MAP:
        final pointer = value.u.list;
        if (pointer == nullptr) return null;
        final list = pointer.ref;
        if (list.num < 0 ||
            list.num > 2048 ||
            (list.num > 0 && list.values == nullptr)) {
          return null;
        }
        if (value.format == mpv.mpv_format.MPV_FORMAT_NODE_ARRAY) {
          return [
            for (var index = 0; index < list.num; index++)
              copy((list.values + index).ref, depth + 1),
          ];
        }
        if (list.num > 0 && list.keys == nullptr) return null;
        return <String, Object?>{
          for (var index = 0; index < list.num; index++)
            text(list.keys[index]): copy((list.values + index).ref, depth + 1),
        };
      default:
        return null;
    }
  }

  return copy(node, 0);
}
