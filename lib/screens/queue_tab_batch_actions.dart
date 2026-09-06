part of 'queue_tab.dart';

extension _QueueTabBatchActions on _QueueTabState {
  List<LocalLibraryItem> _selectedFlacEligibleLocalItems(
    List<UnifiedLibraryItem> allItems,
  ) {
    final selectedItems = _selectedItemsFromAll(allItems);
    return selectedItems
        .map((item) => item.localItem)
        .whereType<LocalLibraryItem>()
        .where(LocalTrackRedownloadService.isFlacUpgradeEligible)
        .toList(growable: false);
  }

  Future<void> _queueSelectedLocalAsFlac(List<UnifiedLibraryItem> allItems) =>
      queueLocalTracksAsFlac(
        context,
        ref,
        _selectedFlacEligibleLocalItems(allItems),
        isActive: () => mounted,
        onComplete: _exitSelectionMode,
      );

  Future<void> _reEnrichSelectedLocalFromQueue(
    List<UnifiedLibraryItem> allItems,
  ) => reEnrichLocalTracks(
    context,
    ref,
    _selectedItemsFromAll(allItems)
        .map((item) => item.localItem)
        .whereType<LocalLibraryItem>()
        .toList(growable: false),
    isActive: () => mounted,
    onSelectionHide: () async {
      _setState(() => _isSelectionMode = false);
      _hideSelectionOverlay();
    },
    onSelectionRestore: () => _setState(() => _isSelectionMode = true),
    onComplete: _exitSelectionMode,
  );

  /// Share selected tracks via system share sheet
  Future<void> _shareSelected(List<UnifiedLibraryItem> allItems) async {
    final itemsById = {for (final item in allItems) item.id: item};
    final safUris = <String>[];
    final filesToShare = <XFile>[];

    for (final id in _selectedIds) {
      final item = itemsById[id];
      if (item == null) continue;
      final path = item.filePath;
      if (isContentUri(path)) {
        if (await fileExists(path)) safUris.add(path);
      } else if (await fileExists(path)) {
        filesToShare.add(XFile(path));
      }
    }

    if (safUris.isEmpty && filesToShare.isEmpty) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.selectionShareNoFiles)),
        );
      }
      return;
    }

    if (safUris.isNotEmpty) {
      try {
        if (safUris.length == 1) {
          await PlatformBridge.shareContentUri(safUris.first);
        } else {
          await PlatformBridge.shareMultipleContentUris(safUris);
        }
      } catch (_) {}
    }

    if (filesToShare.isNotEmpty) {
      await SharePlus.instance.share(ShareParams(files: filesToShare));
    }
  }

  Future<void> _showBatchConvertSheet(
    BuildContext context,
    List<UnifiedLibraryItem> allItems,
  ) {
    return showBatchConvertSheet(
      context,
      ref,
      _selectedItemsFromAll(allItems),
      onExitSelectionMode: _exitSelectionMode,
      onSheetOpen: () {
        _suppressSelectionOverlay = true;
        _hideSelectionOverlay();
        _hidePlaylistSelectionOverlay();
      },
      onSheetClosed: (didStartConversion) async {
        // Wait out the sheet's exit animation before restoring the toolbar so
        // it doesn't pop in front of the still-closing sheet.
        await Future<void>.delayed(const Duration(milliseconds: 260));
        if (!mounted) {
          _suppressSelectionOverlay = false;
          return;
        }
        _suppressSelectionOverlay = false;
        if (didStartConversion) return;
        if (_isSelectionMode) {
          _syncSelectionOverlay(
            items: allItems,
            bottomPadding: MediaQuery.of(this.context).padding.bottom,
          );
        } else if (_isPlaylistSelectionMode) {
          _syncPlaylistSelectionOverlay(
            playlists: ref.read(libraryCollectionsProvider).playlists,
            bottomPadding: MediaQuery.of(this.context).padding.bottom,
          );
        }
      },
    );
  }

  Future<void> _runBatchReplayGain(List<UnifiedLibraryItem> allItems) {
    return runBatchReplayGain(
      context,
      _selectedItemsFromAll(allItems),
      onExitSelectionMode: _exitSelectionMode,
      onConfirmOpen: () {
        _suppressSelectionOverlay = true;
        _hideSelectionOverlay();
        _hidePlaylistSelectionOverlay();
      },
      onConfirmClosed: (confirmed) async {
        if (!mounted) {
          _suppressSelectionOverlay = false;
          return;
        }
        if (confirmed) {
          _suppressSelectionOverlay = false;
          return;
        }
        // Restore after the dialog's exit animation.
        await Future<void>.delayed(const Duration(milliseconds: 220));
        _suppressSelectionOverlay = false;
        if (!mounted) return;
        if (_isSelectionMode) {
          _syncSelectionOverlay(
            items: allItems,
            bottomPadding: MediaQuery.paddingOf(context).bottom,
          );
        }
      },
    );
  }
}
