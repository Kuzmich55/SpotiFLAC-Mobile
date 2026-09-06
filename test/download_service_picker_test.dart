import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/l10n/app_localizations.dart';
import 'package:spotiflac_android/models/settings.dart';
import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/providers/extension_provider.dart';
import 'package:spotiflac_android/providers/settings_provider.dart';
import 'package:spotiflac_android/widgets/download_service_picker.dart';

class _PickerExtensions extends ExtensionNotifier {
  @override
  ExtensionState build() => const ExtensionState(
    extensions: [
      Extension(
        id: 'provider-a',
        name: 'provider-a',
        displayName: 'Audio A',
        version: '1.0.0',
        description: '',
        enabled: true,
        status: 'loaded',
        hasDownloadProvider: true,
        qualityOptions: [
          QualityOption(id: 'LOSSLESS', label: 'Lossless'),
          QualityOption(id: 'HI_RES', label: 'Hi-Res'),
          QualityOption(id: 'HI_RES_LOSSLESS', label: 'Hi-Res'),
        ],
      ),
      Extension(
        id: 'provider-b',
        name: 'provider-b',
        displayName: 'Audio B',
        version: '1.0.0',
        description: '',
        enabled: true,
        status: 'loaded',
        hasDownloadProvider: true,
        qualityOptions: [
          QualityOption(id: 'opus_256', label: 'Opus 256kbps'),
          QualityOption(id: 'custom', label: 'Custom'),
        ],
      ),
    ],
  );
}

class _PickerSettings extends SettingsNotifier {
  final AppSettings _settings;

  _PickerSettings(this._settings);

  @override
  AppSettings build() => _settings;
}

Future<void> _openPicker(
  WidgetTester tester, {
  Duration? duration,
  String? audioQuality = '16bit/44.1kHz',
  String? source = 'provider-a',
  AppSettings settings = const AppSettings(),
  Locale locale = const Locale('en'),
  double textScale = 1,
  void Function(String, String)? onSelect,
}) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        extensionProvider.overrideWith(_PickerExtensions.new),
        settingsProvider.overrideWith(() => _PickerSettings(settings)),
      ],
      child: MaterialApp(
        locale: locale,
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        builder: (context, child) => MediaQuery(
          data: MediaQuery.of(
            context,
          ).copyWith(textScaler: TextScaler.linear(textScale)),
          child: child!,
        ),
        home: Scaffold(
          body: Builder(
            builder: (context) => TextButton(
              onPressed: () => DownloadServicePicker.show(
                context,
                trackName: 'Selected tracks',
                tracks: [
                  Track(
                    id: 'sample',
                    name: 'Sample',
                    artistName: 'Artist',
                    albumName: 'Album',
                    duration: duration?.inSeconds ?? 0,
                    source: source,
                    audioQuality: audioQuality,
                  ),
                ],
                onSelect: onSelect ?? (_, _) {},
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
}

void main() {
  testWidgets('estimates change with the provider and selection stays intact', (
    tester,
  ) async {
    (String, String)? selected;
    await _openPicker(
      tester,
      duration: const Duration(minutes: 4),
      onSelect: (quality, service) => selected = (quality, service),
    );
    expect(find.text('≈ 26.2 MB'), findsNWidgets(3));
    expect(find.textContaining('MB–'), findsNothing);
    await tester.tap(find.text('Audio B'));
    await tester.pumpAndSettle();
    expect(find.text('≈ 7.3 MB'), findsOneWidget);
    expect(find.text('Size estimate unavailable'), findsOneWidget);
    expect(find.text('≈ 26.2 MB'), findsNothing);
    expect(find.textContaining('if 24-bit'), findsNothing);
    await tester.tap(find.text('Opus 256kbps'));
    await tester.pumpAndSettle();
    expect(selected, ('opus_256', 'provider-b'));
    expect(find.byType(DownloadServicePicker), findsNothing);
  });

  testWidgets(
    'missing duration stays unknown and downloads remain selectable',
    (tester) async {
      await _openPicker(tester, locale: const Locale('id'));
      expect(find.text('Estimasi ukuran belum tersedia'), findsNWidgets(3));
      expect(find.textContaining('≈'), findsNothing);
      await tester.tap(find.text('FLAC Lossless'));
      await tester.pumpAndSettle();
      expect(find.byType(DownloadServicePicker), findsNothing);
    },
  );

  testWidgets('metadata caps both hi-res tiers to the available sample rate', (
    tester,
  ) async {
    await _openPicker(
      tester,
      duration: const Duration(minutes: 4),
      audioQuality: '24bit/96kHz',
      locale: const Locale('id'),
    );
    expect(find.text('≈ 85.7 MB'), findsNWidgets(2));
    expect(find.text('≈ 26.2 MB'), findsOneWidget);
    expect(find.textContaining('171.4'), findsNothing);
  });

  testWidgets('foreign or incomplete track quality stays unavailable', (
    tester,
  ) async {
    await _openPicker(
      tester,
      duration: const Duration(minutes: 4),
      source: 'provider-b',
    );
    expect(find.text('Size estimate unavailable'), findsNWidgets(3));
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pumpAndSettle();
    await _openPicker(
      tester,
      duration: const Duration(minutes: 4),
      audioQuality: '24bit',
    );
    expect(find.text('Size estimate unavailable'), findsNWidgets(3));
  });

  testWidgets(
    'conversion estimate is separate and fits narrow, enlarged text',
    (tester) async {
      tester.view.physicalSize = const Size(320, 720);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await _openPicker(
        tester,
        duration: const Duration(minutes: 4),
        textScale: 1.8,
        settings: const AppSettings(
          autoConvertDownloads: true,
          autoConvertFormat: 'opus',
          autoConvertBitrate: '256k',
        ),
      );
      expect(find.text('≈ 26.2 MB'), findsNWidgets(3));
      final conversionNote = find.textContaining(
        'After conversion to OPUS: ≈ 7.3 MB',
      );
      expect(conversionNote, findsOneWidget);
      await tester.ensureVisible(conversionNote);
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
    },
  );
}
