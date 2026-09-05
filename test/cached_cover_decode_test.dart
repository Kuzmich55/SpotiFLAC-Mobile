import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:spotiflac_android/widgets/cached_cover_image.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  for (final explicit in [false, true]) {
    testWidgets(
      'grid decode follows constraints, explicit override=$explicit',
      (tester) async {
        await tester.pumpWidget(
          MaterialApp(
            home: MediaQuery(
              data: const MediaQueryData(devicePixelRatio: 2),
              child: Center(
                child: SizedBox.square(
                  dimension: 180,
                  child: CachedCoverImage(
                    imageUrl: 'https://example.invalid/cover.png',
                    memCacheWidth: explicit ? 1200 : null,
                    errorWidget: (_, _, _) => const SizedBox(),
                  ),
                ),
              ),
            ),
          ),
        );
        final image = tester.widget<CachedNetworkImage>(
          find.byType(CachedNetworkImage),
        );
        expect(image.memCacheWidth, explicit ? 1200 : 360);
        expect(image.memCacheHeight, isNull);
        expect(image.maxWidthDiskCache, isNull);
        expect(
          tester.getSize(find.byType(CachedCoverImage)),
          const Size(180, 180),
        );
        await tester.pumpWidget(const SizedBox());
      },
    );
  }
}
