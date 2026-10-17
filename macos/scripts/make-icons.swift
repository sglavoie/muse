#!/usr/bin/env swift
//
// Renders muse.app's icons into macos/MuseApp/Resources/Assets.xcassets:
//
//   AppIcon.appiconset     the full-color Dock/Finder icon, every mac size
//   StatusIcon.imageset    the monochrome menu-bar template image
//
// The PNGs are committed, so this runs only when the artwork changes:
//
//   swift macos/scripts/make-icons.swift
//
// Everything is drawn from code (no source image), on the macOS icon grid: an
// 824-point rounded square centered in a 1024 canvas. The glyph is a beamed
// pair of eighth notes over a faint waveform: the app plays music. Sizes of 32
// pixels and under drop the waveform, which blurs at that scale. Adapted from
// tt's make-icons.swift.

import AppKit

let scriptURL = URL(fileURLWithPath: CommandLine.arguments[0]).standardizedFileURL
let macosDir = scriptURL.deletingLastPathComponent().deletingLastPathComponent()
let assets = macosDir.appendingPathComponent("MuseApp/Resources/Assets.xcassets")

// MARK: - Drawing helpers

func rgb(_ hex: UInt32, _ alpha: CGFloat = 1) -> CGColor {
    CGColor(
        srgbRed: CGFloat((hex >> 16) & 0xFF) / 255,
        green: CGFloat((hex >> 8) & 0xFF) / 255,
        blue: CGFloat(hex & 0xFF) / 255,
        alpha: alpha)
}

