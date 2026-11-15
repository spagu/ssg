"""SMTP checks for smoke.sh: logins, one message per port, and the refusals
that make this not an open relay. Standard library only.

usage: python3 -I smtp_checks.py PORT465 PORT587 PASSWORD_FILE
"""

import smtplib
import ssl
import sys

HOST = "127.0.0.1"
USER = "newsletter"

# The smoke test's certificate is self-signed: check encryption, not trust.
CTX = ssl.create_default_context()
CTX.check_hostname = False
CTX.verify_mode = ssl.CERT_NONE

MESSAGE = (
    "From: News <news@example.com>\r\n"
    "To: reader@example.com\r\n"
    "Subject: smoke test\r\n"
    "Message-ID: <smoke-{port}@example.com>\r\n"
    "Date: Sun, 15 Nov 2026 12:00:00 +0000\r\n"
    "MIME-Version: 1.0\r\n"
    "Content-Type: text/plain; charset=utf-8\r\n"
    "List-Unsubscribe: <https://example.com/u/abc>\r\n"
    "List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n"
    "\r\n"
    "Hello from the smoke test.\r\n"
)


def expect_refusal(label, action):
    try:
        action()
    except smtplib.SMTPException as e:
        code = getattr(e, "smtp_code", None) or next(iter(getattr(e, "recipients", {}).values()), (None,))[0]
        print(f"ok: {label} refused ({code})")
        return
    raise SystemExit(f"FAIL: {label} was accepted")


def main(port465, port587, password_file):
    password = open(password_file).read().strip()

    # Before STARTTLS the server must not offer AUTH (smtpd_tls_auth_only).
    with smtplib.SMTP(HOST, port587, timeout=10) as s:
        s.ehlo("client.example.com")
        if s.has_extn("auth"):
            raise SystemExit("FAIL: AUTH offered before STARTTLS")
        code, _ = s.docmd("MAIL FROM:<news@example.com>")
        if code < 500:
            raise SystemExit(f"FAIL: MAIL accepted in clear text on 587 ({code})")
        print(f"ok: 587 without STARTTLS: no AUTH, MAIL refused ({code})")

    with smtplib.SMTP_SSL(HOST, port465, context=CTX, timeout=10) as s:
        s.login(USER, password)
        s.sendmail("news@example.com", ["reader@example.com"], MESSAGE.format(port=465))
        print("ok: 465 AUTH and message accepted")

    with smtplib.SMTP(HOST, port587, timeout=10) as s:
        s.starttls(context=CTX)
        s.login(USER, password)
        s.sendmail("news@example.com", ["reader@example.com"], MESSAGE.format(port=587))
        print("ok: 587 STARTTLS, AUTH and message accepted")

    def wrong_password():
        with smtplib.SMTP_SSL(HOST, port465, context=CTX, timeout=10) as s:
            s.login(USER, password + "x")

    expect_refusal("wrong password", wrong_password)

    def open_relay():
        with smtplib.SMTP(HOST, port587, timeout=10) as s:
            s.starttls(context=CTX)
            s.sendmail("news@example.com", ["someone@example.org"], MESSAGE.format(port=0))

    expect_refusal("relaying without a login (open-relay test)", open_relay)

    def foreign_sender():
        with smtplib.SMTP_SSL(HOST, port465, context=CTX, timeout=10) as s:
            s.login(USER, password)
            s.sendmail("someone@example.org", ["reader@example.com"], MESSAGE.format(port=1))

    expect_refusal("logged-in client with a sender outside SENDER_DOMAINS", foreign_sender)


if __name__ == "__main__":
    main(int(sys.argv[1]), int(sys.argv[2]), sys.argv[3])
