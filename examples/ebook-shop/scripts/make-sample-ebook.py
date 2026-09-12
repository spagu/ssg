#!/usr/bin/env python3
"""Writes the demo shop's sample ebooks.

A real PDF, built by hand rather than by a library, so the example has a file to
sell without the repository carrying a binary anyone has to trust. Two pages,
Helvetica, no images: enough for the upload check (which sniffs %PDF), the R2
round-trip and the range request the download endpoint supports.

    python3 examples/ebook-shop/scripts/make-sample-ebook.py
"""

import sys
import zlib
from pathlib import Path


def escape(text):
    return text.replace("\\", r"\\").replace("(", r"\(").replace(")", r"\)")


def page_stream(lines):
    """lines: (x, y, size, text) in PDF points, origin bottom-left."""
    out = []
    for x, y, size, text in lines:
        out.append("BT /F1 {} Tf {} {} Td ({}) Tj ET".format(size, x, y, escape(text)))
    return zlib.compress("\n".join(out).encode("latin-1", "replace"))


def build(title, author, pages):
    objects = []

    def add(body):
        objects.append(body)
        return len(objects)

    font = add(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
    # A page object has to name the pages tree, and the tree has to list the
    # pages, so the tree's number is reserved here and its body written last.
    tree = add(b"")
    page_ids = []
    for content in pages:
        stream = page_stream(content)
        contents = add(
            b"<< /Length %d /Filter /FlateDecode >>\nstream\n" % len(stream) + stream + b"\nendstream"
        )
        page_ids.append(
            add(
                b"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] "
                b"/Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>"
                % (tree, font, contents)
            )
        )
    kids = b" ".join(b"%d 0 R" % i for i in page_ids)
    objects[tree - 1] = b"<< /Type /Pages /Kids [%s] /Count %d >>" % (kids, len(page_ids))
    info = add(
        b"<< /Title (%s) /Author (%s) /Producer (ssg example shop) >>"
        % (escape(title).encode("latin-1", "replace"), escape(author).encode("latin-1", "replace"))
    )
    catalog = add(b"<< /Type /Catalog /Pages %d 0 R >>" % tree)

    out = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = [0]
    for number, body in enumerate(objects, start=1):
        offsets.append(len(out))
        out += b"%d 0 obj\n" % number + body + b"\nendobj\n"

    start = len(out)
    out += b"xref\n0 %d\n" % (len(objects) + 1)
    out += b"0000000000 65535 f \n"
    for offset in offsets[1:]:
        out += b"%010d 00000 n \n" % offset
    out += b"trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (
        len(objects) + 1,
        catalog,
        info,
        start,
    )
    return bytes(out)


BOOKS = [
    (
        "ten-thousand-words-a-month.pdf",
        "Ten Thousand Words a Month",
        "Maria Halloran",
        [
            [
                (70, 720, 28, "Ten Thousand Words a Month"),
                (70, 680, 14, "Maria Halloran - Paperless Press"),
                (70, 620, 11, "This is the sample file the example shop sells."),
                (70, 600, 11, "It exists so a checkout can be tested end to end without"),
                (70, 580, 11, "anyone having to write a book first."),
            ],
            [
                (70, 760, 18, "1. The hour you have"),
                (70, 720, 11, "A quota is not a measure of ambition. It is a measure of"),
                (70, 700, 11, "how much of your day is genuinely yours, which for most"),
                (70, 680, 11, "working writers is one hour and not the one they wanted."),
            ],
        ],
    ),
    (
        "the-one-person-shop.pdf",
        "The One-Person Shop",
        "Tomasz Wierzbicki",
        [
            [
                (70, 720, 28, "The One-Person Shop"),
                (70, 680, 14, "Tomasz Wierzbicki - Paperless Press"),
                (70, 620, 11, "This is the sample file the example shop sells."),
                (70, 600, 11, "Upload it in the panel, buy it with a test card, and watch"),
                (70, 580, 11, "the invoice, the email and the download link appear."),
            ],
            [
                (70, 760, 18, "1. Where the sale happens"),
                (70, 720, 11, "For a digital service the place of supply is the customer's"),
                (70, 700, 11, "place, and you must hold two non-contradictory pieces of"),
                (70, 680, 11, "evidence for where that is. Not one. Two."),
            ],
        ],
    ),
]


def main():
    out_dir = Path(__file__).resolve().parent.parent / "files"
    out_dir.mkdir(exist_ok=True)
    for name, title, author, pages in BOOKS:
        path = out_dir / name
        path.write_bytes(build(title, author, pages))
        print("{}  {} bytes".format(path, path.stat().st_size))
    return 0


if __name__ == "__main__":
    sys.exit(main())
