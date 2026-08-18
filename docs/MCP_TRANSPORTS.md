# MCP transports: stdio and Streamable HTTP

`ssg mcp` speaks the Model Context Protocol on two bindings. Protocol semantics
are identical on both — a transport defines how messages are framed and
delivered, not what they mean — so every tool behaves the same whichever one a
client uses.

| Transport | When to use it |
|---|---|
| **stdio** (default) | The client launches `ssg mcp` and owns its standard streams. The right choice for an assistant on the same machine |
| **Streamable HTTP** | The client dials an address. The only way to reach the server from anywhere else |

Both can run at once, which is the ordinary case while developing: an editor
spawns the process over stdio, and a second client reaches the same running
server over HTTP.

## stdio

```jsonc
// the client's MCP configuration
{ "command": "ssg", "args": ["mcp", "--role=designer"] }
```

Nothing to configure and nothing to secure: the client is the parent process.

## Streamable HTTP

```bash
ssg mcp --listen=7823
#  🌐 MCP endpoint: http://127.0.0.1:7823/mcp (Streamable HTTP)
```

The endpoint is `/mcp` and accepts POST. Each JSON-RPC request is its own POST
and gets its own response:

```bash
curl -X POST http://127.0.0.1:7823/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2025-06-18' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

| Flag | Meaning |
|---|---|
| `--listen=ADDR` | Serve the MCP endpoint. **A bare port means localhost** — `--listen=7823` binds `127.0.0.1:7823`, not every interface |
| `--token=SECRET` | Require `Authorization: Bearer SECRET`. Minted and printed automatically when the listener is not on loopback |
| `--allow-origin=URL` | Accept one browser origin; repeatable |
| `--no-stdio` | Serve only the endpoint, for a server a supervisor starts rather than a client |

### Security

**This server writes files and runs git.** An MCP endpoint a web page can reach
is a remote code execution path, so the transport enforces the specification's
rules rather than offering them:

- **`Origin` is validated.** Anything not on `--allow-origin` gets `403`. Without
  this, a page the operator merely visits can resolve a name to `127.0.0.1` and
  drive a local MCP server — DNS rebinding. A request carrying **no** `Origin` is
  allowed: that is a non-browser client, which cannot be a rebinding victim.
- **Localhost by default**, so exposing the server is a decision rather than an
  accident.
- **A bearer token off loopback**, generated if you did not supply one:

  ```
  🌐 MCP endpoint: http://[::]:7824/mcp (Streamable HTTP)
     Authorization: Bearer ff4d45eb53e6f283236ca5c2ec94660f…
     ⚠️  Listening beyond localhost. This server writes files and runs git:
        put it behind TLS and keep the token secret.
  ```

Put a real deployment behind TLS. The token is a shared secret, and plain HTTP
carries it in the clear on every request.

### Serving a CMS over a network

```bash
ssg mcp --listen=127.0.0.1:7823 --token="$SSG_MCP_TOKEN" --no-stdio
```

Bind to loopback and let a reverse proxy own TLS and the public address, rather
than binding the server to a routable interface directly. Then the proxy holds
the certificate, the rate limiting and the access log, and the MCP server keeps
one job.

`GET` and `DELETE` answer `405` by design: they belonged to the standalone-stream
and session mechanics of earlier protocol revisions, which this transport does
not implement.

## Protocol version

`ssg mcp` implements **`2025-06-18`** and echoes back whatever version a client
asks for during `initialize`.

It deliberately does not claim `2026-07-28`, the current revision. That one
removed the `initialize` handshake entirely — every request carries its own
version and capabilities, `server/discover` is mandatory, and every result needs
a `resultType` — and declaring a shape a server does not implement is a lie a
client acts on. Adopting it is tracked in
[#174](https://github.com/spagu/ssg/issues/174).

The HTTP+SSE transport from `2024-11-05` is deprecated in the specification and
is not implemented here; Streamable HTTP replaced it.
