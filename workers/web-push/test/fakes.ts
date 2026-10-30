// In-memory stand-ins for the bindings the worker uses: just the D1 calls
// _store.ts makes, a KV map, and a Pages context.

export interface Row {
  endpoint: string;
  p256dh: string;
  auth: string;
  ua_hash: string | null;
}

export function fakeDB(rows = new Map<string, Row>()) {
  const stmt = (sql: string, args: unknown[] = []) => ({
    bind: (...a: unknown[]) => stmt(sql, a),
    async run() {
      if (sql.startsWith("INSERT")) {
        const [endpoint, p256dh, auth, ua_hash] = args as [string, string, string, string | null];
        rows.set(endpoint, { endpoint, p256dh, auth, ua_hash });
      } else if (sql.startsWith("DELETE")) {
        rows.delete(args[0] as string);
      }
      return { success: true };
    },
    async all() {
      return { results: [...rows.values()].map(({ endpoint, p256dh, auth }) => ({ endpoint, p256dh, auth })) };
    },
  });
  return {
    rows,
    prepare: (sql: string) => stmt(sql.trim()),
    batch: async (list: unknown[]) => list.map(() => ({ success: true })),
  };
}

export function fakeKV(init: Record<string, string> = {}) {
  const data = new Map(Object.entries(init));
  return {
    data,
    get: async (k: string) => data.get(k) ?? null,
    put: async (k: string, v: string) => void data.set(k, v),
  };
}

export function ctx(request: Request, env: Record<string, unknown>) {
  return { request, env, waitUntil: () => {}, params: {}, data: {}, next: async () => new Response() } as never;
}

// A browser subscription with valid keys (RFC 8291's example user agent).
export const SUB = {
  endpoint: "https://fcm.googleapis.com/fcm/send/abc",
  keys: {
    p256dh: "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
    auth: "BTBZMqHH6r4Tts7J_aSIgg",
  },
};
