// In-memory stand-ins: an SMTP server behind a fake "cloudflare:sockets"
// connect, a recording D1, and a fetch that answers from a script.

import type { SmtpConnect, SmtpSocket } from "../functions/api/newsletter/_smtp.ts";

export type Stage = "greeting" | "EHLO" | "STARTTLS" | "AUTH" | "MAIL" | "RCPT" | "DATA" | "DOT" | "RSET" | "QUIT";

export interface ServerOptions {
  starttls?: boolean; // advertise STARTTLS on the plain connection (default true)
  auth?: string; // the AUTH line's mechanisms (default "PLAIN LOGIN"; "" = no AUTH line)
  user?: string;
  password?: string;
  // Override a reply: a reply string, "drop" (close the connection) or "hang"
  // (never answer). Return undefined for the normal reply.
  on?: (stage: Stage, line: string, tls: boolean) => string | undefined;
  afterStartTls?: string; // a line sent along with the "220" to STARTTLS
  closeThrows?: boolean; // socket.close() rejects
}

const enc = new TextEncoder();
const dec = new TextDecoder();

export function smtpServer(o: ServerOptions = {}) {
  const user = o.user ?? "mailer@example.com";
  const password = o.password ?? "s3cret";
  const state = {
    log: [] as { line: string; tls: boolean }[], // every command line, with whether TLS was on
    messages: [] as string[], // DATA payloads as received (still dot-stuffed)
    connects: [] as { address: { hostname: string; port: number }; options: { secureTransport: string; allowHalfOpen: boolean } }[],
    startTls: 0,
    closes: 0,
  };

  function socket(tls: boolean, greet: boolean): SmtpSocket {
    let ctrl!: ReadableStreamDefaultController<Uint8Array>;
    let open = true;
    const readable = new ReadableStream<Uint8Array>({ start: (c) => void (ctrl = c) });
    const say = (s: string) => open && ctrl.enqueue(enc.encode(s));
    const end = () => {
      if (open) {
        open = false;
        ctrl.close();
      }
    };
    const respond = (stage: Stage, line: string, normal: string): string => {
      const r = o.on?.(stage, line, tls) ?? normal;
      if (r === "drop") end();
      else if (r !== "hang") say(`${r}\r\n`);
      return r;
    };

    let pending = "";
    let inData = false;
    let data = "";
    let login = 0;
    let loginUser = "";
    const handle = (line: string) => {
      if (inData) {
        if (line !== ".") return void (data += `${line}\r\n`);
        inData = false;
        state.messages.push(data);
        data = "";
        return void respond("DOT", line, "250 2.0.0 queued as 1");
      }
      state.log.push({ line, tls });
      if (login === 1) {
        login = 2;
        loginUser = line;
        return void respond("AUTH", line, "334 UGFzc3dvcmQ6");
      }
      if (login === 2) {
        login = 0;
        const good = atob(loginUser) === user && atob(line) === password;
        return void respond("AUTH", line, good ? "235 2.7.0 ok" : "535 5.7.8 bad credentials");
      }
      const [verb, mech, arg] = line.split(" ");
      switch (verb) {
        case "EHLO": {
          const caps = ["smtp.example.com hello"];
          if (!tls && o.starttls !== false) caps.push("STARTTLS");
          if ((o.auth ?? "PLAIN LOGIN") !== "") caps.push(o.auth?.startsWith("=") ? `AUTH${o.auth}` : `AUTH ${o.auth ?? "PLAIN LOGIN"}`);
          caps.push("8BITMIME");
          return void respond("EHLO", line, caps.map((c, i) => `250${i === caps.length - 1 ? " " : "-"}${c}`).join("\r\n"));
        }
        case "STARTTLS":
          // Injected bytes ride in the same segment as the 220, as in the attack.
          return void respond("STARTTLS", line, `220 2.0.0 ready${o.afterStartTls ? `\r\n${o.afterStartTls}` : ""}`);
        case "AUTH":
          if (mech === "PLAIN") {
            return void respond("AUTH", line, atob(arg) === `\0${user}\0${password}` ? "235 2.7.0 ok" : "535 5.7.8 bad credentials");
          }
          login = 1;
          return void respond("AUTH", line, "334 VXNlcm5hbWU6");
        case "MAIL":
          return void respond("MAIL", line, "250 2.1.0 ok");
        case "RCPT":
          return void respond("RCPT", line, "250 2.1.5 ok");
        case "DATA":
          inData = respond("DATA", line, "354 go ahead").startsWith("354");
          return;
        case "RSET":
          return void respond("RSET", line, "250 2.0.0 reset");
        case "QUIT":
          respond("QUIT", line, "221 2.0.0 bye");
          return end();
        default:
          say("500 5.5.1 unknown command\r\n");
      }
    };

    const writable = new WritableStream<Uint8Array>({
      write(chunk) {
        pending += dec.decode(chunk);
        for (let i = pending.indexOf("\r\n"); i >= 0; i = pending.indexOf("\r\n")) {
          const line = pending.slice(0, i);
          pending = pending.slice(i + 2);
          handle(line);
        }
      },
    });

    if (greet) queueMicrotask(() => respond("greeting", "", "220 smtp.example.com ESMTP"));
    return {
      readable,
      writable,
      startTls() {
        state.startTls++;
        return socket(true, false);
      },
      async close() {
        state.closes++;
        end();
        if (o.closeThrows) throw new Error("already closed");
      },
    };
  }

  const connect: SmtpConnect = (address, options) => {
    state.connects.push({ address, options });
    return socket(options.secureTransport === "on", true);
  };
  return { connect, state, user, password };
}

// fakeDB records every statement; `first` answers SELECTs, `changes` UPDATEs.
export function fakeDB(opts: { first?: unknown; changes?: number } = {}) {
  const calls: { sql: string; args: unknown[] }[] = [];
  const stmt = (sql: string, args: unknown[] = []) => ({
    bind: (...a: unknown[]) => stmt(sql, a),
    async run() {
      calls.push({ sql, args });
      return { success: true, meta: opts.changes === undefined ? {} : { changes: opts.changes } };
    },
    async first() {
      calls.push({ sql, args });
      return opts.first ?? null;
    },
  });
  return { calls, prepare: (sql: string) => stmt(sql.replace(/\s+/g, " ").trim()) };
}

// fakeFetch installs a global fetch answering `respond`, recording requests.
export function fakeFetch(respond: (url: string, init: RequestInit) => Response | Promise<Response>) {
  const calls: { url: string; init: RequestInit; body: unknown }[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    calls.push({ url, init, body: typeof init.body === "string" ? JSON.parse(init.body) : init.body });
    return respond(url, init);
  }) as typeof fetch;
  return { calls, restore: () => void (globalThis.fetch = original) };
}

// quietly runs fn with console.warn captured.
export async function quietly<T>(fn: () => Promise<T>): Promise<{ result: T; warnings: string[] }> {
  const warnings: string[] = [];
  const original = console.warn;
  console.warn = (...a: unknown[]) => void warnings.push(a.join(" "));
  try {
    return { result: await fn(), warnings };
  } finally {
    console.warn = original;
  }
}
