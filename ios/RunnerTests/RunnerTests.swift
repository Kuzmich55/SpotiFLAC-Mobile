import Foundation
import XCTest
@testable import Runner

class RunnerTests: XCTestCase {
    func testProgressRestartDoesNotAcceptCancelledWaiterState() {
        let oldStarted = expectation(description: "old waiter started")
        let replacementDelivered = expectation(description: "replacement snapshot")
        let replacementAdvanced = expectation(description: "replacement owns cursor")
        let oldReleased = expectation(description: "old waiter returned")
        let releaseOld = DispatchSemaphore(value: 0)
        let lock = NSLock()
        var calls = 0
        var invalidCursors = [Int64]()
        let stream = DownloadProgressSubscription(interval: 0.01) { sequence, _ in
            lock.lock()
            calls += 1
            let call = calls
            if (call <= 2 && sequence != 0) || (call > 2 && sequence != 2) {
                invalidCursors.append(sequence)
            }
            lock.unlock()
            if call == 1 {
                oldStarted.fulfill()
                _ = releaseOld.wait(timeout: .now() + 3)
                oldReleased.fulfill()
                return "{\"seq\":99,\"items\":{}}"
            }
            if call == 2 { return "{\"seq\":2,\"reset\":true,\"items\":{}}" }
            if call == 3 { replacementAdvanced.fulfill() }
            return ""
        }
        stream.start { _ in XCTFail("Cancelled listener received an event") }
        wait(for: [oldStarted], timeout: 2)
        stream.stop()
        stream.start { event in
            XCTAssertEqual((event as? [String: Any])?["seq"] as? Int, 2)
            replacementDelivered.fulfill()
            releaseOld.signal()
        }
        wait(for: [replacementDelivered, oldReleased, replacementAdvanced], timeout: 2)
        stream.stop()
        lock.lock()
        XCTAssertTrue(invalidCursors.isEmpty, "Unexpected cursors: \(invalidCursors)")
        lock.unlock()
    }

    func testParsesOAuthCallback() {
        let route = ExtensionCallbackParser.parse(
            URL(string: "spotiflac://callback?code=auth-code&state=metadata-provider")!
        )

        XCTAssertEqual(
            route,
            ExtensionCallbackRoute(
                code: "auth-code",
                state: "metadata-provider",
                isSessionGrant: false
            )
        )
    }

    func testParsesSignedSessionGrant() {
        let route = ExtensionCallbackParser.parse(
            URL(
                string:
                    "spotiflac://session-grant?grant=session-token&state=provider"
            )!
        )

        XCTAssertEqual(
            route,
            ExtensionCallbackRoute(
                code: "session-token",
                state: "provider",
                isSessionGrant: true
            )
        )
    }

    func testRejectsUntrustedOrIncompleteCallbacks() {
        XCTAssertNil(
            ExtensionCallbackParser.parse(
                URL(string: "https://callback?code=auth&state=provider")!
            )
        )
        XCTAssertNil(
            ExtensionCallbackParser.parse(
                URL(string: "spotiflac://unknown?code=auth&state=provider")!
            )
        )
        XCTAssertNil(
            ExtensionCallbackParser.parse(
                URL(string: "spotiflac://callback?code=auth")!
            )
        )
        XCTAssertNil(
            ExtensionCallbackParser.parse(
                URL(string: "spotiflac://callback?state=provider")!
            )
        )
    }
}
