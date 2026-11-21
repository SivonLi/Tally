import CoreGraphics
import Foundation
import ImageIO

struct WindowFrame: Decodable {
    let x: Double
    let y: Double
    let width: Double
    let height: Double
}

struct TextNode: Decodable {
    let role: String
    let text: String
    let x: Double
    let y: Double
    let width: Double
    let height: Double
}

struct Snapshot: Decodable {
    let window: WindowFrame
    let nodes: [TextNode]
}

struct Finding {
    let ratio: Double
    let threshold: Double
    let node: TextNode
    let x: Double
    let y: Double
}

func fail(_ message: String) -> Never {
    FileHandle.standardError.write((message + "\n").data(using: .utf8)!)
    exit(1)
}

func linearize(_ value: Double) -> Double {
    if value <= 0.03928 {
        return value / 12.92
    }
    return pow((value + 0.055) / 1.055, 2.4)
}

func contrastRatio(_ first: Double, _ second: Double) -> Double {
    let light = max(first, second)
    let dark = min(first, second)
    return (light + 0.05) / (dark + 0.05)
}

func histogram(of image: CGImage) -> (bins: [Int], count: Int)? {
    let width = image.width
    let height = image.height
    if width <= 0 || height <= 0 {
        return nil
    }
    var pixels = [UInt8](repeating: 255, count: width * height * 4)
    let colorSpace = CGColorSpaceCreateDeviceRGB()
    let bitmapInfo =
        CGBitmapInfo.byteOrder32Big.rawValue |
        CGImageAlphaInfo.premultipliedLast.rawValue
    guard let context = CGContext(
        data: &pixels,
        width: width,
        height: height,
        bitsPerComponent: 8,
        bytesPerRow: width * 4,
        space: colorSpace,
        bitmapInfo: bitmapInfo
    ) else {
        return nil
    }
    context.interpolationQuality = .none
    context.draw(image, in: CGRect(x: 0, y: 0, width: width, height: height))
    var bins = [Int](repeating: 0, count: 256)
    var index = 0
    while index < pixels.count {
        let red = Double(pixels[index]) / 255
        let green = Double(pixels[index + 1]) / 255
        let blue = Double(pixels[index + 2]) / 255
        let luminance =
            0.2126 * linearize(red) +
            0.7152 * linearize(green) +
            0.0722 * linearize(blue)
        let bucket = min(255, max(0, Int(luminance * 255)))
        bins[bucket] += 1
        index += 4
    }
    return (bins, width * height)
}

func percentile(_ bins: [Int], _ total: Int, _ position: Double) -> Double {
    let target = max(1, Int(Double(total) * position))
    var accumulated = 0
    for bucket in 0..<bins.count {
        accumulated += bins[bucket]
        if accumulated >= target {
            return Double(bucket) / 255
        }
    }
    return 1
}

func meanLuminance(_ bins: [Int], _ total: Int) -> Double {
    var sum = 0.0
    for bucket in 0..<bins.count {
        sum += Double(bucket) / 255 * Double(bins[bucket])
    }
    return sum / Double(total)
}

func variance(_ bins: [Int], _ total: Int, _ mean: Double) -> Double {
    var sum = 0.0
    for bucket in 0..<bins.count {
        let value = Double(bucket) / 255
        sum += (value - mean) * (value - mean) * Double(bins[bucket])
    }
    return sum / Double(total)
}

func clamp(_ value: Double, _ low: Double, _ high: Double) -> Double {
    return min(max(value, low), high)
}

func displayText(_ text: String) -> String {
    let single = text.replacingOccurrences(of: "\n", with: " ")
    if single.count <= 36 {
        return single
    }
    return String(single.prefix(36)) + "..."
}

func sampleRect(
    _ image: CGImage,
    x: Double,
    y: Double,
    width: Double,
    height: Double
) -> (bins: [Int], count: Int)? {
    let left = clamp(floor(x), 0, Double(image.width - 1))
    let top = clamp(floor(y), 0, Double(image.height - 1))
    let right = clamp(ceil(x + width), left + 1, Double(image.width))
    let bottom = clamp(ceil(y + height), top + 1, Double(image.height))
    let rect = CGRect(
        x: left,
        y: top,
        width: right - left,
        height: bottom - top
    )
    guard let crop = image.cropping(to: rect) else {
        return nil
    }
    return histogram(of: crop)
}

