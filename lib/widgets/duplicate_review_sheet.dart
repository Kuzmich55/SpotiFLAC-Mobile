import 'dart:async';

import 'package:flutter/material.dart';
import 'package:spotiflac_android/widgets/app_bottom_sheet.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path/path.dart' as p;
import 'package:spotiflac_android/l10n/l10n.dart';
import 'package:spotiflac_android/providers/download_history_provider.dart';
import 'package:spotiflac_android/providers/local_library_provider.dart';
import 'package:spotiflac_android/services/downloaded_embedded_cover_resolver.dart';
import 'package:spotiflac_android/services/library_database.dart';
import 'package:spotiflac_android/utils/duplicate_cleanup_policy.dart';
import 'package:spotiflac_android/utils/file_access.dart';
import 'package:spotiflac_android/utils/path_match_keys.dart';
import 'package:spotiflac_android/widgets/audio_quality_badges.dart';
import 'package:spotiflac_android/widgets/settings_group.dart';

/// Bottom sheet listing tracks that exist more than once (same ISRC) across
/// download history and the local library, with keep-best and per-copy
/// delete actions.
class DuplicateReviewSheet extends ConsumerStatefulWidget {
  const DuplicateReviewSheet({super.key});

  static Future<void> show(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    return showAppBottomSheet<void>(
      context: context,
      useRootNavigator: true,
      backgroundColor: colorScheme.surfaceContainerHigh,
      title: context.l10n.duplicatesTitle,
      maxHeightFactor: 0.85,
      builder: (_) => const DuplicateReviewSheet(),
    );
  }

  @override
  ConsumerState<DuplicateReviewSheet> createState() =>
      _DuplicateReviewSheetState();
}

class _DuplicateReviewSheetState extends ConsumerState<DuplicateReviewSheet> {
  late final LocalLibraryNotifier _localLibraryNotifier;
  late Future<List<IsrcDuplicateGroup>> _groupsFuture;
  bool _deletedLocalRows = false;
  bool _deleteActionInProgress = false;

  @override
  void initState() {
    super.initState();
    _localLibraryNotifier = ref.read(localLibraryProvider.notifier);
    _groupsFuture = LibraryDatabase.instance.findIsrcDuplicateGroups();
  }

  @override
  void dispose() {
    if (_deletedLocalRows) {
      unawaited(_localLibraryNotifier.reloadFromStorage());
    }
    super.dispose();
  }

  void _reload() {
    setState(() {
      _groupsFuture = LibraryDatabase.instance.findIsrcDuplicateGroups();
    });
  }

  String _qualityLabel(IsrcDuplicateEntry entry) {
    var format = entry.format?.trim().toUpperCase() ?? '';
    if (format.isEmpty) {
      format = p
          .extension(
            DownloadedEmbeddedCoverResolver.cleanFilePath(entry.filePath),
          )
          .replaceFirst('.', '')
          .toUpperCase();
    }
    final bitDepth = entry.bitDepth ?? 0;
    final sampleRate = entry.sampleRate ?? 0;
    if (bitDepth > 0 && sampleRate > 0) {
      final khz = sampleRate / 1000;
      final khzText = khz % 1 == 0 ? khz.toInt().toString() : khz.toString();
      return format.isEmpty
          ? '$bitDepth/$khzText'
          : '$bitDepth/$khzText $format';
    }
    final bitrate = entry.bitrate ?? 0;
    if (bitrate > 0) {
      return '${(bitrate / 1000).round()}k${format.isEmpty ? '' : ' $format'}';
    }
    return format;
  }

