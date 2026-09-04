import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/utils/extension_auth_launcher.dart';

void main() {
  test('verification challenge expires after three minutes', () {
    expect(extensionVerificationGrantTimeout, const Duration(minutes: 3));
  });

  test('extracts the extension that raised a verification challenge', () {
    expect(
      extensionIdFromVerificationError(
        "verification_required: extension 'tidal-web' needs verification",
        const ['amazon-web', 'tidal-web'],
      ),
      'tidal-web',
    );
  });

  test('prefers the longest known extension id in legacy errors', () {
    expect(
      extensionIdFromVerificationError(
        'qobuz-web verification_required',
        const ['qobuz', 'qobuz-web'],
      ),
      'qobuz-web',
    );
  });

  test(
    'retries an extension operation after successful verification',
    () async {
      var operationCalls = 0;
      var verificationCalls = 0;

      final result = await runExtensionOperationWithVerificationRetry(
        extensionId: 'qobuz-web',
        browserMode: 'in_app_first',
        operation: () async {
          operationCalls++;
          if (operationCalls == 1) throw Exception('VERIFY_REQUIRED');
          return 'artist metadata';
        },
        verify: () async {
          verificationCalls++;
          return true;
        },
      );

      expect(result, 'artist metadata');
      expect(operationCalls, 2);
      expect(verificationCalls, 1);
    },
  );

  test('does not retry when extension verification is not completed', () async {
    var operationCalls = 0;

    await expectLater(
      runExtensionOperationWithVerificationRetry(
        extensionId: 'qobuz-web',
        browserMode: 'in_app_first',
        operation: () async {
          operationCalls++;
          throw Exception('VERIFY_REQUIRED');
        },
        verify: () async => false,
      ),
      throwsA(isA<Exception>()),
    );

    expect(operationCalls, 1);
  });

  test('verification wait can be cancelled before foreground resume', () async {
    final foreground = Completer<void>();
    final cancellation = Completer<void>();
    final result = openVerificationAndAwaitGrant(
      'tidal-web',
      browserMode: 'in_app_first',
      awaitForeground: (_) => foreground.future,
      cancellationSignal: cancellation.future,
    );

    cancellation.complete();

    expect(await result.timeout(const Duration(seconds: 1)), isFalse);
  });
}
