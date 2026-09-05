import 'dart:async';
import 'dart:convert';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:spotiflac_android/providers/track_provider.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'latest search wins and A-B-A never reuses a cancelled request',
    () async {
      const channel = MethodChannel('com.zarz.spotiflac/backend');
      final messenger =
          TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      final requests = <(String, String, Completer<String>)>[];
      final cancellations = <String>[];
      messenger.setMockMethodCallHandler(channel, (call) async {
        final arguments = call.arguments as Map;
        if (call.method == 'cancelExtensionRequest') {
          cancellations.add(arguments['request_id'] as String);
          return null;
        }
        if (call.method == 'customSearchWithExtension') {
          final result = Completer<String>();
          requests.add((
            arguments['query'] as String,
            arguments['request_id'] as String,
            result,
          ));
          return result.future;
        }
        return null;
      });
      addTearDown(() => messenger.setMockMethodCallHandler(channel, null));
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final notifier = container.read(trackProvider.notifier);
      final first = notifier.customSearch('regression-search-provider', 'A');
      await Future<void>.delayed(Duration.zero);
      final second = notifier.customSearch('regression-search-provider', 'B');
      await Future<void>.delayed(Duration.zero);
      final third = notifier.customSearch('regression-search-provider', 'A');
      await Future<void>.delayed(Duration.zero);
      expect(requests.map((r) => r.$1), ['A', 'B', 'A']);
      expect(cancellations, containsAll([requests[0].$2, requests[1].$2]));
      String response(String id) => jsonEncode([
        {
          'id': id,
          'name': id,
          'artist_name': 'Artist',
          'album_name': 'Album',
          'duration': 1000,
        },
      ]);
      requests[2].$3.complete(response('latest'));
      await third;
      requests[0].$3.complete(response('old-A'));
      requests[1].$3.complete(response('old-B'));
      await Future.wait([first, second]);
      expect(container.read(trackProvider).tracks.single.id, 'latest');
      expect(container.read(trackProvider).isLoading, isFalse);
    },
  );
}
