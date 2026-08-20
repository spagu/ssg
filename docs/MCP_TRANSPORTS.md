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
| `--token=SECRET` | Require `Authorization: Bearer SECRET`. Falls back to **`$SSG_MCP_TOKEN`** when the flag is absent, and is minted and printed when neither is set |
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
- **A bearer token on every endpoint**, generated if you did not supply one:

  ```
  🌐 MCP endpoint: http://[::]:7824/mcp (Streamable HTTP)
     Authorization: Bearer ff4d45eb53e6f283236ca5c2ec94660f…
     ⚠️  Listening beyond localhost. This server writes files and runs git:
        put it behind TLS and keep the token secret.
  ```

Put a real deployment behind TLS. The token is a shared secret, and plain HTTP
carries it in the clear on every request.

#### Where the token comes from

Three sources, in order: `--token=…`, then `$SSG_MCP_TOKEN`, then a freshly
minted one printed at startup. **Every `--listen` endpoint ends up with a token**
— loopback included.

That last part changed in 1.8.47. Minting used to be conditional on the listener
being off loopback, which made the deployment recommended immediately below —
loopback listener, reverse proxy owning the public address — the only shape that
could end up with no authentication at all. `--token="$SSG_MCP_TOKEN"` with the
variable unset expands to an empty argument; the address is loopback; nothing was
minted; and the startup line said *"No token — loopback only"* about a server
that writes files and runs git and was, at that moment, reachable from the
internet. A token on a loopback listener nobody proxies costs nothing — the
client is being configured anyway — so there is no longer a case without one.

Prefer the variable to the flag. A command line is visible in `ps`, in the shell
history, in every `docker inspect`, and in the supervisor's own log when it
echoes what it started; `ssg` already takes this position for `mcp.git.token`,
whose documentation says to always use `$ENV`.

```
   Authorization: Bearer 6f1c…            # supplied: yours, unchanged
   Minted for this run — set SSG_MCP_TOKEN to keep it stable across restarts.
```

A minted token is new on every start, so a client configured with one stops
working when the server restarts. Set `SSG_MCP_TOKEN` for anything long-lived.

### Serving a CMS over a network

```bash
SSG_MCP_TOKEN=… ssg mcp --listen=127.0.0.1:7823 --no-stdio
```

Bind to loopback and let a reverse proxy own TLS and the public address, rather
than binding the server to a routable interface directly. Then the proxy holds
the certificate, the rate limiting and the access log, and the MCP server keeps
one job.

The token travels in the environment, not on the command line — and if the
variable is missing, the endpoint still comes up authenticated with a minted
token rather than open. Read the startup line to find out which happened:

```
🌐 MCP endpoint: http://127.0.0.1:7823/mcp (Streamable HTTP)
   Authorization: Bearer …
   Minted for this run — set SSG_MCP_TOKEN to keep it stable across restarts.
```

That second line is the one to alert on in a supervised deployment: it means the
secret you thought you passed did not arrive.

`GET` and `DELETE` answer `405` by design: they belonged to the standalone-stream
and session mechanics of earlier protocol revisions, which this transport does
not implement.

## Protocol version

`ssg mcp` speaks **both eras**, and says so:

```bash
curl -sX POST http://127.0.0.1:7823/mcp -H 'Mcp-Method: server/discover' \
  -d '{"jsonrpc":"2.0","id":1,"method":"server/discover"}'
# {"resultType":"complete","protocolVersions":["2026-07-28","2025-06-18"], …}
```

**`2026-07-28`** is the stateless shape: no `initialize`, every request carrying
its own protocol version and client identity in `_meta`, `server/discover`
mandatory, a `resultType` on every result, and `ttlMs`/`cacheScope` on list
results so a client caches instead of polling.

```bash
curl -sX POST http://127.0.0.1:7823/mcp \
  -H 'MCP-Protocol-Version: 2026-07-28' -H 'Mcp-Method: tools/list' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list",
       "params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}'
```

**`2025-06-18`** — the `initialize` handshake — is answered exactly as it always
was. A request that declares no version in `_meta` is an older client, and it
gets the older shape unchanged: that revision changed the protocol rather than
extending it, and adopting it by abandoning the older era would strand every
client that has not moved.

A version this server does not implement is refused with
`UnsupportedProtocolVersionError` (`-32022`) **listing what it does**, so a
client can retry rather than guess.

### Header validation

From `2026-07-28`, a POST mirrors `Mcp-Method` and `Mcp-Name` into headers so
gateways can route without parsing the body — and the server checks they agree
with it. An intermediary routing on the header while the server executes on the
body is how a request ends up somewhere it was not authorised for, so a mismatch
is `HeaderMismatch` (`-32020`):

```
{"code":-32020,"message":"header mismatch: Mcp-Name header value 'wrong' does not match body value 'help'"}
```

A name that cannot be written as plain ASCII travels as `=?base64?…?=` and is
decoded before the comparison, so a non-ASCII tool name is not mistaken for an
attack. Only modern requests are held to this: demanding the headers of an older
client would reject everything it has ever sent.

The HTTP+SSE transport from `2024-11-05` is deprecated in the specification and
is not implemented here; Streamable HTTP replaced it.
