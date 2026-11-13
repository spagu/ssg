// The SMTP wire: one command out, one (possibly multi-line) reply back, under
// a per-command time limit. A leading underscore keeps this file out of the
// Pages route table — it is imported, never served. Portable: see _mime.ts.

// The part of a "cloudflare:sockets" Socket the client uses.
export interface SmtpSocket {
  readable: ReadableStream<Uint8Array>;
  writable: WritableStream<Uint8Array>;
  startTls(): SmtpSocket;
  close(): Promise<void>;
}

export interface Reply {
  code: number;
  lines: string[]; // the text of each line, without the code
}

// SmtpFailure carries the classification all the way up. `answered` means
// the server replied (with a code we did not want), so the connection itself
// is still in a known state.
export class SmtpFailure extends Error {
  readonly temporary: boolean;
  readonly answered: boolean;

  constructor(temporary: boolean, message: string, answered = false) {
    super(message);
    this.temporary = temporary;
    this.answered = answered;
  }
}

const encoder = new TextEncoder();

export class Wire {
  private reader!: ReadableStreamDefaultReader<Uint8Array>;
  private writer!: WritableStreamDefaultWriter<Uint8Array>;
  private socket!: SmtpSocket;
  private readonly timeoutMs: number;
  private decoder = new TextDecoder();
  private buffer = "";

  constructor(socket: SmtpSocket, timeoutMs: number) {
    this.timeoutMs = timeoutMs;
    this.attach(socket);
  }

  private attach(socket: SmtpSocket): void {
    this.socket = socket;
    this.reader = socket.readable.getReader();
    this.writer = socket.writable.getWriter();
  }

  // upgrade switches to TLS after the server's "220" to STARTTLS. Anything
  // already buffered was sent before the handshake by someone who may not be
  // the server (the STARTTLS command-injection attack), so it is refused.
  upgrade(): void {
    if (this.buffer) throw new SmtpFailure(false, "smtp: unexpected data after the STARTTLS reply");
    this.reader.releaseLock();
    this.writer.releaseLock();
    this.attach(this.socket.startTls());
  }

  private async line(): Promise<string> {
    for (;;) {
      const end = this.buffer.indexOf("\n");
      if (end >= 0) {
        const line = this.buffer.slice(0, end).replace(/\r$/, "");
        this.buffer = this.buffer.slice(end + 1);
        return line;
      }
      const { value, done } = await this.reader.read();
      if (done) throw new Error("connection closed by the server");
      this.buffer += this.decoder.decode(value, { stream: true });
    }
  }

  // reply reads "250-first", "250-second", "250 last" as one reply.
  private async reply(): Promise<Reply> {
    const lines: string[] = [];
    for (;;) {
      const line = await this.line();
      const m = /^(\d{3})([ -]?)(.*)$/.exec(line);
      if (!m) throw new SmtpFailure(true, `smtp: malformed reply ${JSON.stringify(line.slice(0, 80))}`);
      lines.push(m[3]);
      if (m[2] !== "-") return { code: Number(m[1]), lines };
    }
  }

  // exchange sends `command` (null: just read, for the greeting) and returns
  // the reply, or fails as temporary on a drop or once timeoutMs has passed.
  async exchange(stage: string, command: string | null): Promise<Reply> {
    let timer: ReturnType<typeof setTimeout> | null = null; // set synchronously below
    const work = (async () => {
      if (command !== null) await this.writer.write(encoder.encode(`${command}\r\n`));
      return this.reply();
    })();
    work.catch(() => {}); // the loser of the race below must not go unhandled
    const timeout = new Promise<never>((_, reject) => {
      timer = setTimeout(() => reject(new SmtpFailure(true, `smtp: no reply at ${stage} within ${this.timeoutMs} ms`)), this.timeoutMs);
    });
    return Promise.race([work, timeout])
      .catch((e) => {
        throw e instanceof SmtpFailure ? e : new SmtpFailure(true, `smtp: connection lost at ${stage}: ${(e as Error).message ?? e}`);
      })
      .finally(() => clearTimeout(timer));
  }

  // expect is exchange that also insists on one of the `ok` codes. A 4xx is
  // temporary, a 5xx permanent; anything else is a confused server, retried.
  async expect(stage: string, command: string | null, ok: number[]): Promise<Reply> {
    const reply = await this.exchange(stage, command);
    if (!ok.includes(reply.code)) {
      throw new SmtpFailure(reply.code < 500, `smtp ${reply.code} at ${stage}: ${reply.lines.join(" ")}`, true);
    }
    return reply;
  }

  async close(): Promise<void> {
    try {
      await this.socket.close();
    } catch {
      /* already closed */
    }
  }
}
