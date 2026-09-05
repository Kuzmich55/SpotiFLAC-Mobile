import 'dart:async';
import 'dart:io';
import 'package:spotiflac_android/utils/cache_byte_budget.dart';
import 'package:spotiflac_android/utils/periodic_async_task.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  testWidgets(
    'maintenance repeats without overlapping and survives pause/resume and failure',
    (tester) async {
      final pending = <Completer<void>>[];
      var errors = 0;
      final task = PeriodicAsyncTask(
        interval: const Duration(minutes: 15),
        run: () {
          final done = Completer<void>();
          pending.add(done);
          return done.future;
        },
        onError: (_, _) => errors++,
      );
      addTearDown(task.stop);
      task.start(delay: const Duration(seconds: 20));
      task.start(delay: Duration.zero);
      await tester.pump(const Duration(seconds: 19));
      expect(pending, isEmpty);
      await tester.pump(const Duration(seconds: 1));
      expect(pending, hasLength(1));
      await tester.pump(const Duration(minutes: 30));
      expect(pending, hasLength(1));
      task.stop();
      task.start(delay: Duration.zero);
      await tester.pump(const Duration(milliseconds: 1));
      expect(pending, hasLength(1));
      pending[0].complete();
      await tester.pump();
      expect(pending, hasLength(2));
      pending[1].completeError(StateError('temporary I/O error'));
      await tester.pump();
      expect(errors, 1);
      await tester.pump(const Duration(minutes: 15));
      expect(pending, hasLength(3));
      task.stop();
      pending[2].complete();
      await tester.pump(const Duration(hours: 1));
      expect(pending, hasLength(3));
    },
  );

  test(
    'cache budget trims oldest payloads while retaining metadata and new files',
    () async {
      final directory = await Directory.systemTemp.createTemp(
        'cover-budget-test-',
      );
      addTearDown(() => directory.delete(recursive: true));
      Future<File> file(String name, int minutesOld) async {
        final file = File('${directory.path}/$name');
        await file.writeAsBytes(List.filled(100, 0));
        await file.setLastModified(
          DateTime.now().subtract(Duration(minutes: minutesOld)),
        );
        return file;
      }

      final oldest = await file('old-cover', 60);
      final newer = await file('newer-cover', 10);
      final active = await file('active-cover', 0);
      final metadata = await file('cache.json', 120);
      expect(
        await trimCacheToByteBudget(directory, maxBytes: 350, targetBytes: 150),
        0,
      );
      expect(
        await trimCacheToByteBudget(directory, maxBytes: 250, targetBytes: 150),
        2,
      );
      expect(await oldest.exists(), isFalse);
      expect(await newer.exists(), isFalse);
      expect(await active.exists(), isTrue);
      expect(await metadata.exists(), isTrue);
    },
  );
}
