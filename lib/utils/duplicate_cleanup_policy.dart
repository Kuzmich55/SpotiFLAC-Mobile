import 'package:spotiflac_android/services/library_database.dart';

class DuplicateCleanupPlan {
  final List<IsrcDuplicateEntry> retainedEntries;
  final List<IsrcDuplicateEntry> entriesToDelete;
  final int groupCount;

  const DuplicateCleanupPlan({
    required this.retainedEntries,
    required this.entriesToDelete,
    required this.groupCount,
  });
}

/// Keeps the first (highest-quality) entry from every duplicate group and
/// schedules every remaining copy for deletion.
///
/// [LibraryDatabase.findIsrcDuplicateGroups] returns entries best-quality
/// first, so this policy deliberately preserves that ordering contract.
DuplicateCleanupPlan buildKeepBestAllDuplicatePlan(
  List<IsrcDuplicateGroup> groups,
) {
  final retainedEntries = <IsrcDuplicateEntry>[];
  final entriesToDelete = <IsrcDuplicateEntry>[];
  var groupCount = 0;

  for (final group in groups) {
    if (group.entries.length < 2) continue;
    groupCount++;
    retainedEntries.add(group.entries.first);
    entriesToDelete.addAll(group.entries.skip(1));
  }

  return DuplicateCleanupPlan(
    retainedEntries: List.unmodifiable(retainedEntries),
    entriesToDelete: List.unmodifiable(entriesToDelete),
    groupCount: groupCount,
  );
}