func check(
    pngPath: String,
    jsonPath: String
) {
    guard
        let source = CGImageSourceCreateWithURL(
            URL(fileURLWithPath: pngPath) as CFURL,
            nil
        ),
        let image = CGImageSourceCreateImageAtIndex(source, 0, nil)
    else {
        fail("无法读取截图: \(pngPath)")
    }
    let data: Data
    do {
        data = try Data(contentsOf: URL(fileURLWithPath: jsonPath))
    } catch {
        fail("无法读取控件快照: \(jsonPath)")
    }
    let snapshot: Snapshot
    do {
        snapshot = try JSONDecoder().decode(Snapshot.self, from: data)
    } catch {
        fail("控件快照格式错误: \(error)")
    }
    let frame = snapshot.window
    if frame.width <= 0 || frame.height <= 0 {
        fail("窗口尺寸无效")
    }
    guard let full = histogram(of: image) else {
        fail("无法解析截图像素")
    }
    let fullMean = meanLuminance(full.bins, full.count)
    let fullVariance = variance(full.bins, full.count, fullMean)
    let scaleX = Double(image.width) / frame.width
    let scaleY = Double(image.height) / frame.height

    print("截图: \(pngPath)")
    print("窗口: \(Int(frame.width))x\(Int(frame.height)) 点")
    print(
        String(
            format: "像素: %dx%d (%.2fx)",
            image.width,
            image.height,
            scaleX
        )
    )
    if abs(scaleX - scaleY) > 0.05 {
        print(
            String(
                format: "尺寸异常: 横向 %.2fx，纵向 %.2fx，窗口可能在截图时移动",
                scaleX,
                scaleY
            )
        )
    }
    if fullVariance < 0.0001 {
        print(
            String(
                format: "纯色检查: 失败，画面方差 %.5f，可能是屏幕录制权限缺失或者窗口没有绘制",
                fullVariance
            )
        )
    } else {
        print(String(format: "纯色检查: 通过，画面方差 %.5f", fullVariance))
    }

    var inside = 0
    var skipped = 0
    var findings: [Finding] = []
    for node in snapshot.nodes {
        let relativeX = node.x - frame.x
        let relativeY = node.y - frame.y
        let fullyVisible =
            relativeX >= -1 && relativeY >= -1 &&
            relativeX + node.width <= frame.width + 1 &&
            relativeY + node.height <= frame.height + 1
        if !fullyVisible {
            skipped += 1
            continue
        }
        var pixelX = relativeX * scaleX
        var pixelY = relativeY * scaleY
        var pixelWidth = node.width * scaleX
        var pixelHeight = node.height * scaleY
        let inset = 1.0 * scaleX
        if pixelWidth > inset * 2 + 2 && pixelHeight > inset * 2 + 2 {
            pixelX += inset
            pixelY += inset
            pixelWidth -= inset * 2
            pixelHeight -= inset * 2
        }
        if pixelWidth < 4 || pixelHeight < 4 {
            skipped += 1
            continue
        }
        guard
            let sample = sampleRect(
                image,
                x: pixelX,
                y: pixelY,
                width: pixelWidth,
                height: pixelHeight
            )
        else {
            skipped += 1
            continue
        }
        inside += 1
        let background = percentile(sample.bins, sample.count, 0.5)
        let low = percentile(sample.bins, sample.count, 0.02)
        let high = percentile(sample.bins, sample.count, 0.98)
        let foreground =
            abs(high - background) >= abs(background - low) ? high : low
        let ratio = contrastRatio(background, foreground)
        let threshold = node.height >= 22 ? 3.0 : 4.5
        if ratio < threshold {
            findings.append(
                Finding(
                    ratio: ratio,
                    threshold: threshold,
                    node: node,
                    x: relativeX,
                    y: relativeY
                )
            )
        }
    }
    print(
        "文字检查: 窗口内 \(inside) 个静态文本，跳过 \(skipped) 个（超出窗口或者尺寸过小）"
    )
    findings.sort { $0.ratio < $1.ratio }
    if findings.isEmpty {
        print("对比度: 未发现低于阈值的文字")
    } else {
        print("对比度候选（从低到高，需要对照截图确认）:")
        for finding in findings.prefix(40) {
            print(
                String(
                    format: "  %.2f  阈值 %.1f  %@  \"%@\"  @ %.0f,%.0f %.0fx%.0f",
                    finding.ratio,
                    finding.threshold,
                    finding.node.role,
                    displayText(finding.node.text),
                    finding.x,
                    finding.y,
                    finding.node.width,
                    finding.node.height
                )
            )
        }
        if findings.count > 40 {
            print("  其余 \(findings.count - 40) 处未列出")
        }
        print("结论: \(findings.count) 处文字对比度低于阈值")
    }
}

func selfTest() {
    var pixels = [UInt8](repeating: 0, count: 8 * 8 * 4)
    for y in 0..<4 {
        for x in 0..<4 {
            let index = (y * 8 + x) * 4
            pixels[index] = 255
            pixels[index + 1] = 255
            pixels[index + 2] = 255
            pixels[index + 3] = 255
        }
    }
    guard
        let provider = CGDataProvider(data: Data(pixels) as CFData),
        let image = CGImage(
            width: 8,
            height: 8,
            bitsPerComponent: 8,
            bitsPerPixel: 32,
            bytesPerRow: 32,
            space: CGColorSpaceCreateDeviceRGB(),
            bitmapInfo: CGBitmapInfo(
                rawValue: CGImageAlphaInfo.noneSkipLast.rawValue
            ),
            provider: provider,
            decode: nil,
            shouldInterpolate: false,
            intent: .defaultIntent
        )
    else {
        fail("自检失败: 无法建立测试图片")
    }
    guard
        let crop = image.cropping(
            to: CGRect(x: 0, y: 0, width: 4, height: 4)
        ),
        let sample = histogram(of: crop)
    else {
        fail("自检失败: 无法裁剪测试图片")
    }
    let mean = meanLuminance(sample.bins, sample.count)
    if mean < 0.9 {
        fail("自检失败: 截图坐标原点不在左上角")
    }
    let ratio = contrastRatio(0, 1)
    if abs(ratio - 21) > 0.01 {
        fail("自检失败: 对比度公式错误")
    }
    print("自检通过")
}

let arguments = CommandLine.arguments
if arguments.count == 2 && arguments[1] == "--selftest" {
    selfTest()
    exit(0)
}
if arguments.count != 3 {
    fail("用法: visualcheck.swift <截图.png> <控件快照.json>")
}
check(pngPath: arguments[1], jsonPath: arguments[2])
