import 'dart:math' as math;

import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/providers/extension_provider.dart';

class DownloadSizeEstimate {
  final int minBytes;
  final int maxBytes;

  const DownloadSizeEstimate({required this.minBytes, required this.maxBytes});
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
}) {
  if (duration == null || duration <= Duration.zero) return null;
  final parameters = quality.sizeEstimate ?? _legacyParameters(quality);
  if (parameters == null) return null;
  final seconds = duration.inMilliseconds / 1000;
  final bitrate = parameters.bitrateKbps;
  if (bitrate != null) {
    if (bitrate <= 0 || parameters.isMaximum) return null;
    final bytes = (seconds * bitrate * 1000 / 8).round();
    return DownloadSizeEstimate(minBytes: bytes, maxBytes: bytes);
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
  final minDepth = parameters.isMaximum ? math.min(depth, 16) : depth;
  final minRate = parameters.isMaximum ? math.min(rate, 44100) : rate;
  // A rough compressed-lossless range (50–80% of PCM), not a bound or a
  // guarantee. Capped tiers can deliver CD quality instead of their maximum.
  // Artwork, tags, container overhead and later conversion are excluded.
  return DownloadSizeEstimate(
    minBytes: (seconds * minDepth * minRate * parameters.channels / 8 * 0.5)
        .round(),
    maxBytes: (seconds * depth * rate * parameters.channels / 8 * 0.8).round(),
  );
}

QualitySizeEstimate? _legacyParameters(QualityOption quality) {
  // These are the same legacy tiers already labelled by the picker. Custom
  // IDs (including best/high/low and spatial audio) need declared parameters.
  switch (quality.id.toUpperCase()) {
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
