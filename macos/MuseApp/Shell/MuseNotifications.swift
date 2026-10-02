import AppKit
import UserNotifications

/// Shows the track-change banners muse asks for over the control socket, as
/// muse.app's own notifications rather than alerter's. muse's
/// `notify_on_track_change` decides whether a notify event is sent at all;
/// the app's toggle (`MuseNotificationsPreference`) can only suppress one. A
/// port of tt.app's NotificationPresenter, without its task actions.
final class MuseNotifications: NSObject, UNUserNotificationCenterDelegate {
    /// Whether to show one at all; read per notification.
    var isEnabled: () -> Bool = { true }
    /// Whether muse's window is visible in the frontmost app, where the
    /// track change is already on screen.
    var isMuseInFront: () -> Bool = { false }
    /// Brings muse's window forward; run when a banner is clicked.
    var onActivate: (() -> Void)?

    static let threadIdentifier = "muse.track"

    private lazy var center: UNUserNotificationCenter = {
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        return center
    }()
    private var authorizationRequested = false

    /// Whether a notify event becomes a banner.
    static func shouldPost(enabled: Bool, museInFront: Bool) -> Bool {
        enabled && !museInFront
    }

    /// The banner one notify event becomes: the title, the artist beneath it,
    /// and muse's line about the track (its album) as the body.
    static func content(for notification: MuseNotification) -> UNMutableNotificationContent {
        let content = UNMutableNotificationContent()
        content.title = notification.title.isEmpty ? "muse" : notification.title
        content.subtitle = notification.subtitle ?? ""
        content.body = notification.body ?? ""
        content.threadIdentifier = threadIdentifier
        return content
    }

    /// A notify event from the control socket.
    func post(_ notification: MuseNotification) {
        guard Self.shouldPost(enabled: isEnabled(), museInFront: isMuseInFront()) else { return }
        let content = Self.content(for: notification)
        let center = self.center
        let deliver = {
            let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
            center.add(request) { error in
                if let error { NSLog("muse: notification: %@", error.localizedDescription) }
            }
        }
        if authorizationRequested {
            deliver()
            return
        }
        // Asked on the first banner rather than at launch, so a user who never
        // turns muse's track notifications on is never prompted.
        authorizationRequested = true
        center.requestAuthorization(options: [.alert]) { granted, error in
            if let error { NSLog("muse: notification authorization: %@", error.localizedDescription) }
            if granted { deliver() }
        }
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        DispatchQueue.main.async { [weak self] in
            let inFront = self?.isMuseInFront() ?? false
            completionHandler(inFront ? [] : [.banner, .list])
        }
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let action = response.actionIdentifier
        DispatchQueue.main.async { [weak self] in
            self?.respond(to: action)
            completionHandler()
        }
    }

    func respond(to action: String) {
        if action == UNNotificationDefaultActionIdentifier { onActivate?() }
    }
}
