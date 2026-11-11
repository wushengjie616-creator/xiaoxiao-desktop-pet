import AppKit
import ImageIO
import QuartzCore

enum Pose: String, CaseIterable {
    case resting, sitting, watching, grooming, belly, tail, sleeping, walking, sprinting, spinning, scaling

    var title: String {
        switch self {
        case .resting: return "趴着"
        case .sitting: return "坐着侧耳"
        case .watching: return "抬头张望"
        case .grooming: return "低头梳毛"
        case .belly: return "躺着伸爪"
        case .tail: return "背身摆尾"
        case .sleeping: return "侧躺睡觉"
        case .walking: return "一直慢慢走"
        case .sprinting: return "小小冲刺"
        case .spinning: return "俯视转圈"
        case .scaling: return "变大变小"
        }
    }

    var next: Pose {
        let poses = Self.allCases
        return poses[(poses.firstIndex(of: self)! + 1) % poses.count]
    }

    static let active: [Pose] = [.sitting, .watching, .resting, .grooming, .tail, .belly, .spinning, .scaling]
}

struct MotionClip {
    let frames: [CGImage]
    let frameStarts: [NSNumber]
    let duration: Double

    private static func source(for pose: Pose) -> CGImageSource? {
        guard let url = Bundle.main.url(forResource: pose.rawValue, withExtension: "png") else { return nil }
        return CGImageSourceCreateWithURL(url as CFURL, nil)
    }

    static func firstFrame(for pose: Pose) -> CGImage? {
        guard let source = source(for: pose) else { return nil }
        return CGImageSourceCreateImageAtIndex(source, 0, nil)
    }

    init?(pose: Pose) {
        guard let source = Self.source(for: pose),
              CGImageSourceGetCount(source) > 1 else { return nil }
        var images: [CGImage] = []
        var starts: [Double] = [0]
        var elapsed = 0.0
        for index in 0..<CGImageSourceGetCount(source) {
            guard let image = CGImageSourceCreateImageAtIndex(source, index, nil) else { return nil }
            let properties = CGImageSourceCopyPropertiesAtIndex(source, index, nil) as? [CFString: Any]
            let png = properties?[kCGImagePropertyPNGDictionary] as? [CFString: Any]
            let delay = png?[kCGImagePropertyAPNGUnclampedDelayTime] as? Double
                ?? png?[kCGImagePropertyAPNGDelayTime] as? Double ?? 0.08
            images.append(image)
            elapsed += max(delay, 0.025)
            starts.append(elapsed)
        }
        frames = images
        duration = elapsed
        frameStarts = starts.map { NSNumber(value: $0 / elapsed) }
    }
}

final class CatView: NSView {
    var onClick: (() -> Void)?
    private let picture = CALayer()
    private var clip: MotionClip?
    private var mouseStart = NSPoint.zero
    private var windowStart = NSPoint.zero
    private var dragged = false
    private(set) var isHolding = false

    override init(frame: NSRect) {
        super.init(frame: frame)
        wantsLayer = true
        picture.anchorPoint = CGPoint(x: 0.5, y: 0)
        picture.contentsGravity = .resizeAspect
        picture.contentsScale = 2
        layer?.addSublayer(picture)
        NSWorkspace.shared.notificationCenter.addObserver(
            self, selector: #selector(updateMotion),
            name: NSWorkspace.accessibilityDisplayOptionsDidChangeNotification, object: nil
        )
        updateMotion()
    }

    required init?(coder: NSCoder) { nil }

    override func layout() {
        super.layout()
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        picture.bounds = bounds.insetBy(dx: 4, dy: 4)
        picture.position = CGPoint(x: bounds.midX, y: 4)
        CATransaction.commit()
    }

    func show(_ clip: MotionClip) {
        self.clip = clip
        setPicture(clip.frames[0])
        updateMotion()
    }

    func showStill(_ image: CGImage) {
        clip = nil
        picture.removeAnimation(forKey: "motion")
        setPicture(image)
    }

    private func setPicture(_ image: CGImage) {
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        picture.contents = image
        CATransaction.commit()
    }

    func faceRight(_ right: Bool) {
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        picture.setAffineTransform(CGAffineTransform(scaleX: right ? -1 : 1, y: 1))
        CATransaction.commit()
    }

    @objc private func updateMotion() {
        picture.removeAnimation(forKey: "motion")
        guard !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion, let clip else { return }
        let motion = CAKeyframeAnimation(keyPath: "contents")
        motion.values = clip.frames + [clip.frames[0]]
        motion.keyTimes = clip.frameStarts
        motion.calculationMode = .discrete
        motion.duration = clip.duration
        motion.repeatCount = .infinity
        picture.add(motion, forKey: "motion")
    }

