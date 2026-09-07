import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:spotiflac_android/l10n/app_localizations.dart';
import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/screens/album_screen.dart';
import 'package:spotiflac_android/widgets/track_list_tile.dart';

void main() {
  testWidgets('album tracks retain the extended tags supplied in search', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    await tester.binding.setSurfaceSize(const Size(430, 1200));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    const channel = MethodChannel('com.zarz.spotiflac/backend');
    const payload = <String, dynamic>{
      'id': 'song-1',
      'spotify_id': 'song-1',
      'name': 'Example Song',
      'artists': 'Example Artist',
      'album_name': 'Example Album',
      'duration_ms': 180000,
      'track_number': 1,
      'genre': 'Example Genre',
      'label': 'Example Label',
      'copyright': 'Example Copyright',
      'comment': 'Example Comment',
      'upc': '0123456789012',
    };
    var albumRequested = false;
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          if (call.method == 'getProviderMetadata') {
            expect(call.arguments, {
              'provider_id': 'example-metadata',
              'resource_type': 'album',
              'resource_id': 'metadata-album',
            });
            albumRequested = true;
            return jsonEncode({
              'track_list': [payload],
              'album_info': {
                'name': 'Example Album',
                'total_tracks': 1,
                'album_type': 'album',
              },
            });
          }
          return null;
        });
    addTearDown(() {
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, null);
    });

    final searchTrack = Track.fromBackendMap(payload);
    expect(searchTrack.genre, 'Example Genre');
    expect(searchTrack.label, 'Example Label');
    expect(searchTrack.copyright, 'Example Copyright');

    await tester.pumpWidget(
      ProviderScope(
        child: MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: const AlbumScreen(
            albumId: 'metadata-album',
            albumName: 'Example Album',
            extensionId: 'example-metadata',
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(albumRequested, isTrue);
    final albumTrack = tester
        .widget<TrackListTile>(find.byType(TrackListTile).first)
        .track;
    expect(albumTrack.id, searchTrack.id);
    expect(albumTrack.name, searchTrack.name);
    expect(albumTrack.genre, searchTrack.genre);
    expect(albumTrack.label, searchTrack.label);
    expect(albumTrack.copyright, searchTrack.copyright);
    expect(albumTrack.comment, searchTrack.comment);
    expect(albumTrack.upc, searchTrack.upc);
    expect(albumTrack.albumId, 'metadata-album');
    expect(albumTrack.albumType, 'album');
    expect(albumTrack.totalTracks, 1);
    await tester.pumpWidget(const SizedBox());
    await tester.pumpAndSettle();
  });
}
