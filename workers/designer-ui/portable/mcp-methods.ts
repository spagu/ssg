// The MCP methods of a server that offers tools and nothing else: initialize,
// ping, server/discover, tools/list, tools/call. The HTTP side — methods,
// headers, size, the JSON-RPC envelope, which protocol era a request speaks —
// is mcp-transport.ts; this file only answers a request already checked.
//
// Portable: no import outside this folder, so designer-ui copies it byte for
// byte with mcp-transport.ts. The host (the worker) supplies the tools, the
// permission check and the mapping of its own errors.
//
// Two eras, one endpoint (specification 2026-07-28, "Versioning and
// Compatibility"): a request carrying `_meta["io.modelcontextprotocol/
// protocolVersion"]` is modern and gets `resultType` and the server's
// identity in `_meta`; one without it comes from an `initialize`-based
// client (2025-11-25 and earlier) and is answered in that shape. Neither
// needs state: this server never mints a session.

export const MODERN_VERSIONS: readonly string[] = ["2026-07-28"];
export const LEGACY_VERSIONS: readonly string[] = ["2025-11-25", "2025-06-18", "2025-03-26"];
export const SUPPORTED_VERSIONS: readonly string[] = [...MODERN_VERSIONS, ...LEGACY_VERSIONS];

// JSON-RPC 2.0 codes, and the two the MCP specification allocates that apply here.
export const CODE = {
  parse: -32700,
  invalidRequest: -32600,
  methodNotFound: -32601,
  invalidParams: -32602,
  internal: -32603,
  headerMismatch: -32020,
  unsupportedVersion: -32022,
} as const;

export const META_VERSION = "io.modelcontextprotocol/protocolVersion";
export const META_CAPABILITIES = "io.modelcontextprotocol/clientCapabilities";
const META_SERVER = "io.modelcontextprotocol/serverInfo";
// A tool list depends on the caller's permissions: private, and short-lived.
const CACHE_HINTS = { ttlMs: 60_000, cacheScope: "private" };
const MAX_ECHO = 128; // characters of a caller's tool name repeated in an error

export type Json = Record<string, unknown>;
// Call runs one of the host's operations with the caller's permission check.
export type Call<Op> = (op: Op, input: unknown) => Promise<unknown>;

export interface McpTool<Op = unknown> {
  name: string;
  description: string;
  inputSchema: Json;
  op: Op;
  annotations?: Json;
  // input builds the operation's input from the call's arguments (default:
  // the arguments as they are). `call` lets it ask another operation first —
  // the name a new file would get, say — under the same permission check.
  input?: (args: Json, call: Call<Op>) => unknown;
}

export interface McpHost<Op = unknown> {
  server: { name: string; version: string };
  instructions: string;
  tools: readonly McpTool<Op>[];
  visible: (tool: McpTool<Op>) => boolean; // shown in tools/list to this caller
  call: Call<Op>;
  // refusal: the body of a refusal the caller may read (a tool error), or
  // null for an unexpected error — answered as an internal error, never with
  // its text.
  refusal: (err: unknown) => Promise<Json | null>;
  maxBytes?: number; // the request body's limit (default DEFAULT_MAX_BYTES)
  origins?: readonly string[]; // browser origins accepted besides the endpoint's own
}

export interface RpcRequest { id: string | number; method: string; params: Json; modern: boolean }

// RpcFailure: a JSON-RPC error to answer, with its HTTP status; `id` once the
// request's id is known (an error about an unreadable message has none).
export class RpcFailure extends Error {
  readonly code: number;
  readonly status: number;
  readonly data?: unknown;
  id?: string | number;
  constructor(code: number, message: string, status = 200, data?: unknown) {
    super(message);
    this.code = code;
    this.status = status;
    this.data = data;
  }
}

export const isObject = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);

export const unsupportedVersion = (requested: string): RpcFailure =>
  new RpcFailure(CODE.unsupportedVersion, "unsupported protocol version", 400, { supported: SUPPORTED_VERSIONS, requested });

const capabilities = { tools: { listChanged: false } };

// answer runs one request and returns its result (or throws RpcFailure).
export async function answer<Op>(host: McpHost<Op>, req: RpcRequest): Promise<Json> {
  const result = await methodResult(host, req);
  return req.modern ? { resultType: "complete", ...result, _meta: { [META_SERVER]: host.server } } : result;
}

async function methodResult<Op>(host: McpHost<Op>, req: RpcRequest): Promise<Json> {
  switch (req.method) {
    case "initialize": {
      // Version negotiation of the initialize era: the client's version when
      // this server speaks it, else the newest of that era.
      const asked = req.params.protocolVersion;
      const version = typeof asked === "string" && LEGACY_VERSIONS.includes(asked) ? asked : LEGACY_VERSIONS[0];
      return { protocolVersion: version, capabilities, serverInfo: host.server, instructions: host.instructions };
    }
    case "ping":
      return {};
    case "server/discover":
      return { supportedVersions: SUPPORTED_VERSIONS, capabilities, instructions: host.instructions, ...CACHE_HINTS };
    case "tools/list":
      // Every tool fits one page; a cursor this server never gave is refused.
      if (req.params.cursor !== undefined) throw new RpcFailure(CODE.invalidParams, "invalid cursor");
      return { tools: host.tools.filter((t) => host.visible(t)).map(definition), ...(req.modern ? CACHE_HINTS : {}) };
    case "tools/call":
      return callTool(host, req.params);
    default:
      // The modern binding answers an unknown method 404 (Streamable HTTP,
      // "Protocol Version Header"); the initialize era expects a 200.
      throw new RpcFailure(CODE.methodNotFound, "method not found", req.modern ? 404 : 200);
  }
}

function definition<Op>(t: McpTool<Op>): Json {
  return { name: t.name, description: t.description, inputSchema: t.inputSchema, ...(t.annotations ? { annotations: t.annotations } : {}) };
}

// callTool: a refusal of the operation is a tool error the model can read
// (isError); an unknown tool or malformed arguments are protocol errors.
// A tool the caller cannot see may still be called by name: the operation's
// own permission check refuses it, exactly as the REST route would.
async function callTool<Op>(host: McpHost<Op>, params: Json): Promise<Json> {
  const name = params.name;
  if (typeof name !== "string") throw new RpcFailure(CODE.invalidParams, "params.name must be a tool name");
  const tool = host.tools.find((t) => t.name === name);
  if (!tool) throw new RpcFailure(CODE.invalidParams, `unknown tool: ${name.slice(0, MAX_ECHO)}`);
  const args = params.arguments ?? {};
  if (!isObject(args)) throw new RpcFailure(CODE.invalidParams, "params.arguments must be an object");
  try {
    const input = tool.input ? await tool.input(args, host.call) : args;
    return toolResult(await host.call(tool.op, input), false);
  } catch (err) {
    const body = await host.refusal(err);
    if (!body) throw new RpcFailure(CODE.internal, "internal error");
    return toolResult(body, true);
  }
}

// toolResult: the data as structured content and, for clients that read
// text only, the same JSON as one text block.
function toolResult(data: unknown, isError: boolean): Json {
  const structured = isObject(data) ? data : { result: data };
  return { content: [{ type: "text", text: JSON.stringify(structured) }], structuredContent: structured, isError };
}
