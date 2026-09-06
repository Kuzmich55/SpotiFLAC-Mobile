import 'dart:math' as math;

import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/providers/extension_provider.dart';

class DownloadSizeEstimate {
  final int bytes;

  const DownloadSizeEstimate({required this.bytes});
}

/// Unknown durations must not make a batch estimate look like a complete total.
Duration? totalDownloadDuration(Iterable<Track> tracks) {
  var seconds = 0;
  for (final track in tracks) {
    if (track.isCollection || track.duration <= 0) return null;
    seconds += track.duration;
  }
  return seconds > 0 ? Duration(seconds: seconds) : null;
}

DownloadSizeEstimate? estimateDownloadSize({
  required Duration? duration,
  required QualityOption quality,
  List<Track>? tracks,
  String? providerId,
}) {
  if (duration == null || duration <= Duration.zero) return null;
  final parameters = quality.sizeEstimate ?? _legacyParameters(quality);
  if (parameters == null) return null;
  final seconds = duration.inMilliseconds / 1000;
  final bitrate = parameters.bitrateKbps;
  if (bitrate != null) {
    if (bitrate <= 0 || parameters.isMaximum) return null;
    final bytes = (seconds * bitrate * 1000 / 8).round();
    return DownloadSizeEstimate(bytes: bytes);
  }

  if (tracks != null) {
    if (tracks.isEmpty || providerId == null || providerId.isEmpty) return null;
    var bytes = 0;
    for (final track in tracks) {
      // Quality from a metadata provider is not evidence for another catalog.
      if (track.source != providerId ||
          track.isCollection ||
          track.duration <= 0) {
        return null;
      }
      final match = _trackQualityPattern.firstMatch(track.audioQuality ?? '');
      if (match == null) return null;
      final depth = int.parse(match.group(1)!);
      final rate =
          (double.parse(match.group(2)!) *
                  (match.group(3)!.toLowerCase() == 'khz' ? 1000 : 1))
              .round();
      if (depth <= 0 || rate <= 0) return null;
      // Metadata reports the highest available quality of this track. A lower
      // selection caps it; a higher selection must never upscale the estimate.
      final estimate = estimateDownloadSize(
        duration: Duration(seconds: track.duration),
        quality: QualityOption(
          id: quality.id,
          label: quality.label,
          sizeEstimate: QualitySizeEstimate(
            bitDepth: math.min(parameters.bitDepth ?? depth, depth),
            sampleRate: math.min(parameters.sampleRate ?? rate, rate),
            channels: parameters.channels,
          ),
        ),
      );
      if (estimate == null) return null;
      bytes += estimate.bytes;
    }
    return DownloadSizeEstimate(bytes: bytes);
  }

  final depth = parameters.bitDepth;
  final rate = parameters.sampleRate;
  if (depth == null ||
      depth <= 0 ||
      rate == null ||
      rate <= 0 ||
      parameters.channels <= 0) {
    return null;
  }
  // Estimate compressed audio from the effective quality, assuming 65% of PCM.
  // This is a heuristic, not a measurement of this recording's compression.
  // Artwork, tags, container overhead and later conversion are excluded.
  return DownloadSizeEstimate(
    bytes: (seconds * depth * rate * parameters.channels / 8 * 0.65).round(),
  );
}

final _trackQualityPattern = RegExp(
  r'^\s*(\d+)\s*-?\s*bit\s*/\s*(\d+(?:\.\d+)?)\s*(kHz|Hz)\s*$',
  caseSensitive: false,
);

QualitySizeEstimate? _legacyParameters(QualityOption quality) {
  // These are the same legacy tiers already labelled by the picker. Custom
  // IDs (including best/high/low and spatial audio) need declared parameters.
  switch (quality.id.toUpperCase()) {
    case 'BEST':
    case 'DEFAULT':
    case 'FLAC':
      if (quality.kind == 'lossless' || quality.label.toUpperCase() == 'FLAC') {
        return const QualitySizeEstimate();
      }
      return null;
    case 'LOSSLESS':
      return const QualitySizeEstimate(bitDepth: 16, sampleRate: 44100);
    case 'HI_RES':
      return const QualitySizeEstimate(
        bitDepth: 24,
        sampleRate: 96000,
        isMaximum: true,
      );
    case 'HI_RES_LOSSLESS':
      return const QualitySizeEstimate(
        bitDepth: 24,
        sampleRate: 192000,
        isMaximum: true,
      );
  }
  // Only explicit codec/bitrate labels or IDs; never infer a bitrate from a
  // vague tier name or a description mentioning other fallback formats.
  final bitratePattern = RegExp(
    r'^(?:mp3|opus|aac)[ _-]+(\d+)(?:\s*(?:kbps|kb/s|kbit/s|k))?$',
    caseSensitive: false,
  );
  for (final value in [quality.id, quality.label]) {
    final match = bitratePattern.firstMatch(value.trim());
    if (match != null) {
      return QualitySizeEstimate(bitrateKbps: int.tryParse(match.group(1)!));
    }
  }
  return null;
}
