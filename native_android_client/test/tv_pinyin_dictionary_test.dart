// SPDX-License-Identifier: GPL-3.0-or-later
import 'package:flutter_test/flutter_test.dart';
import 'package:yingxia_native/tv_pinyin_dictionary.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late TvPinyinDictionary dictionary;

  setUpAll(() async {
    dictionary = await TvPinyinDictionary.load();
  });

  test('bundled data ranks common letters and retains rare Han characters', () {
    expect(dictionary.candidates('nǐ').first, '你');
    expect(dictionary.candidates('hao').first, '好');
    expect(dictionary.candidates('lü'), contains('绿'));
    expect(dictionary.candidates('lu:'), dictionary.candidates('lv'));
    expect(dictionary.candidates('qiu'), containsAll(['球', '㐀']));
  });

  test('bundled phrases and movie titles work without any network', () async {
    expect(dictionary.candidates("shang'hai").first, '上海');
    expect(dictionary.candidates('LIU LANG DI QIU').first, '流浪地球');
    expect(dictionary.candidates('nazha').first, '哪吒');
    expect(
      dictionary.candidates('ne zha zhi mo tong nao hai').first,
      '哪吒之魔童闹海',
    );
    expect((await lookupTvPinyin('haizeiwang')).first, '海贼王');
  });

  test('composes an uncommon syllable sequence and bounds candidates', () {
    final candidates = dictionary.candidates('ni hao qiu');
    expect(candidates.first, '你好球');
    expect(candidates.toSet().length, candidates.length);
    expect(candidates.length, lessThanOrEqualTo(48));
    expect(dictionary.candidates('nihaoqiu'), contains('你好球'));
    expect(dictionary.candidates('not@pinyin'), isEmpty);
    expect(dictionary.candidates(' '), isEmpty);
    expect(dictionary.candidates('a' * 81), isEmpty);
  });
}
