"""Verify one DKIM-Signature (rsa-sha256, relaxed/relaxed) with the standard
library and the openssl CLI — enough for the smoke test, no dkimpy needed.

usage: python3 -I dkim_verify.py MESSAGE PUBLIC_KEY_PEM
Exit 0 and "dkim: pass" when the body hash and the signature both check out.
"""

import base64
import hashlib
import re
import subprocess
import sys
import tempfile


def split_message(raw):
    head, _, body = raw.partition(b"\r\n\r\n")
    headers = []
    for line in head.split(b"\r\n"):
        if line[:1] in (b" ", b"\t") and headers:
            headers[-1] += b"\r\n" + line
        else:
            headers.append(line)
    return headers, body


def relaxed_header(field):
    name, _, value = field.partition(b":")
    value = re.sub(rb"\r\n", b"", value)
    value = re.sub(rb"[ \t]+", b" ", value).strip(b" ")
    return name.strip().lower() + b":" + value


def relaxed_body(body):
    lines = [re.sub(rb"[ \t]+", b" ", line).rstrip(b" ") for line in body.split(b"\r\n")]
    while lines and lines[-1] == b"":
        lines.pop()
    return b"".join(line + b"\r\n" for line in lines)


def main(message_path, key_path):
    raw = open(message_path, "rb").read().replace(b"\r\n", b"\n").replace(b"\n", b"\r\n")
    headers, body = split_message(raw)
    sig_field = next(h for h in headers if h.lower().startswith(b"dkim-signature:"))
    tags = {}
    for part in re.sub(rb"\s+", b"", sig_field.partition(b":")[2]).split(b";"):
        if b"=" in part:
            k, _, v = part.partition(b"=")
            tags[k.decode()] = v
    assert tags["a"] == b"rsa-sha256" and tags["c"] == b"relaxed/relaxed", tags
    body_hash = base64.b64encode(hashlib.sha256(relaxed_body(body)).digest())
    if body_hash != tags["bh"]:
        print("dkim: fail (body hash)")
        return 1
    used = set()
    data = b""
    for name in tags["h"].decode().split(":"):
        # The last unused instance of each named header, bottom up (RFC 6376 5.4.2).
        for i in range(len(headers) - 1, -1, -1):
            if i not in used and headers[i].partition(b":")[0].strip().lower() == name.lower().encode():
                used.add(i)
                data += relaxed_header(headers[i]) + b"\r\n"
                break
    unsigned = re.sub(rb"(\bb=)[^;]*", rb"\1", sig_field)
    data += relaxed_header(unsigned)
    with tempfile.NamedTemporaryFile() as d, tempfile.NamedTemporaryFile() as s:
        d.write(data)
        d.flush()
        s.write(base64.b64decode(tags["b"]))
        s.flush()
        ok = subprocess.run(
            ["openssl", "dgst", "-sha256", "-verify", key_path, "-signature", s.name, d.name],
            capture_output=True,
        ).returncode == 0
    print("dkim: pass" if ok else "dkim: fail (signature)")
    print("signed headers: " + tags["h"].decode())
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1], sys.argv[2]))
