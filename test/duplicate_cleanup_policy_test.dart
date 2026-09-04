import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/services/library_database.dart';
import 'package:spotiflac_android/utils/duplicate_cleanup_policy.dart';

IsrcDuplicateEntry _entry(String id) {
  return IsrcDuplicateEntry(
    id: id,
    source: 'local',
    trackName: 'Track $id',
    artistName: 'Artist',
    albumName: 'Album',
    filePath: '/music/$id.flac',
  );
}

void main() {
  test('keep-best-all retains one entry and deletes the rest per group', () {
    final firstBest = _entry('first-best');
    final firstLower = _entry('first-lower');
    final firstLowest = _entry('first-lowest');
    final secondBest = _entry('second-best');
    final secondLower = _entry('second-lower');

    final plan = buildKeepBestAllDuplicatePlan([
      IsrcDuplicateGroup(
        isrc: 'FIRST',
        entries: [firstBest, firstLower, firstLowest],
      ),
      IsrcDuplicateGroup(isrc: 'SECOND', entries: [secondBest, secondLower]),
    ]);

    expect(plan.groupCount, 2);
    expect(plan.retainedEntries, [firstBest, secondBest]);
    expect(plan.entriesToDelete, [firstLower, firstLowest, secondLower]);
  });

  test('keep-best-all ignores groups that no longer contain duplicates', () {
    final onlyCopy = _entry('only-copy');

    final plan = buildKeepBestAllDuplicatePlan([
      IsrcDuplicateGroup(isrc: 'ONLY', entries: [onlyCopy]),
      const IsrcDuplicateGroup(isrc: 'EMPTY', entries: []),
    ]);

    expect(plan.groupCount, 0);
    expect(plan.retainedEntries, isEmpty);
    expect(plan.entriesToDelete, isEmpty);
  });
}
