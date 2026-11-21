import CoreGraphics
import Foundation

let args = CommandLine.arguments
if args.count < 6 {
    FileHandle.standardError.write("用法: windowid.swift <owner> <x> <y> <width> <height>\n".data(using: .utf8)!)
    exit(1)
}

let owner = args[1]
let x = Double(args[2]) ?? 0
let y = Double(args[3]) ?? 0
let width = Double(args[4]) ?? 0
let height = Double(args[5]) ?? 0

guard let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] else {
    exit(1)
}

for window in windows {
    let layer = window[kCGWindowLayer as String] as? Int ?? -1
    if layer != 0 { continue }
    let windowOwner = window[kCGWindowOwnerName as String] as? String ?? ""
    if windowOwner != owner { continue }
    guard let bounds = window[kCGWindowBounds as String] as? [String: Any] else { continue }
    let bx = (bounds["X"] as? NSNumber)?.doubleValue ?? 0
    let by = (bounds["Y"] as? NSNumber)?.doubleValue ?? 0
    let bw = (bounds["Width"] as? NSNumber)?.doubleValue ?? 0
    let bh = (bounds["Height"] as? NSNumber)?.doubleValue ?? 0
    if abs(bx - x) <= 2 && abs(by - y) <= 2 && abs(bw - width) <= 2 && abs(bh - height) <= 2 {
        print(window[kCGWindowNumber as String] as? Int ?? 0)
        exit(0)
    }
}

exit(1)
