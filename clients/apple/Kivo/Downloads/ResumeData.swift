import Foundation

/// A transfer's resume data pointed at another URL, so it resumes by range
/// from the server's new address instead of the one it was fetching from.
///
/// Resume data is URLSession's own format, which Apple doesn't document,
/// and this is the one place that reads it: a keyed archive (a plain
/// property list before iOS 12) of a dictionary whose
/// `NSURLSessionDownloadURL` is the URL as a string, and whose
/// `NSURLSessionResumeOriginalRequest` and
/// `NSURLSessionResumeCurrentRequest` are each a keyed archive of the
/// request, headers and all. The bytes fetched, the partial file and the
/// entity tag stay as they are: the server is the same, at another address.
/// When the format isn't what this expects, there's nothing to resume, and
/// the transfer starts over.
enum ResumeData {
    private static let urlKey = "NSURLSessionDownloadURL"
    private static let requestKeys = ["NSURLSessionResumeOriginalRequest", "NSURLSessionResumeCurrentRequest"]
    /// The archives' root key: URLSession writes the constant's name, not its value, `root`.
    private static let rootKey = "NSKeyedArchiveRootObjectKey"
    /// What the archives hold. A request is read and written only by secure coding.
    private static let classes: [AnyClass] = [
        NSDictionary.self, NSArray.self, NSString.self, NSNumber.self, NSData.self, NSDate.self, NSURL.self, NSURLRequest.self,
    ]

    /// [data] resuming from [url]: as it is when it already does, rewritten
    /// when it can be, else nil.
    static func pointing(_ data: Data, to url: URL) -> Data? {
        guard let plist = try? PropertyListSerialization.propertyList(from: data, format: nil) as? [String: Any] else { return nil }
        let keyed = plist["$archiver"] != nil
        guard var info = keyed ? unarchived(data) as? [String: Any] : plist else { return nil }
        if info[urlKey] as? String == url.absoluteString { return data }
        var rewritten = 0
        for key in requestKeys {
            guard let blob = info[key] as? Data else { continue }
            guard let request = unarchived(blob) as? NSURLRequest, let changed = request.mutableCopy() as? NSMutableURLRequest else { return nil }
            changed.url = url
            info[key] = archived(changed.copy() as? NSURLRequest ?? changed)
            rewritten += 1
        }
        guard rewritten > 0 else { return nil }
        info[urlKey] = url.absoluteString
        if keyed { return archived(info as NSDictionary) }
        return try? PropertyListSerialization.data(fromPropertyList: info, format: .binary, options: 0)
    }

    /// The URL [data] resumes from, as it says; nil when it can't be read.
    static func url(of data: Data) -> URL? {
        guard let plist = try? PropertyListSerialization.propertyList(from: data, format: nil) as? [String: Any] else { return nil }
        let info = plist["$archiver"] != nil ? unarchived(data) as? [String: Any] : plist
        return (info?[urlKey] as? String).flatMap(URL.init(string:))
    }

    /// The requests [data] resumes with, as it says; empty when they can't be read.
    static func requests(of data: Data) -> [URLRequest] {
        guard let plist = try? PropertyListSerialization.propertyList(from: data, format: nil) as? [String: Any],
              let info = plist["$archiver"] != nil ? unarchived(data) as? [String: Any] : plist else { return [] }
        return requestKeys.compactMap { (info[$0] as? Data).flatMap(unarchived) as? URLRequest }
    }

    private static func unarchived(_ data: Data) -> Any? {
        guard let unarchiver = try? NSKeyedUnarchiver(forReadingFrom: data) else { return nil }
        defer { unarchiver.finishDecoding() }
        return unarchiver.decodeObject(of: classes, forKey: rootKey)
    }

    private static func archived(_ object: Any) -> Data {
        let archiver = NSKeyedArchiver(requiringSecureCoding: true)
        archiver.encode(object, forKey: rootKey)
        archiver.finishEncoding()
        return archiver.encodedData
    }
}