    override func acceptsFirstMouse(for event: NSEvent?) -> Bool { true }

    override func mouseDown(with event: NSEvent) {
        isHolding = true
        mouseStart = window?.convertPoint(toScreen: event.locationInWindow) ?? NSEvent.mouseLocation
        windowStart = window?.frame.origin ?? .zero
        dragged = false
    }

    override func mouseDragged(with event: NSEvent) {
        let current = window?.convertPoint(toScreen: event.locationInWindow) ?? NSEvent.mouseLocation
        let dx = current.x - mouseStart.x
        let dy = current.y - mouseStart.y
        guard dragged || hypot(dx, dy) >= 4 else { return }
        dragged = true
        window?.setFrameOrigin(NSPoint(x: windowStart.x + dx, y: windowStart.y + dy))
    }

    override func mouseUp(with event: NSEvent) {
        isHolding = false
        if !dragged { onClick?() }
    }
}

final class Pet: NSObject, NSApplicationDelegate {
    private let sizes = [120, 180, 240, 300, 360]
    private let catView = CatView(frame: .zero)
    private var currentClip: MotionClip?
    private var activeIndex = 0
    private var nextActivePoseAt = 0.0
    private var panel: NSPanel!
    private var statusItem: NSStatusItem!
    private var timer: Timer?
    private var walkTimer: Timer?
    private var lastWalkTick = 0.0
    private var walkDirection: CGFloat = -1
    private var walkX: CGFloat?
    private var edgeRestUntil: TimeInterval?
    private let edgeRestDuration: TimeInterval = 2
    private lazy var edgeRestImage = MotionClip.firstFrame(for: .sitting)
    // Must match STRIDE in render_walk.py, in original pixels per APNG cycle.
    private let walkStride: CGFloat = 72
    private let sprintStride: CGFloat = 210
    private var pose = Pose.resting
    private var automatic = true
    private var size = 240

    func applicationDidFinishLaunching(_ notification: Notification) {
        let savedSize = UserDefaults.standard.integer(forKey: "petSize")
        if sizes.contains(savedSize) { size = savedSize }

        panel = NSPanel(contentRect: .zero,
                        styleMask: [.borderless, .nonactivatingPanel],
                        backing: .buffered, defer: false)
        panel.title = "小小"
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = false
        panel.level = .floating
        panel.hidesOnDeactivate = false
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .ignoresCycle]
        panel.contentView = catView
        catView.onClick = { [weak self] in
            guard let self else { return }
            self.automatic = false
            self.show(self.pose.next)
        }

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem.button?.title = "小小"
        statusItem.button?.toolTip = "小小桌宠"
        updateAutomaticPose()
        resetPosition()
        panel.orderFrontRegardless()

