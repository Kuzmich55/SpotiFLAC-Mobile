import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/l10n/app_localizations.dart';
import 'package:spotiflac_android/models/settings.dart';
import 'package:spotiflac_android/providers/extension_provider.dart';
import 'package:spotiflac_android/providers/settings_provider.dart';
import 'package:spotiflac_android/widgets/download_service_picker.dart';

class _Extensions extends ExtensionNotifier {
  @override
  ExtensionState build() => ExtensionState(
    extensions: [
      Extension(
        id: 'example',
        name: 'example',
        displayName: 'Example',
        version: '1.0.0',
        description: '',
        enabled: true,
        status: 'loaded',
        hasDownloadProvider: true,
        qualityOptions: List.generate(
          16,
          (index) => QualityOption(id: '$index', label: 'Quality $index'),
        ),
      ),
    ],
  );

  @override
  void refreshEnabledExtensionHealth({bool force = false}) {}
}

class _Settings extends SettingsNotifier {
  @override
  AppSettings build() => const AppSettings();
}

void main() {
  testWidgets('iOS picker scrolls options and dismisses when pulled down', (
    tester,
  ) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          extensionProvider.overrideWith(_Extensions.new),
          settingsProvider.overrideWith(_Settings.new),
        ],
        child: MaterialApp(
          theme: ThemeData(platform: TargetPlatform.iOS),
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: Scaffold(
            body: Builder(
              builder: (context) => TextButton(
                onPressed: () => DownloadServicePicker.show(
                  context,
                  trackName: 'Example track',
                  onSelect: (_, _) {},
                ),
                child: const Text('Open'),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    final scroll = find.descendant(
      of: find.byType(DownloadServicePicker),
      matching: find.byType(SingleChildScrollView),
    );
    await tester.drag(scroll, const Offset(0, -300));
    await tester.pumpAndSettle();
    expect(find.byType(DownloadServicePicker), findsOneWidget);
    await tester.drag(scroll, const Offset(0, 1500));
    await tester.pumpAndSettle();
    await tester.drag(find.text('Example track'), const Offset(0, 600));
    await tester.pumpAndSettle();
    expect(find.byType(DownloadServicePicker), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