func bitmap(_ pixels: Int, _ draw: (CGContext) -> Void) -> Data {
    let space = CGColorSpace(name: CGColorSpace.sRGB)!
    let ctx = CGContext(
        data: nil, width: pixels, height: pixels, bitsPerComponent: 8, bytesPerRow: 0,
        space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    ctx.interpolationQuality = .high
    ctx.setShouldAntialias(true)
    draw(ctx)
    let rep = NSBitmapImageRep(cgImage: ctx.makeImage()!)
    return rep.representation(using: .png, properties: [:])!
}

/// Two eighth notes joined by a beam, in a 100-unit box with its origin at the
/// bottom left: tilted oval heads, stems up from their right edges, and a
/// slanted beam across the stem tops.
func notesPath() -> CGPath {
    let path = CGMutablePath()
    let stemTops = [CGPoint(x: 37, y: 88), CGPoint(x: 87, y: 98)]
    let radiusX: CGFloat = 15
    let radiusY: CGFloat = 11
    let angle: CGFloat = -0.35
    // Align each tilted oval's rightmost point with the stem's outer edge,
    // and extend the stem to that tangent so the join has no protruding lip.
    let rightExtent = sqrt(pow(radiusX * cos(angle), 2) + pow(radiusY * sin(angle), 2))
    let tangentY = (radiusX * radiusX - radiusY * radiusY) * sin(angle) * cos(angle) / rightExtent
    let heads = zip(stemTops, [CGFloat(18), CGFloat(28)]).map { top, y in
        CGPoint(x: top.x - rightExtent, y: y)
    }
    for center in heads {
        let tilt = CGAffineTransform(translationX: center.x, y: center.y).rotated(by: angle)
        path.addEllipse(
            in: CGRect(x: -radiusX, y: -radiusY, width: 2 * radiusX, height: 2 * radiusY),
            transform: tilt)
    }
    let stem: CGFloat = 6
    let beamThickness: CGFloat = 14
    for (head, top) in zip(heads, stemTops) {
        let bottom = head.y + tangentY
        // End inside the beam so the flat stem cap cannot protrude above
        // its slanted upper edge.
        let stemTop = top.y - beamThickness / 2
        path.addRect(CGRect(x: top.x - stem, y: bottom, width: stem, height: stemTop - bottom))
    }
    // The beam: a parallelogram 14 units thick from the first stem top to the
    // second, wound counterclockwise like the rects so the overlaps stay filled.
    path.move(to: CGPoint(x: stemTops[0].x - stem, y: stemTops[0].y - beamThickness))
    path.addLine(to: CGPoint(x: stemTops[1].x, y: stemTops[1].y - beamThickness))
    path.addLine(to: CGPoint(x: stemTops[1].x, y: stemTops[1].y))
    path.addLine(to: CGPoint(x: stemTops[0].x - stem, y: stemTops[0].y))
    path.closeSubpath()
    return path
}

/// The notes scaled to `height` and centered in `rect`.
func notes(height: CGFloat, in rect: CGRect, dx: CGFloat = 0, dy: CGFloat = 0) -> CGPath {
    let scale = height / 100
    var fit = CGAffineTransform(scaleX: scale, y: scale)
    let path = notesPath().copy(using: &fit)!
    let box = path.boundingBoxOfPath
    var move = CGAffineTransform(
        translationX: rect.midX - box.midX + dx, y: rect.midY - box.midY + dy)
    return path.copy(using: &move)!
}

// MARK: - App icon

/// Draws the app icon on a 1024 canvas scaled to `pixels`.
func appIcon(pixels: Int) -> Data {
    bitmap(pixels) { ctx in
        let scale = CGFloat(pixels) / 1024
        ctx.scaleBy(x: scale, y: scale)
        let detailed = pixels > 32

        // Tile: 824 on the 1024 grid, corner radius 185, with a soft drop shadow.
        let tile = CGRect(x: 100, y: 100, width: 824, height: 824)
        let shape = CGPath(roundedRect: tile, cornerWidth: 185, cornerHeight: 185, transform: nil)
        ctx.saveGState()
        ctx.setShadow(offset: CGSize(width: 0, height: -10), blur: 28, color: rgb(0x000000, 0.35))
        ctx.addPath(shape)
        ctx.setFillColor(rgb(0x1E1A2E))
        ctx.fillPath()
        ctx.restoreGState()

        ctx.saveGState()
        ctx.addPath(shape)
        ctx.clip()
        let gradient = CGGradient(
            colorsSpace: CGColorSpace(name: CGColorSpace.sRGB),
            colors: [rgb(0x4A3D70), rgb(0x1E1A2E)] as CFArray, locations: [0, 1])!
        ctx.drawLinearGradient(
            gradient, start: CGPoint(x: 512, y: 924), end: CGPoint(x: 512, y: 100), options: [])
        // A faint waveform of rounded bars along the bottom, read as "audio".
        if detailed {
            let bars: [CGFloat] = [40, 90, 150, 70, 120, 200, 110, 60, 140, 80, 170, 50]
            let width: CGFloat = 34
            let gap: CGFloat = 26
            let total = CGFloat(bars.count) * width + CGFloat(bars.count - 1) * gap
            var x = tile.midX - total / 2
            ctx.setFillColor(rgb(0xFFFFFF, 0.08))
            for height in bars {
                let bar = CGRect(x: x, y: 180, width: width, height: height)
                ctx.addPath(CGPath(roundedRect: bar, cornerWidth: width / 2, cornerHeight: width / 2, transform: nil))
                x += width + gap
            }
            ctx.fillPath()
        }
        ctx.restoreGState()

        // Hairline edge so the tile holds on light and dark backgrounds.
        ctx.addPath(shape)
        ctx.setStrokeColor(rgb(0xFFFFFF, 0.10))
        ctx.setLineWidth(4)
        ctx.strokePath()

        // The notes, larger at small sizes where they are the whole icon.
        let glyph = detailed
            ? notes(height: 470, in: tile, dy: 40)
            : notes(height: 600, in: tile)
        ctx.addPath(glyph)
        ctx.setFillColor(rgb(0xF2C6E0))
        ctx.fillPath()
    }
}

// MARK: - Status icon

/// The menu-bar template: the beamed notes, black on clear. AppKit tints them
/// for light, dark, and highlighted menu bars.
func statusIcon(pixels: Int) -> Data {
    bitmap(pixels) { ctx in
        let scale = CGFloat(pixels) / 18
        ctx.scaleBy(x: scale, y: scale)
        ctx.addPath(notes(height: 15, in: CGRect(x: 0, y: 0, width: 18, height: 18)))
        ctx.setFillColor(rgb(0x000000))
        ctx.fillPath()
    }
}

// MARK: - Output

func write(_ data: Data, _ url: URL) {
    try! data.write(to: url, options: .atomic)
}

func writeJSON(_ object: Any, _ url: URL) {
    let data = try! JSONSerialization.data(
        withJSONObject: object, options: [.prettyPrinted, .sortedKeys])
    write(data + Data("\n".utf8), url)
}

let fm = FileManager.default

let appIconDir = assets.appendingPathComponent("AppIcon.appiconset")
try! fm.createDirectory(at: appIconDir, withIntermediateDirectories: true)
var appImages: [[String: String]] = []
for points in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let name = scale == 1 ? "icon_\(points)x\(points).png" : "icon_\(points)x\(points)@2x.png"
        write(appIcon(pixels: points * scale), appIconDir.appendingPathComponent(name))
        appImages.append([
            "filename": name, "idiom": "mac", "scale": "\(scale)x", "size": "\(points)x\(points)",
        ])
    }
}
writeJSON(
    ["images": appImages, "info": ["author": "xcode", "version": 1]],
    appIconDir.appendingPathComponent("Contents.json"))

let statusDir = assets.appendingPathComponent("StatusIcon.imageset")
try! fm.createDirectory(at: statusDir, withIntermediateDirectories: true)
var statusImages: [[String: String]] = []
for scale in [1, 2] {
    let name = scale == 1 ? "status_18.png" : "status_18@2x.png"
    write(statusIcon(pixels: 18 * scale), statusDir.appendingPathComponent(name))
    statusImages.append(["filename": name, "idiom": "mac", "scale": "\(scale)x"])
}
writeJSON(
    [
        "images": statusImages, "info": ["author": "xcode", "version": 1],
        "properties": ["template-rendering-intent": "template"],
    ],
    statusDir.appendingPathComponent("Contents.json"))

print("Wrote \(appIconDir.path) and \(statusDir.path)")
