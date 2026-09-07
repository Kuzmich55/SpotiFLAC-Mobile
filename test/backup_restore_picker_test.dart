import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/l10n/app_localizations.dart';
import 'package:spotiflac_android/screens/settings/backup_restore_page.dart';

class _BackupFilePicker extends FilePickerPlatform {
  bool opened = false;

  @override
  Future<PlatformFile?> pickFile({
    String? dialogTitle,
    String? initialDirectory,
    FileType type = FileType.any,
    List<String>? allowedExtensions,
    dynamic Function(FilePickerStatus)? onFileLoading,
    int compressionQuality = 0,
    AndroidOptions androidOptions = const AndroidOptions(),
    DarwinOptions darwinOptions = const DarwinOptions(),
    WindowsOptions windowsOptions = const WindowsOptions(),
    LinuxOptions linuxOptions = const LinuxOptions(),
    WebOptions webOptions = const WebOptions(),
  }) async {
    opened = true;
    // A custom JSON/SFLB filter becomes JSON-only on Android because SFLB
    // has no system MIME mapping.
    expect(type, FileType.any);
    expect(allowedExtensions, isNull);
    return null;
  }
}

void main() {
  testWidgets('restore does not exclude backups with an unknown MIME type', (
    tester,
  ) async {
    final original = FilePickerPlatform.instance;
    final picker = _BackupFilePicker();
    FilePickerPlatform.instance = picker;
    addTearDown(() => FilePickerPlatform.instance = original);
    await tester.pumpWidget(
      const ProviderScope(
        child: MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: BackupRestorePage(),
        ),
      ),
    );
    final restore = find.text('Choose backup file');
    await tester.ensureVisible(restore);
    await tester.tap(restore);
    await tester.pumpAndSettle();
    expect(picker.opened, isTrue);
    expect(find.byType(AlertDialog), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