  Future<bool> _confirmDelete(String message) async {
    final confirmed = await showDialog<bool>(
      context: context,
      useRootNavigator: true,
      builder: (ctx) => AlertDialog(
        title: Text(ctx.l10n.dialogDeleteSelectedTitle),
        content: Text(message),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(ctx.l10n.dialogCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: FilledButton.styleFrom(
              backgroundColor: Theme.of(ctx).colorScheme.error,
            ),
            child: Text(ctx.l10n.dialogDelete),
          ),
        ],
      ),
    );
    return confirmed == true && mounted;
  }

  Future<void> _deleteEntries(
    List<IsrcDuplicateEntry> entries, {
    required List<IsrcDuplicateEntry> retainedEntries,
  }) async {
    final historyNotifier = ref.read(downloadHistoryProvider.notifier);
    var deleted = 0;
    for (final entry in entries) {
      final entryPath = DownloadedEmbeddedCoverResolver.cleanFilePath(
        entry.filePath,
      );
      final sharesRetainedFile = isPhysicalFileRetained(
        entryPath,
        retainedEntries.map(
          (retained) =>
              DownloadedEmbeddedCoverResolver.cleanFilePath(retained.filePath),
        ),
      );
      // Two database rows may describe one SAF file through different URI or
      // raw-path aliases. Remove only the redundant row in that case; deleting
      // the shared file would also destroy the copy the user chose to retain.
      final fileDeleted = sharesRetainedFile || await deleteFile(entryPath);
      if (!fileDeleted) continue;
      if (entry.source == 'downloaded') {
        historyNotifier.removeFromHistory(entry.id);
      } else {
        await LibraryDatabase.instance.deleteByPath(entry.filePath);
        _deletedLocalRows = true;
      }
      deleted++;
    }
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(context.l10n.snackbarDeletedTracks(deleted))),
    );
    _reload();
  }

  Future<void> _confirmAndDelete({
    required String message,
    required List<IsrcDuplicateEntry> entries,
    required List<IsrcDuplicateEntry> retainedEntries,
  }) async {
    if (_deleteActionInProgress || entries.isEmpty) return;
    setState(() => _deleteActionInProgress = true);
    try {
      final confirmed = await _confirmDelete(message);
      if (confirmed) {
        await _deleteEntries(entries, retainedEntries: retainedEntries);
      }
    } finally {
      if (mounted) {
        setState(() => _deleteActionInProgress = false);
      }
    }
  }

  Future<void> _keepBest(IsrcDuplicateGroup group) async {
    final toDelete = group.entries.sublist(1);
    await _confirmAndDelete(
      message: context.l10n.duplicatesKeepBestMessage(
        toDelete.length,
        group.entries.first.trackName,
      ),
      entries: toDelete,
      retainedEntries: [group.entries.first],
    );
  }

  Future<void> _keepBestAll(List<IsrcDuplicateGroup> groups) async {
    final plan = buildKeepBestAllDuplicatePlan(groups);
    await _confirmAndDelete(
      message: context.l10n.duplicatesKeepBestAllMessage(
        plan.entriesToDelete.length,
        plan.groupCount,
      ),
      entries: plan.entriesToDelete,
      retainedEntries: plan.retainedEntries,
    );
  }

  Future<void> _deleteSingle(
    IsrcDuplicateGroup group,
    IsrcDuplicateEntry entry,
  ) async {
    await _confirmAndDelete(
      message: context.l10n.duplicatesDeleteCopyMessage(entry.trackName),
      entries: [entry],
      retainedEntries: group.entries
          .where(
            (candidate) =>
                candidate.source != entry.source || candidate.id != entry.id,
          )
          .toList(growable: false),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    return FutureBuilder<List<IsrcDuplicateGroup>>(
      future: _groupsFuture,
      builder: (context, snapshot) {
        if (snapshot.connectionState != ConnectionState.done) {
          return const Padding(
            padding: EdgeInsets.all(32),
            child: Center(child: CircularProgressIndicator()),
          );
        }
        final groups = snapshot.data ?? const [];
        if (groups.isEmpty) {
          return Padding(
            padding: const EdgeInsets.fromLTRB(24, 8, 24, 32),
            child: Text(
              context.l10n.duplicatesEmpty,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: colorScheme.onSurfaceVariant,
              ),
            ),
          );
        }
        // shrinkWrap keeps a small sheet compact but builds every
        // group eagerly; past a screenful, switch to a lazy
        // full-height list so a large duplicate set can't jank the
        // opening frame.
        final compact = groups.length <= 12;
        return ListView.builder(
          shrinkWrap: compact,
          itemCount: groups.length + 1,
          itemBuilder: (context, index) {
            if (index == 0) {
              return _buildKeepBestAllAction(context, colorScheme, groups);
            }
            return _buildGroup(context, colorScheme, groups[index - 1]);
          },
        );
      },
    );
  }

  Widget _buildKeepBestAllAction(
    BuildContext context,
    ColorScheme colorScheme,
    List<IsrcDuplicateGroup> groups,
  ) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
      child: SizedBox(
        width: double.infinity,
        child: FilledButton.icon(
          onPressed: _deleteActionInProgress
              ? null
              : () => _keepBestAll(groups),
          style: FilledButton.styleFrom(
            backgroundColor: colorScheme.error,
            foregroundColor: colorScheme.onError,
          ),
          icon: _deleteActionInProgress
              ? SizedBox.square(
                  dimension: 18,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: colorScheme.onError,
                  ),
                )
              : const Icon(Icons.delete_sweep_outlined),
          label: Text(context.l10n.duplicatesKeepBestAll),
        ),
      ),
    );
  }

  Widget _buildGroup(
    BuildContext context,
    ColorScheme colorScheme,
    IsrcDuplicateGroup group,
  ) {
    final first = group.entries.first;
    return SettingsGroup(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 8, 4),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      first.trackName,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.titleSmall?.copyWith(
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '${first.artistName} • ${group.isrc}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: colorScheme.onSurfaceVariant,
                      ),
                    ),
                  ],
                ),
              ),
              TextButton(
                onPressed: _deleteActionInProgress
                    ? null
                    : () => _keepBest(group),
                child: Text(context.l10n.duplicatesKeepBest),
              ),
            ],
          ),
        ),
        for (var i = 0; i < group.entries.length; i++)
          ListTile(
            dense: true,
            title: Row(
              children: [
                AudioQualityBadge(
                  label: _qualityLabel(group.entries[i]),
                  colorScheme: colorScheme,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    p.basename(
                      DownloadedEmbeddedCoverResolver.cleanFilePath(
                        group.entries[i].filePath,
                      ),
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ),
              ],
            ),
            trailing: i == 0
                ? Icon(Icons.check_circle_outline, color: colorScheme.primary)
                : IconButton(
                    tooltip: context.l10n.dialogDelete,
                    icon: Icon(Icons.delete_outline, color: colorScheme.error),
                    onPressed: _deleteActionInProgress
                        ? null
                        : () => _deleteSingle(group, group.entries[i]),
                  ),
          ),
      ],
    );
  }
}
