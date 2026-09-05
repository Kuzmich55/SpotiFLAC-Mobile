import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/providers/download_history_provider.dart';
import 'package:spotiflac_android/providers/library_collections_provider.dart';
import 'package:spotiflac_android/services/history_database.dart';

/// Collection providers replace their track list when its contents change.
/// Keep derived data across selection/layout rebuilds of the same snapshot.
class LibraryTracksFolderData {
  List<CollectionTrackEntry>? _entries;
  Set<String> keys = const {};
  List<Track> tracks = const [];
  HistoryBatchLookupRequest historyRequest = const HistoryBatchLookupRequest(
    [],
  );

  void update(List<CollectionTrackEntry> entries) {
    if (identical(entries, _entries)) return;
    _entries = entries;
    keys = Set.unmodifiable(entries.map((entry) => entry.key));
    tracks = List.unmodifiable(entries.map((entry) => entry.track));
    historyRequest = HistoryBatchLookupRequest.snapshot(
      tracks.map(historyLookupForTrack),
    );
  }
}
