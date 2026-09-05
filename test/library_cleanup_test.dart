import 'dart:convert';
import 'dart:io';
import 'package:flutter/services.dart';
import 'package:sqflite/sqflite.dart';
import 'package:spotiflac_android/services/library_cleanup.dart';
import 'package:spotiflac_android/utils/file_access.dart';
import 'package:flutter_test/flutter_test.dart';

// Records the paging/compare-and-delete contract without requiring a device DB.
class _CleanupDatabase implements Database, Transaction {
  final rows = <String, Map<String, Object?>>{};
  final keys = <String>{};
  final limits = <int?>[];
  int transactions = 0;
  void add(String id, String path, {String source = 'source'}) {
    rows[id] = {'id': id, 'file_path': path, 'source_id': source};
    keys.add(id);
  }

  @override
  Future<List<Map<String, Object?>>> query(
    String table, {
    bool? distinct,
    List<String>? columns,
    String? where,
    List<Object?>? whereArgs,
    String? groupBy,
    String? having,
    String? orderBy,
    int? limit,
    int? offset,
  }) async {
    expect(offset, isNull);
    limits.add(limit);
    var argsIndex = 0;
    final source = where?.contains('source_id = ?') == true
        ? whereArgs![argsIndex++]
        : null;
    final upper = where?.contains('id <= ?') == true
        ? whereArgs![argsIndex++] as String
        : null;
    final after = where?.contains('id > ?') == true
        ? whereArgs![argsIndex++] as String
        : null;
    final result =
        rows.values.where((row) {
            final id = row['id'] as String;
            return (source == null || row['source_id'] == source) &&
                (upper == null || id.compareTo(upper) <= 0) &&
                (after == null || id.compareTo(after) > 0);
          }).toList()
          ..sort((a, b) => (a['id'] as String).compareTo(b['id'] as String));
    final sorted = orderBy == 'id DESC' ? result.reversed : result;
    return sorted
        .take(limit!)
        .map((row) => {for (final key in columns!) key: row[key]})
        .toList();
  }

  @override
  Future<T> transaction<T>(
    Future<T> Function(Transaction txn) action, {
    bool? exclusive,
  }) {
    transactions++;
    return action(this);
  }

  @override
  Future<int> rawDelete(String sql, [List<Object?>? arguments]) async {
    expect(sql, contains('id = ? AND file_path = ?'));
    final hasSource = sql.contains('source_id = ?');
    final count = arguments!.length - (hasSource ? 1 : 0);
    final ids = <String>[];
    for (var i = 0; i < count; i += 2) {
      final id = arguments[i] as String;
      final row = rows[id];
      if (row != null &&
          row['file_path'] == arguments[i + 1] &&
          (!hasSource || row['source_id'] == arguments.last)) {
        ids.add(id);
      }
    }
    for (final id in ids) {
      if (sql.startsWith('DELETE FROM library_path_keys')) {
        keys.remove(id);
      } else {
        rows.remove(id);
      }
    }
    return ids.length;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('com.zarz.spotiflac/backend');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  tearDown(() => messenger.setMockMethodCallHandler(channel, null));

  test(
    'cleanup pages without skipping after deletions and preserves unknown',
    () async {
      final db = _CleanupDatabase();
      for (var i = 0; i < 700; i++) {
        db.add(i.toString().padLeft(4, '0'), 'file-$i');
      }
      db.add('other', 'file-other', source: 'other');
      final checked = <String>[];
      final removed = await cleanupMissingLibraryRows(
        db,
        sourceId: 'source',
        checkPaths: (paths) async {
          expect(paths.length, lessThanOrEqualTo(256));
          checked.addAll(paths);
          return {
            for (final path in paths)
              path: switch (int.parse(path.split('-').last) % 3) {
                0 => false,
                1 => true,
                _ => null,
              },
          };
        },
      );
      expect(checked.length, 700);
      expect(checked.toSet().length, 700);
      expect(removed, 234);
      expect(db.rows.length, 467);
      expect(db.keys, db.rows.keys.toSet());
      expect(db.rows.containsKey('other'), isTrue);
      expect(db.limits.first, 1);
      expect(db.limits.skip(1), everyElement(256));
    },
  );

  test(
    'cleanup retains repaired paths, newly added rows and unavailable sources',
    () async {
      final db = _CleanupDatabase()
        ..add('a', 'old')
        ..add('b', 'missing');
      final removed = await cleanupMissingLibraryRows(
        db,
        checkPaths: (paths) async {
          db.add('a', 'repaired');
          db.add('z', 'new-row');
          return {for (final path in paths) path: false};
        },
      );
      expect(removed, 1);
      expect(db.keys, {'a', 'z'});
      expect(
        await cleanupMissingLibraryRows(
          db,
          canDelete: () async => false,
          checkPaths: (paths) async => {for (final path in paths) path: false},
        ),
        0,
      );
      expect(
        await cleanupMissingLibraryRows(
          db,
          checkPaths: (_) async => throw const FileSystemException('offline'),
        ),
        0,
      );
      expect(db.keys, {'a', 'z'});
    },
  );

  test(
    'SAF probes use bounded batches and preserve unknown or failed results',
    () async {
      final batches = <List<String>>[];
      messenger.setMockMethodCallHandler(channel, (call) async {
        expect(call.method, 'safExistsBatch');
        final paths =
            (jsonDecode((call.arguments as Map)['uris_json'] as String) as List)
                .cast<String>();
        batches.add(paths);
        if (batches.length == 2) {
          throw PlatformException(code: 'permission-denied');
        }
        return jsonEncode({
          for (final path in paths)
            path: path.endsWith('/0')
                ? 'missing'
                : path.endsWith('/1')
                ? 'unknown'
                : 'found',
        });
      });
      final paths = List.generate(140, (i) => 'content://test/$i');
      final results = await fileExistenceByPath([
        ...paths,
        '${paths.first}#track01',
      ]);
      expect(batches.map((batch) => batch.length), [64, 64, 12]);
      expect(results[paths[0]], isFalse);
      expect(results['${paths.first}#track01'], isFalse);
      expect(results[paths[1]], isNull);
      expect(results[paths[64]], isNull);
      expect(results[paths.last], isTrue);
    },
  );

  test(
    'local probes distinguish present and missing files and keep empty paths unknown',
    () async {
      final dir = await Directory.systemTemp.createTemp('cleanup_probes_');
      addTearDown(() => dir.delete(recursive: true));
      final file = await File('${dir.path}/album.cue').writeAsString('test');
      final absent = '${dir.path}/missing.flac';
      final result = await fileExistenceByPath([
        file.path,
        '${file.path}#track01',
        absent,
        '',
      ]);
      expect(result[file.path], isTrue);
      expect(result['${file.path}#track01'], isTrue);
      expect(result[absent], isFalse);
      expect(result[''], isNull);
    },
  );
}