        timer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            self?.updateAutomaticPose()
        }
    }

    private func show(_ next: Pose) {
        let loaded: MotionClip?
        if pose == next, let currentClip {
            loaded = currentClip
        } else {
            loaded = MotionClip(pose: next)
        }
        guard let loaded else {
            let alert = NSAlert()
            alert.messageText = "小小的动画无法打开"
            alert.informativeText = "请保留完整的「小小.app」应用。"
            alert.runModal()
            NSApp.terminate(nil)
            return
        }
        let needsAnimation = currentClip == nil || pose != next
        currentClip = loaded
        pose = next
        let image = loaded.frames[0]
        let ratio = CGFloat(size) / CGFloat(max(image.width, image.height))
        let dimensions = NSSize(width: CGFloat(image.width) * ratio + 8,
                                height: CGFloat(image.height) * ratio + 8)
        walkX = nil
        let previous = panel.frame
        let proposed = NSRect(x: previous.midX - dimensions.width / 2, y: previous.minY,
                              width: dimensions.width, height: dimensions.height)
        let screen = panel.screen ?? NSScreen.main
        if let screen {
            let visible = screen.visibleFrame
            let x = max(visible.minX, min(proposed.minX, visible.maxX - proposed.width))
            let y = max(visible.minY, min(proposed.minY, visible.maxY - proposed.height))
            panel.setFrame(NSRect(origin: NSPoint(x: x, y: y), size: dimensions), display: true)
        } else {
            panel.setFrame(proposed, display: true)
        }
        if needsAnimation {
            catView.show(loaded)
            if pose == .walking || pose == .sprinting {
                startWalking()
            } else {
                walkTimer?.invalidate()
                walkTimer = nil
                edgeRestUntil = nil
                catView.faceRight(false)
            }
        }
        rebuildMenu()
    }

    private func startWalking() {
        walkTimer?.invalidate()
        walkDirection = -1
        edgeRestUntil = nil
        catView.faceRight(false)
        lastWalkTick = ProcessInfo.processInfo.systemUptime
        let timer = Timer(timeInterval: 1.0 / 30.0, repeats: true) { [weak self] _ in
            self?.advanceWalk()
        }
        walkTimer = timer
        RunLoop.main.add(timer, forMode: .common)
    }

    private func advanceWalk() {
        let now = ProcessInfo.processInfo.systemUptime
        let elapsed = min(now - lastWalkTick, 0.15)
        lastWalkTick = now
        guard !catView.isHolding, !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion else {
            walkX = nil
            return
        }
        guard let clip = currentClip, let screen = panel.screen ?? NSScreen.main else { return }
        if let deadline = edgeRestUntil {
            guard now >= deadline else { return }
            edgeRestUntil = nil
            walkDirection = -walkDirection
            catView.faceRight(walkDirection > 0)
            catView.show(clip)
            return
        }
        let frame = panel.frame
        let left = screen.visibleFrame.minX + 12
        let right = screen.visibleFrame.maxX - frame.width - 12
        guard left < right else { return }
        let framePixels = CGFloat(max(clip.frames[0].width, clip.frames[0].height))
        let stride = pose == .sprinting ? sprintStride : walkStride
        let distance = elapsed * stride * CGFloat(size) / (framePixels * clip.duration)
        let nextX = (walkX ?? frame.minX) + distance * walkDirection
        let reachedEdge = (nextX <= left && walkDirection < 0)
            || (nextX >= right && walkDirection > 0)
        let clamped = min(right, max(left, nextX))
        walkX = clamped
        panel.setFrameOrigin(NSPoint(x: clamped, y: frame.minY))
        if reachedEdge {
            edgeRestUntil = now + edgeRestDuration
            catView.faceRight(false)
            catView.showStill(edgeRestImage ?? clip.frames[0])
        }
    }

    private func rebuildMenu() {
        let menu = NSMenu()
        let auto = menu.addItem(withTitle: "自动切换姿态", action: #selector(enableAutomatic), keyEquivalent: "")
        auto.target = self
        auto.state = automatic ? .on : .off
        for pose in Pose.allCases {
            let item = menu.addItem(withTitle: pose.title, action: #selector(selectPose(_:)), keyEquivalent: "")
            item.target = self
            item.representedObject = pose.rawValue
            item.state = !automatic && self.pose == pose ? .on : .off
        }

        menu.addItem(.separator())
        let scale = NSMenu()
        for dimension in sizes {
            let item = scale.addItem(withTitle: "\(dimension) 点", action: #selector(selectSize(_:)), keyEquivalent: "")
            item.target = self
            item.tag = dimension
            item.state = size == dimension ? .on : .off
        }
        menu.addItem(withTitle: "大小", action: nil, keyEquivalent: "").submenu = scale
        menu.addItem(withTitle: "移回右下角", action: #selector(resetPosition), keyEquivalent: "").target = self
        menu.addItem(.separator())
        menu.addItem(withTitle: "退出小小", action: #selector(quit), keyEquivalent: "q").target = self
        statusItem.menu = menu
        catView.menu = menu.copy() as? NSMenu
    }

    private func updateAutomaticPose() {
        guard automatic else { return }
        let idle = CGEventSource.secondsSinceLastEventType(.combinedSessionState,
                                                           eventType: .init(rawValue: UInt32.max)!)
        let next: Pose
        switch idle {
        case 300...: next = .sleeping
        case 90...: next = .resting
        default:
            let now = ProcessInfo.processInfo.systemUptime
            if now >= nextActivePoseAt {
                next = Pose.active[activeIndex]
                activeIndex = (activeIndex + 1) % Pose.active.count
                nextActivePoseAt = now + 16
            } else {
                next = Pose.active[(activeIndex + Pose.active.count - 1) % Pose.active.count]
            }
        }
        if currentClip == nil || next != pose { show(next) }
    }

    @objc private func enableAutomatic() {
        automatic = true
        nextActivePoseAt = 0
        updateAutomaticPose()
        rebuildMenu()
    }

    @objc private func selectPose(_ item: NSMenuItem) {
        guard let name = item.representedObject as? String, let pose = Pose(rawValue: name) else { return }
        automatic = false
        show(pose)
    }

    @objc private func selectSize(_ item: NSMenuItem) {
        size = item.tag
        UserDefaults.standard.set(size, forKey: "petSize")
        show(pose)
    }

    @objc private func resetPosition() {
        guard let screen = NSScreen.main else { return }
        let visible = screen.visibleFrame
        walkX = nil
        panel.setFrameOrigin(NSPoint(x: visible.maxX - panel.frame.width - 20, y: visible.minY + 20))
    }

    @objc private func quit() { NSApp.terminate(nil) }
}

let app = NSApplication.shared
let pet = Pet()
app.setActivationPolicy(.accessory)
app.delegate = pet
app.run()
