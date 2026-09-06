import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/utils/lyrics_metadata_helper.dart';

void main() {
  group('shared lyric usability cases', () {
    final cases = File(
      'android/app/src/test/resources/lyrics_usability_cases.tsv',
    ).readAsLinesSync();
    for (final line in cases) {
      if (line.isEmpty || line.startsWith('#')) continue;
      final fields = line.split('\t');
      test(fields.first, () {
        expect(fields, hasLength(3));
        final lyrics = fields[2]
            .replaceAll(r'\n', '\n')
            .replaceAll(r'\r', '\r')
            .replaceAll(r'\t', '\t');
        expect(hasUsableLyricsContent(lyrics), fields[1] == 'true');
      });
    }
  });

  group('lyrics display normalization', () {
    test('rejects metadata-only embedded LRC', () {
      const raw = '''
[ti:Banna Re]
[ar:Dhvani Bhanushali]
[al:Banna Re]
[by:SpotiFLAC Mobile]
''';

      expect(cleanLyricsForDisplay(raw), isEmpty);
      expect(hasUsableLyricsContent(raw), isFalse);
      expect(
        hasEmbeddedLyricsMetadata({'LYRICS': raw, 'UNSYNCEDLYRICS': raw}),
        isFalse,
      );
    });

    test('keeps timed, background, and speaker lyric content', () {
      const raw = '''
[ti:Song]
[00:01.25]Lead line
[bg:Background line]
[00:02:500]<00:02.500>v2: Harmony line
''';

      expect(
        cleanLyricsForDisplay(raw),
        'Lead line\nBackground line\nHarmony line',
      );
      expect(hasUsableLyricsContent(raw), isTrue);
    });

    test('recognizes the instrumental marker without rendering it as text', () {
      expect(isInstrumentalLyricsMarker(' [Instrumental:TRUE] '), isTrue);
      expect(hasUsableLyricsContent('[instrumental:true]'), isTrue);
      expect(cleanLyricsForDisplay('[instrumental:true]'), isEmpty);
    });
  });

  group('metadata conversion merge', () {
    test('preserves explicit advisory using the canonical tag', () {
      final metadata = <String, String>{};

      mergePlatformMetadataForTagEmbed(
        target: metadata,
        source: {'explicit': true},
      );

      expect(metadata['ITUNESADVISORY'], '1');
    });

    test('removes a stale explicit advisory for a non-explicit file', () {
      final metadata = <String, String>{'ITUNESADVISORY': '1'};

      mergePlatformMetadataForTagEmbed(
        target: metadata,
        source: {'explicit': false},
      );

      expect(metadata, isNot(contains('ITUNESADVISORY')));
    });
  });
}
