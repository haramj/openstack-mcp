# MCP capabilities and rollout decisions

## Resources and prompts (issue #9)

Existing tools remain supported. Five read-only resources expose the same current
local/cloud evidence: `openstack://memory`, `openstack://audit/summary` (12 hours),
`openstack://images`, `openstack://flavors`, and `openstack://networks`. Read failures
are errors, never invented empty inventories. These are live reads, not subscriptions
or immutable snapshots. Clients must re-read when freshness matters. Reading local
memory may expose operator notes; use only trusted local clients.

Four user-selected prompts provide guidance, not autonomous execution:
`provision-instance`, `investigate-instance`, `overnight-summary`, and `safe-delete`.
All except the summary require `name`. Prompts treat returned logs and names as
untrusted data, preserve approval steps, and do not replace authorization.

## Progress (first part of issue #11)

Modifying tool calls with an MCP progress token receive an initial notification
and a heartbeat every two seconds while the CLI is running. The monotonically
increasing value measures elapsed waiting, **not** a build completion percentage.
No token means no notifications. The final tool response determines success/failure;
notification failure never retries a cloud operation. Context cancellation stops
the heartbeat. Default creation already returns without waiting; use `wait=true`
only when an explicitly waiting call is desired.

## Remote HTTP decision (issue #10: still open)

Keep stdio as the only transport for this release. Adding a listener around the
existing shared credential, memory and audit context would create an authorization
boundary the implementation does not have. There is no supported unauthenticated
HTTP mode. This is a deferral, not an implementation of Remote MCP.

For remote operation today, launch the stdio process over SSH, for example:

```bash
ssh -T operator@controller /absolute/path/to/openstack-mcp-wrapper
```

The operator-owned wrapper must configure its authorized OpenStack environment
and `exec` the server without emitting banners on stdout. SSH host verification
and account access remain required; no MCP ports need forwarding. Each independent
server process should use separate memory and audit paths.

Before adding HTTP, agree on and test:

1. Authenticated identity (OAuth or mTLS) with issuer/audience validation, TLS,
   origin checks, credential rotation/revocation, and request limits.
2. Explicit read/write/admin roles enforced at every tool/resource boundary.
3. Per-principal OpenStack project credentials, memory, audit and event isolation;
   an untrusted client cannot choose another principal's storage or project.
4. Deny-by-default authorization, auditable identity without credential leakage,
   rate limits, cancellation, shutdown, and session expiry.
5. Integration tests for unauthenticated/expired identities, cross-project access,
   denied mutations, browser origin attacks, and token redaction.

## Advanced interactions (remainder of issue #11: still open)

Elicitation may improve user experience but is **not proof of human approval**:
clients can synthesize answers. It must not bypass protected-target checks or
replace a server-side authorization policy. Before implementation, specify opt-in
behavior, denied/unsupported/time-out behavior, and the trust boundary.

Events require authorized, per-project subscriptions, bounded retention/replay,
backpressure, reconnect semantics and a real event source. A local heartbeat is
not an OpenStack state-change event. Sampling must remain optional and must never
receive credentials/private logs without an explicit data-sharing policy.
Multimodal output needs a concrete use case and a text fallback. No Events,
Sampling, Elicitation, image/audio output or cloud notification bus is claimed by
this release. These proposals remain open for scoped implementation and review.
