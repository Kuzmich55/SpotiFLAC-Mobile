import Foundation

/// Each listener owns its cursor and pending waiter. Lifecycle and delivery run
/// on the main queue; a cancelled wait may finish, but can only touch its own state.
final class DownloadProgressSubscription {
    typealias Waiter = (Int64, Int64) -> String
    private final class Session {
        let queue = DispatchQueue(label: "com.zarz.spotiflac.download_progress_subscription", qos: .utility)
        private let lock = NSLock()
        private var cancelled = false
        // Accessed only on this session's queue.
        var sequence: Int64 = 0
        var lastPayload: String?

        func cancel() {
            lock.lock()
            cancelled = true
            lock.unlock()
        }

        var isCancelled: Bool {
            lock.lock()
            defer { lock.unlock() }
            return cancelled
        }
    }

    private let waiter: Waiter
    private let interval: TimeInterval
    private var current: Session?

    init(interval: TimeInterval = 0.25, waiter: @escaping Waiter) {
        self.interval = interval
        self.waiter = waiter
    }

    func start(_ receive: @escaping (Any) -> Void) {
        dispatchPrecondition(condition: .onQueue(.main))
        stop()
        let session = Session()
        current = session
        poll(session, receive: receive)
    }

    func stop() {
        dispatchPrecondition(condition: .onQueue(.main))
        current?.cancel()
        current = nil
    }

    deinit { current?.cancel() }

    private func poll(_ session: Session, receive: @escaping (Any) -> Void) {
        let waiter = self.waiter
        session.queue.async { [weak self] in
            guard !session.isCancelled else { return }
            let payload = waiter(session.sequence, 15_000)
            guard !session.isCancelled else { return }
            if !payload.isEmpty && payload != session.lastPayload,
               let data = payload.data(using: .utf8),
               let object = try? JSONSerialization.jsonObject(with: data, options: [.fragmentsAllowed]),
               let delta = object as? [String: Any],
               let sequence = delta["seq"] as? NSNumber {
                session.sequence = max(session.sequence, sequence.int64Value)
                session.lastPayload = payload
                DispatchQueue.main.async { [weak self] in
                    guard let self, self.current === session, !session.isCancelled else { return }
                    receive(object)
                }
            }
            // Schedule from main so subscription identity is never accessed
            // concurrently, and stop/relisten does not wait for an old Go call.
            DispatchQueue.main.async { [weak self] in
                guard let self, self.current === session, !session.isCancelled else { return }
                DispatchQueue.main.asyncAfter(deadline: .now() + self.interval) { [weak self] in
                    guard let self, self.current === session, !session.isCancelled else { return }
                    self.poll(session, receive: receive)
                }
            }
        }
    }
}
