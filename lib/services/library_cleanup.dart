import 'package:sqflite/sqflite.dart';
import 'package:spotiflac_android/utils/file_access.dart';

/// Pages by stable ID (not OFFSET, since rows are removed during traversal).
/// A high-water mark bounds the run if a concurrent scan adds new rows.
Future<int> cleanupMissingLibraryRows(
  Database db, {
  String? sourceId,
  Future<bool> Function()? canDelete,
  Future<Map<String, bool?>> Function(List<String>) checkPaths =
      fileExistenceByPath,
}) async {
  const pageSize = 256;
  final sourceWhere = sourceId == null ? null : 'source_id = ?';
  final sourceArgs = sourceId == null ? null : <Object?>[sourceId];
  final last = await db.query(
    'library',
    columns: ['id'],
    where: sourceWhere,
    whereArgs: sourceArgs,
    orderBy: 'id DESC',
    limit: 1,
  );
  if (last.isEmpty) return 0;
  final upperId = last.single['id'] as String;
  String? cursor;
  var removed = 0;
  while (true) {
    final rows = await db.query(
      'library',
      columns: ['id', 'file_path'],
      where: [
        ?sourceWhere,
        'id <= ?',
        if (cursor != null) 'id > ?',
      ].join(' AND '),
      whereArgs: [?sourceId, upperId, ?cursor],
      orderBy: 'id ASC',
      limit: pageSize,
    );
    if (rows.isEmpty) break;
    cursor = rows.last['id'] as String;
    final paths = rows.map((row) => row['file_path'] as String).toList();
    Map<String, bool?> checks;
    try {
      checks = await checkPaths(paths);
    } catch (_) {
      continue; // No deletion on a failed batch.
    }
    final missing = rows
        .where((row) => checks[row['file_path']] == false)
        .toList();
    if (missing.isEmpty) continue;
    if (canDelete != null) {
      try {
        if (!await canDelete()) break;
      } catch (_) {
        break;
      }
    }
    // Compare the original path as well: a scan may repair it while I/O runs.
    final conditions = List.filled(
      missing.length,
      '(id = ? AND file_path = ?)',
    ).join(' OR ');
    final where =
        '($conditions)${sourceWhere == null ? '' : ' AND $sourceWhere'}';
    final args = <Object?>[
      for (final row in missing) ...[row['id'], row['file_path']],
      ?sourceId,
    ];
    removed += await db.transaction((txn) async {
      await txn.rawDelete(
        'DELETE FROM library_path_keys WHERE item_id IN '
        '(SELECT id FROM library WHERE $where)',
        args,
      );
      return txn.rawDelete('DELETE FROM library WHERE $where', args);
    });
  }
  return removed;
}
