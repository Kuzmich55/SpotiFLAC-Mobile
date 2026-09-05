import 'dart:io';

/// Trims old cached payloads to a byte budget. Repository metadata, symlinks,
/// and recently written files (possibly active downloads) are left alone.
Future<int> trimCacheToByteBudget(
  Directory directory, {
  required int maxBytes,
  required int targetBytes,
  Duration minimumAge = const Duration(minutes: 1),
}) async {
  assert(targetBytes >= 0 && targetBytes <= maxBytes);
  if (!await directory.exists()) return 0;
  final files = <(File, FileStat)>[];
  var total = 0;
  await for (final entry in directory.list(
    recursive: true,
    followLinks: false,
  )) {
    if (entry is! File || entry.path.endsWith('.json')) continue;
    final stat = await entry.stat();
    if (stat.type != FileSystemEntityType.file) continue;
    files.add((entry, stat));
    total += stat.size;
  }
  if (total <= maxBytes) return 0;
  files.sort((a, b) => a.$2.modified.compareTo(b.$2.modified));
  final cutoff = DateTime.now().subtract(minimumAge);
  var deleted = 0;
  for (final (file, stat) in files) {
    if (total <= targetBytes) break;
    if (stat.modified.isAfter(cutoff)) continue;
    try {
      await file.delete();
      total -= stat.size;
      deleted++;
    } on FileSystemException {
      // A cache download/eviction may have changed the directory since listing.
    }
  }
  return deleted;
}
