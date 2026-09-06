import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:spotiflac_android/services/notification_service.dart';
import 'package:spotiflac_android/services/verification_notification.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  AndroidFlutterLocalNotificationsPlugin.registerWith();

  const channel = MethodChannel('dexterous.com/flutter/local_notifications');
  final methodCalls = <MethodCall>[];
  const coldTarget = VerificationNotification(
    extensionId: 'provider-a',
    itemId: 'cold-source-item',
    tapId: 'dart:cold-tap',
  );

  setUp(() {
    debugDefaultTargetPlatformOverride = TargetPlatform.android;
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          methodCalls.add(call);
          if (call.method == 'initialize') return true;
          if (call.method == 'getNotificationAppLaunchDetails') {
            return {
              'notificationLaunchedApp': true,
              'notificationResponse': {
                'notificationId': 4,
                'notificationResponseType': 0,
                'payload': coldTarget.encode(),
              },
            };
          }
          return null;
        });
  });

  tearDown(() {
    debugDefaultTargetPlatformOverride = null;
    methodCalls.clear();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, null);
  });

  test('verification uses a distinct audible Android alert channel', () async {
    final notificationService = NotificationService();
    await notificationService.showVerificationRequired(
      extensionId: 'provider-b',
      itemId: 'source-item',
    );

    final alertChannelCall = methodCalls.firstWhere(
      (call) =>
          call.method == 'createNotificationChannel' &&
          (call.arguments as Map<Object?, Object?>)['id'] ==
              NotificationService.alertChannelId,
    );
    final alertChannel = alertChannelCall.arguments as Map<Object?, Object?>;

    expect(alertChannel['importance'], Importance.defaultImportance.value);
    expect(alertChannel['playSound'], isTrue);
    expect(alertChannel['enableVibration'], isTrue);

    final showCall = methodCalls.lastWhere((call) => call.method == 'show');
    final showArguments = showCall.arguments as Map<Object?, Object?>;
    final androidDetails =
        showArguments['platformSpecifics'] as Map<Object?, Object?>;

    expect(
      NotificationService.verificationRequiredId,
      isNot(NotificationService.downloadProgressId),
    );
    expect(showArguments['id'], NotificationService.verificationRequiredId);
    expect(androidDetails['channelId'], NotificationService.alertChannelId);
    expect(androidDetails['importance'], Importance.defaultImportance.value);
    expect(androidDetails['playSound'], isTrue);
    expect(androidDetails['enableVibration'], isTrue);
    final target = VerificationNotification.parse(showArguments['payload']);
    expect(target?.extensionId, 'provider-b');
    expect(target?.itemId, 'source-item');

    final tapped = <VerificationNotification>[];
    notificationService.verificationNotifications.setHandler((target) async {
      tapped.add(target);
    });
    expect(tapped.single.extensionId, 'provider-a');
    expect(tapped.single.itemId, 'cold-source-item');
    await TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .handlePlatformMessage(
          channel.name,
          const StandardMethodCodec().encodeMethodCall(
            MethodCall('didReceiveNotificationResponse', {
              'notificationResponseType': 0,
              'id': 4,
              'payload': showArguments['payload'],
            }),
          ),
          (_) {},
        );
    expect(tapped.map((target) => target.extensionId), [
      'provider-a',
      'provider-b',
    ]);
    notificationService.verificationNotifications.setHandler(null);

    await notificationService.cancelVerificationRequired();
    expect(
      methodCalls.last,
      isMethodCall(
        'cancel',
        arguments: <String, Object?>{
          'id': NotificationService.verificationRequiredId,
          'tag': null,
        },
      ),
    );
  });
}
