import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/providers/extension_provider.dart';
import 'package:spotiflac_android/utils/download_size_estimate.dart';

Track _track(
  int duration, {
  String? itemType,
  String? audioQuality,
  String? source = 'provider-a',
}) => Track(
  id: 'track',
  name: 'Track',
  artistName: 'Artist',
  albumName: 'Album',
  duration: duration,
  itemType: itemType,
  audioQuality: audioQuality,
  source: source,
);

void main() {
  const duration = Duration(minutes: 4);

  test('track metadata caps quality and sums each recording separately', () {
    final cd = _track(240, audioQuality: '16bit/44.1kHz');
    final hiRes = _track(120, audioQuality: '24-bit/96000Hz');
    for (final id in ['LOSSLESS', 'HI_RES', 'HI_RES_LOSSLESS']) {
      expect(
        estimateDownloadSize(
          duration: duration,
          quality: QualityOption(id: id, label: ''),
          tracks: [cd],
          providerId: 'provider-a',
        )!.bytes,
        27518400,
      );
    }
    final tracks = [cd, hiRes];
    expect(
      estimateDownloadSize(
        duration: totalDownloadDuration(tracks),
        quality: QualityOption.fromJson({
          'id': 'best',
          'label': 'Best',
          'kind': 'lossless',
        }),
        tracks: tracks,
        providerId: 'provider-a',
      )!.bytes,
      27518400 + 44928000,
    );
  });

  test(
    'lossless metadata must be complete and belong to selected provider',
    () {
      for (final track in [
        _track(240),
        _track(240, audioQuality: '24bit'),
        _track(240, audioQuality: '16bit/44.1kHz', source: 'provider-b'),
      ]) {
        expect(
          estimateDownloadSize(
            duration: duration,
            quality: const QualityOption(id: 'LOSSLESS', label: ''),
            tracks: [track],
            providerId: 'provider-a',
          ),
          isNull,
        );
      }
    },
  );

  test('four minutes at 256 kbps is 7,680,000 bytes before overhead', () {
    for (final quality in [
      const QualityOption(id: 'opus_256', label: 'Opus'),
      const QualityOption(id: 'custom', label: 'Opus 256kbps'),
      QualityOption.fromJson({
        'id': 'custom',
        'label': 'Audio',
        'sizeEstimate': {'bitrateKbps': 256},
      }),
    ]) {
      final estimate = estimateDownloadSize(
        duration: duration,
        quality: quality,
      )!;
      expect(estimate.bytes, 7680000);
    }
  });

  test('lossless estimates compare each tier at its own quality', () {
    const cd = QualityOption(id: 'LOSSLESS', label: 'Lossless');
    final cdSize = estimateDownloadSize(duration: duration, quality: cd)!;
    expect(cdSize.bytes, 27518400);
    final batchSize = estimateDownloadSize(
      duration: totalDownloadDuration([_track(240), _track(240)]),
      quality: cd,
    )!;
    expect(batchSize.bytes, cdSize.bytes * 2);

    for (final (id, bytes) in [
      ('HI_RES', 89856000),
      ('HI_RES_LOSSLESS', 179712000),
    ]) {
      final size = estimateDownloadSize(
        duration: duration,
        quality: QualityOption(id: id, label: ''),
      )!;
      expect(size.bytes, bytes);
      expect(size.bytes, greaterThan(cdSize.bytes));
    }
  });

  test(
    'explicit parameters override legacy assumptions and preserve channels',
    () {
      final quality = QualityOption.fromJson({
        'id': 'LOSSLESS',
        'label': 'Custom lossless',
        'sizeEstimate': {'bitDepth': 24, 'sampleRate': 48000, 'channels': 1},
      });
      final estimate = estimateDownloadSize(
        duration: duration,
        quality: quality,
      )!;
      expect(estimate.bytes, 22464000);
      final capped = estimateDownloadSize(
        duration: duration,
        quality: QualityOption.fromJson({
          'id': 'best',
          'label': 'Best',
          'sizeEstimate': {
            'bitDepth': 24,
            'sampleRate': 192000,
            'isMaximum': true,
          },
        }),
      )!;
      expect(capped.bytes, 179712000);
    },
  );

  test(
    'unknown tiers, incomplete or invalid parameters do not invent sizes',
    () {
      for (final quality in [
        const QualityOption(id: 'best', label: 'Best'),
        const QualityOption(id: 'HIGH', label: 'High'),
        const QualityOption(id: 'DOLBY_ATMOS', label: 'Spatial'),
        const QualityOption(id: 'opus', label: 'Opus'),
        const QualityOption(
          id: 'custom',
          label: 'Best',
          description: 'May fall back to Opus 256kbps',
        ),
        const QualityOption(
          id: 'LOSSLESS',
          label: '',
          sizeEstimate: QualitySizeEstimate(bitDepth: 24),
        ),
        const QualityOption(
          id: 'LOSSLESS',
          label: '',
          sizeEstimate: QualitySizeEstimate(bitDepth: 0, sampleRate: 48000),
        ),
        const QualityOption(
          id: 'opus_256',
          label: '',
          sizeEstimate: QualitySizeEstimate(bitrateKbps: -1),
        ),
        const QualityOption(
          id: 'custom',
          label: '',
          sizeEstimate: QualitySizeEstimate(bitrateKbps: 320, isMaximum: true),
        ),
      ]) {
        expect(
          estimateDownloadSize(duration: duration, quality: quality),
          isNull,
        );
      }
      for (final duration in [
        null,
        Duration.zero,
        const Duration(seconds: -1),
      ]) {
        expect(
          estimateDownloadSize(
            duration: duration,
            quality: const QualityOption(id: 'mp3_320', label: 'MP3'),
          ),
          isNull,
        );
      }
    },
  );

  test('batch duration is unavailable when tracks are missing duration', () {
    expect(totalDownloadDuration([]), isNull);
    expect(totalDownloadDuration([_track(240), _track(0)]), isNull);
    expect(totalDownloadDuration([_track(-1)]), isNull);
    expect(totalDownloadDuration([_track(240, itemType: 'album')]), isNull);
    expect(
      totalDownloadDuration([_track(120), _track(180)]),
      const Duration(minutes: 5),
    );
  });
}
