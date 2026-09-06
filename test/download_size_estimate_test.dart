import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/providers/extension_provider.dart';
import 'package:spotiflac_android/utils/download_size_estimate.dart';

Track _track(int duration, {String? itemType}) => Track(
  id: 'track',
  name: 'Track',
  artistName: 'Artist',
  albumName: 'Album',
  duration: duration,
  itemType: itemType,
);

void main() {
  const duration = Duration(minutes: 4);

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
      expect(estimate.assumedBitDepth, isNull);
      expect(estimate.assumedSampleRate, isNull);
    }
  });

  test('lossless estimates compare each tier at its own quality', () {
    const cd = QualityOption(id: 'LOSSLESS', label: 'Lossless');
    final cdSize = estimateDownloadSize(duration: duration, quality: cd)!;
    expect(cdSize.bytes, 27518400);
    expect(cdSize.assumedSampleRate, isNull);
    final batchSize = estimateDownloadSize(
      duration: totalDownloadDuration([_track(240), _track(240)]),
      quality: cd,
    )!;
    expect(batchSize.bytes, cdSize.bytes * 2);

    for (final (id, rate, bytes) in [
      ('HI_RES', 96000, 89856000),
      ('HI_RES_LOSSLESS', 192000, 179712000),
    ]) {
      final size = estimateDownloadSize(
        duration: duration,
        quality: QualityOption(id: id, label: ''),
      )!;
      expect(size.bytes, bytes);
      expect(size.bytes, greaterThan(cdSize.bytes));
      expect(size.assumedBitDepth, 24);
      expect(size.assumedSampleRate, rate);
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
      expect(estimate.assumedBitDepth, isNull);
      expect(estimate.assumedSampleRate, isNull);
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
      expect(capped.assumedBitDepth, 24);
      expect(capped.assumedSampleRate, 192000);
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
