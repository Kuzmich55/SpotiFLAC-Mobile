import 'dart:collection';
import 'dart:math';
import 'package:spotiflac_android/models/download_item.dart';
import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/providers/download_queue_state.dart';
import 'package:spotiflac_android/utils/chunked_list.dart';
import 'package:flutter_test/flutter_test.dart';

class _CountingList<T> extends ListBase<T> {
  _CountingList(this.source);
  final List<T> source;
  int reads = 0;
  @override
  int get length => source.length;
  @override
  set length(int value) => throw UnsupportedError('read only');
  @override
  T operator [](int index) {
    reads++;
    return source[index];
  }

  @override
  void operator []=(int index, T value) => throw UnsupportedError('read only');
}

DownloadItem _item(int index) => DownloadItem(
  id: 'item-$index',
  track: Track(
    id: 'track-${index ~/ 2}',
    name: 'Song',
    artistName: 'Artist',
    albumName: 'Album',
    duration: 1000,
  ),
  service: 'extension.test',
  createdAt: DateTime.utc(2026),
  status: DownloadStatus.downloading,
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'sparse updates preserve snapshots and do not revisit the source list',
    () {
      final source = _CountingList(List.generate(10000, (i) => i));
      final initial = ChunkedList<int>.from(source);
      source.reads = 0;
      var current = initial;
      final expected = List<int>.of(initial);
      final random = Random(42);
      for (var iteration = 0; iteration < 1000; iteration++) {
        final index = random.nextInt(initial.length);
        final previous = current;
        final oldValue = previous[index];
        current = current.updated({index: -iteration - 1});
        expected[index] = -iteration - 1;
        expect(previous[index], oldValue);
      }
      expect(current, expected);
      expect(initial, List.generate(10000, (i) => i));
      expect(source.reads, 0);
      expect(() => current[0] = 1, throwsUnsupportedError);
      expect(() => current.updated({10000: 1}), throwsRangeError);
    },
  );

  test(
    'queue indexes match a full rebuild through sparse status transitions',
    () {
      var state = const DownloadQueueState().copyWith(
        items: List.generate(130, _item),
      );
      final original = state;
      final random = Random(17);
      for (var tick = 0; tick < 150; tick++) {
        final index = random.nextInt(state.items.length);
        final old = state;
        final next = ChunkedList<DownloadItem>.from(state.items).updated({
          index: state.items[index].copyWith(
            progress: tick / 150,
            status: DownloadStatus.values[tick % DownloadStatus.values.length],
          ),
        });
        state = state.copyWith(
          items: next,
          lookup: state.lookup.updatedForIndices(
            previousItems: state.items,
            nextItems: next,
            changedIndices: [index, index],
          ),
        );
        final rebuilt = DownloadQueueLookup.fromItems(next);
        expect(state.lookup.byItemId, rebuilt.byItemId);
        expect(state.lookup.byTrackId, rebuilt.byTrackId);
        expect(state.lookup.queuedCount, rebuilt.queuedCount);
        expect(state.lookup.completedCount, rebuilt.completedCount);
        expect(state.lookup.failedCount, rebuilt.failedCount);
        expect(state.lookup.activeDownloadsCount, rebuilt.activeDownloadsCount);
        expect(state.lookup.finalizingCount, rebuilt.finalizingCount);
        expect(state.lookup.notCompletedItemIds, rebuilt.notCompletedItemIds);
        expect(
          old.lookup.byItemId[old.items[index].id],
          same(old.items[index]),
        );
      }
      expect(original.items.every((item) => item.progress == 0), isTrue);
      expect(
        original.lookup.byItemId.values.every((item) => item.progress == 0),
        isTrue,
      );
      expect(() => state.lookup.byItemId.clear(), throwsUnsupportedError);
    },
  );
}
