# Authenticated Streamable HTTP

Stdio remains the default. Remote mode requires TLS 1.3, client certificates
(mTLS), and explicit operator configuration. No plaintext listener, forwarded
identity-header trust, bearer-token shortcut or OAuth discovery endpoint is offered.
Use an MCP client that can configure a client certificate and trusted server CA;
clients without this capability should use stdio over SSH.

## Deployment

1. Install the OpenStack CLI in a dedicated non-root service account's trusted PATH.
   The executable, Python plugins and system CLI configuration are trusted inputs.
2. Obtain a server certificate with the correct hostname SAN and server-auth usage.
   Issue separate client-auth certificates from a trusted client CA. Keep the CA
   private key offline and never share client private keys between identities.
3. Copy `examples/remote.example.json` and `examples/credentials.example.json` to
   operator-owned paths and replace all placeholders. Use project-scoped application
   credentials with only the OpenStack permissions each principal needs.
4. Get each client certificate's fingerprint:

   ```bash
   openssl x509 -in client.crt -noout -fingerprint -sha256
   ```

   Put its 64 hex digits, without colons, in `certificate_sha256`. Both the trusted
   CA chain and this exact certificate pin must match.
5. Set configuration, credentials and TLS private key permissions to `0600` or
   stricter, and each unique state directory to `0700`. Principal paths must be
   absolute. Keep real credentials outside the repository.
6. Build and start:

   ```bash
   go build -o openstack-mcp-server ./cmd/openstack-mcp-server
   ./openstack-mcp-server --transport=https --http-config=/etc/openstack-mcp/remote.json
   ```

The example endpoint is `https://localhost:8443/mcp`. For another hostname, change
the bind address, certificate SAN and exact `allowed_hosts` entry together. Origin
headers must exactly match an optional `allowed_origins` HTTPS entry; no wildcard
or permissive CORS is enabled. Configure clients with the server CA and their
client certificate/private key. Never disable certificate verification. A proxy
must preserve end-to-end mTLS; identity headers are ignored.

## Roles and isolation

| Role | Allowed operations |
| --- | --- |
| `viewer` | Inventory, resources, prompts, planning, audit summaries, optional analysis and topology |
| `operator` | Viewer operations plus creation |
| `admin` | Operator operations plus deletion, lifecycle actions and memory updates |

Every tool call and interactive retry is authorized; unknown tools are denied.
Each principal has a separate MCP server/session namespace, SSE store, event feed,
memory and audit path. CLI subprocesses receive only explicit application credentials
and a small base environment, with a private HOME/config directory and working
directory. Ambient `OS_*` credentials are not inherited. The MCP role does not reduce
credential rights when those credentials are used outside this server. Separate
cloud projects are required for actual tenant separation; sharing credentials
intentionally shares cloud visibility.

Remote lifecycle/deletion resolves names or IDs, checks immutable operator-defined
`protected_patterns`, then acts on the resolved ID. Omitted patterns default to
`prod-*`, `*-prod`, `production-*`; `[]` disables this protection. All lifecycle
actions, including start, are covered. `record_agent_memory` cannot change this
remote policy. Stdio retains its documented planner-only policy behavior.

`require_elicitation=true` adds confirmation before modifications. Unsupported
clients, declined/canceled answers, altered arguments, changed resolved targets,
and responses older than one minute fail without that mutation. Elicitation is
not proof of human approval: clients can synthesize answers. Roles, protected
patterns, `confirm_name`, and cloud RBAC always apply.

## Limits and operation

- Per principal: 10 requests/second, burst 20; 16 concurrent requests and a
  conservative cap of 16 active/pending sessions. Long subscriptions occupy slots.
  Overflow returns 429. Idle sessions expire after five minutes.
- Requests: 1 MiB bodies, 16 KiB headers, 5-second header and 15-second request-read
  timeouts. Certificates are checked each request; contexts expire with the cert.
- SSE replay: 1 MiB in memory per principal. A purged cursor requires re-reading
  resources. Replay and event history do not survive restart.
- RPC audit records principal, method/tool and outcome without request arguments
  or responses. Existing command audit separately records operation arguments.
  `awaiting_input` is not a completed operation. Counts can include RPC and command
  entries, so do not interpret them as unique cloud operation counts.
- SIGINT/SIGTERM cancels polling, closes sessions and shuts down HTTP. To revoke or
  rotate a certificate, role or credential, edit configuration and restart. This
  closes active streams. CRL/OCSP and live reload are not implemented.
- Rotate/archive audit logs externally and monitor disk usage. Audit writing is
  best effort, not a fail-closed compliance system. Do not share writable state
  files between server processes.

Generated-certificate loopback integration tests cover these boundaries. They do
not constitute independent security certification or live OpenStack acceptance.
Follow [live validation](live-validation.md) before rollout.
