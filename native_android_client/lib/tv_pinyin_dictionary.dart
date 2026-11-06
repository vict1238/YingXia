// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:convert';

import 'package:flutter/services.dart';

/// Local, ranked pinyin lookup for the TV remote keyboard.
///
/// No IME, API call, title history or network permission is needed to enter
/// Chinese. The asset is loaded lazily only when the TV keyboard needs it.
class TvPinyinDictionary {
  TvPinyinDictionary._(this._characters, this._phrases);

  static const assetPath = 'assets/tv_input/pinyin.json';
  static const _maxPhraseCandidates = 48;
  static const _maxSyllableLength = 6;
  static const _maxSyllables = 16;
  static const _maxSegmentations = 8;
  static const _beamWidth = 8;
  static const _queryLimit = 80;

  static final _separators = RegExp(r"[\s'’]+");
  static final _code = RegExp(r'^[a-z]+$');

  final Map<String, String> _characters;
  final Map<String, List<String>> _phrases;

  static Future<TvPinyinDictionary> load({AssetBundle? bundle}) async {
    final data = jsonDecode(await (bundle ?? rootBundle).loadString(assetPath));
    if (data is! Map<String, dynamic> || data['format'] != 1) {
      throw const FormatException('Unsupported TV pinyin dictionary');
    }
    final chars = Map<String, String>.from(data['chars'] as Map);
    final phrases = <String, List<String>>{
      for (final MapEntry(:key, :value) in (data['words'] as Map).entries)
        key as String: (value as List).cast<String>(),
    };
    return TvPinyinDictionary._(chars, phrases);
  }

  /// Returns full candidates for the supplied buffer, in rank order.
  ///
  /// A single syllable includes rare variants after common characters.
  /// Multiple syllables first use dictionary words and then a bounded set of
  /// complete combinations. Spaces and apostrophes specify explicit breaks.
  List<String> candidates(String pinyin) {
    final query = pinyin.trim().toLowerCase().replaceAll('u:', 'v');
    if (query.isEmpty || query.length > _queryLimit) return const [];
    final syllables = _normalize(query).split(_separators);
    if (syllables.any((part) => !_code.hasMatch(part))) return const [];
    final joined = syllables.join();
    final results = <String>{};
    if (syllables.length == 1 && _characters.containsKey(joined)) {
      results.addAll(_characters[joined]!.runes.map(String.fromCharCode));
      return results.toList(growable: false);
    }
    results.addAll(
      (_phrases[joined] ?? const <String>[]).take(_maxPhraseCandidates),
    );
    if (results.length == _maxPhraseCandidates) {
      return results.toList(growable: false);
    }
    final paths =
        syllables.length == 1
            ? _segment(joined)
            : syllables.every(_characters.containsKey) &&
                syllables.length <= _maxSyllables
            ? [syllables]
            : const <List<String>>[];
    for (final path in paths) {
      for (final candidate in _compose(path)) {
        results.add(candidate);
        if (results.length >= _maxPhraseCandidates) {
          return results.toList(growable: false);
        }
      }
    }
    return results.toList(growable: false);
  }

  String _normalize(String input) {
    const variants = {
      'āáǎà': 'a',
      'ēéěèê': 'e',
      'īíǐì': 'i',
      'ōóǒò': 'o',
      'ūúǔù': 'u',
      'üǖǘǚǜ': 'v',
      'ńňǹ': 'n',
      'ḿ': 'm',
    };
    var result = input;
    for (final entry in variants.entries) {
      result = result.replaceAll(RegExp('[${entry.key}]'), entry.value);
    }
    return result;
  }

  List<List<String>> _segment(String input) {
    final suffixes = <int, List<List<String>>>{
      input.length: [[]],
    };
    for (var start = input.length - 1; start >= 0; start--) {
      final paths = <List<String>>[];
      final bound = (start + _maxSyllableLength).clamp(0, input.length);
      for (var end = bound; end > start; end--) {
        final syllable = input.substring(start, end);
        if (!_characters.containsKey(syllable)) continue;
        for (final suffix in suffixes[end] ?? const <List<String>>[]) {
          if (suffix.length < _maxSyllables) paths.add([syllable, ...suffix]);
          if (paths.length >= _maxSegmentations) break;
        }
        if (paths.length >= _maxSegmentations) break;
      }
      suffixes[start] = paths;
    }
    return suffixes[0] ?? const [];
  }

  Iterable<String> _compose(List<String> syllables) {
    var beam = <({String value, int cost})>[(value: '', cost: 0)];
    for (final syllable in syllables) {
      final choices = _characters[syllable]!.runes.take(4).toList();
      final next = [
        for (final previous in beam)
          for (var i = 0; i < choices.length; i++)
            (
              value: previous.value + String.fromCharCode(choices[i]),
              cost: previous.cost + i,
            ),
      ]..sort((a, b) {
        final cost = a.cost.compareTo(b.cost);
        return cost != 0 ? cost : a.value.compareTo(b.value);
      });
      beam = next.take(_beamWidth).toList();
    }
    return beam.map((candidate) => candidate.value);
  }
}

Future<TvPinyinDictionary>? _sharedDictionary;

Future<List<String>> lookupTvPinyin(String query) async {
  if (query.trim().isEmpty) return const [];
  final dictionary = await (_sharedDictionary ??= TvPinyinDictionary.load());
  return dictionary.candidates(query);
}
