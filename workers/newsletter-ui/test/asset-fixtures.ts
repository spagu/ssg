// Tiny image headers for the campaign image store's tests (#446): just enough
// bytes for the sniffer and the dimension parsers, built by hand so each test
// says what it holds. None is a decodable picture; none needs to be.

const bytes = (...parts: (number[] | Uint8Array | string)[]): Uint8Array => {
  const out: number[] = [];
  for (const p of parts) {
    if (typeof p === "string") for (const ch of p) out.push(ch.charCodeAt(0));
    else out.push(...p);
  }
  return new Uint8Array(out);
};
const be16 = (n: number) => [(n >> 8) & 0xff, n & 0xff];
const be32 = (n: number) => [(n >>> 24) & 0xff, (n >> 16) & 0xff, (n >> 8) & 0xff, n & 0xff];
const le16 = (n: number) => [n & 0xff, (n >> 8) & 0xff];
const le24 = (n: number) => [n & 0xff, (n >> 8) & 0xff, (n >> 16) & 0xff];
const le32 = (n: number) => [n & 0xff, (n >> 8) & 0xff, (n >> 16) & 0xff, (n >>> 24) & 0xff];

const segment = (marker: number, data: number[] | Uint8Array) => bytes([0xff, marker], be16(data.length + 2), data);

// jpeg: SOI, optional APP1 Exif, APP0, a fill byte, an RST marker, SOF0 with
// the size, then the start of scan.
export function jpeg(width: number, height: number, opts: { exif?: boolean; sof?: number } = {}): Uint8Array {
  return bytes(
    [0xff, 0xd8],
    opts.exif ? segment(0xe1, bytes("Exif", [0, 0], [0x4d, 0x4d])) : [],
    segment(0xe0, bytes("JFIF", [0])),
    [0xff, 0xff, 0xd0], // a fill byte before a marker, and a marker without a length
    segment(0xc4, [0, 1, 2]), // DHT: a C-marker that is not a frame
    segment(opts.sof ?? 0xc0, [8, ...be16(height), ...be16(width), 3, 1, 0x11, 0]),
    [0xff, 0xda, 0, 2],
  );
}

const chunk = (name: string, data: number[]) => bytes(be32(data.length), name, data, [0, 0, 0, 0]);

export function png(width: number, height: number, opts: { exif?: boolean } = {}): Uint8Array {
  return bytes(
    [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a],
    chunk("IHDR", [...be32(width), ...be32(height), 8, 6, 0, 0, 0]),
    opts.exif ? chunk("eXIf", [0x4d, 0x4d]) : [],
    chunk("IDAT", [1, 2, 3]),
  );
}

const riff = (body: Uint8Array) => bytes("RIFF", le32(body.length + 4), "WEBP", body);

export const webpLossy = (width: number, height: number) =>
  riff(bytes("VP8 ", le32(10), [0, 0, 0], [0x9d, 0x01, 0x2a], le16(width), le16(height)));

export const webpLossless = (width: number, height: number) => {
  const bits = ((width - 1) & 0x3fff) | (((height - 1) & 0x3fff) << 14);
  return riff(bytes("VP8L", le32(5), [0x2f], le32(bits)));
};

export const webpExtended = (width: number, height: number, flags = 0) =>
  riff(bytes("VP8X", le32(10), [flags, 0, 0, 0], le24(width - 1), le24(height - 1)));

export const svg = () => bytes('<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>');
export const gif = () => bytes("GIF89a", [1, 0, 1, 0, 0, 0, 0]);
