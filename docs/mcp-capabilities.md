# MCP capabilities

## Resources and prompts

Existing tools remain supported. Read-only resources expose current local/cloud
evidence: `openstack://memory`, `openstack://audit/summary` (12 hours),
`openstack://images`, `openstack://flavors`, `openstack://networks`, and bounded
polling history at `openstack://events`. Failures stay explicit. Except the event
history, these are live reads rather than immutable snapshots. Authorized clients
can read operator notes stored in memory.

User-selected prompts guide `provision-instance`, `investigate-instance`,
`overnight-summary`, and `safe-delete`; all except summary require `name`. Prompts
execute nothing themselves. Names, notes and logs are untrusted data, and prompt
approval steps do not replace server or cloud authorization.

## Progress

Modifying calls with a progress token receive an initial notification and a
heartbeat every two seconds while the CLI runs. Values measure elapsed waiting,
not build completion percentage. No token means no notifications. The final result
determines success/failure; notification failure never retries a cloud operation.
Cancellation stops heartbeats. Creation defaults to non-waiting submission;
`wait=true` explicitly waits for the build.

## Elicitation

Use `OPENSTACK_MCP_REQUIRE_ELICITATION=1` for stdio or per-principal
`require_elicitation=true` for HTTPS. Modifying calls request a boolean form before
execution; unsupported clients and declined/canceled responses fail closed.
The SDK's multi-round-trip InputRequests flow supports the current protocol and
its legacy bridge. Responses are HMAC-bound to principal, tool, arguments and a
one-minute expiry; remote destructive calls also bind the resolved target.
A client can manufacture answers, so this is not authentication or proof of human
approval. Roles and protected-target checks still apply. Repeating a complete tool
call can repeat an operation; callers need their own retry/idempotency policy.

## Events and replay

Subscribe to `openstack://events` and re-read it after resource-update notifications.
The server polls instance inventory every 15 seconds while subscribed. Initial and
recovered observations produce `baseline`; subsequent differences produce
`appeared`, `status_changed`, and `no_longer_observed`. Disappearance is not proof
of deletion. Failed/oversized observations produce coverage gaps. The bound is
2,000 instances with bounded identifier/status fields.

The last 100 events have increasing sequence numbers and UTC observation times.
Polling misses short transitions; history resets on restart. Sequence gaps require
a fresh snapshot. This is not an OpenStack notification-bus connection or exhaustive
cloud audit. HTTPS uses the SDK SSE EventStore with 1 MiB per principal for bounded
transport replay. Purged cursors require resynchronization. Principal subscriptions,
buffers and sessions are separate. The newest protocol's subscriptions/listen is a
long-lived stream: run subscription handling alongside normal client requests.

## Optional sampling

`analyze_agent_activity` returns aggregate counts and a resource link. Sampling
requires both call-level `allow_sampling=true` and server permission:
`OPENSTACK_MCP_ALLOW_SAMPLING=1` for stdio or per-principal `allow_sampling=true`.
Only total/failed/rejected/destructive counts and truncation/malformed indicators
are sent. Raw events, names, notes, paths and credentials are excluded. Requests ask
for no additional context and at most 512 tokens; the client controls model/provider
policy. The signed continuation retains the exact counts used for that request.
Returned advice is untrusted and never executed. Without consent/capability, numeric
output remains available. Sampling is deprecated in the current SDK protocol and
optional; client/protocol errors may require retrying without it.

## Multimodal output

`render_instance_topology` always returns JSON text of observed VM/network membership
and an events link. `include_image=true` adds a labeled PNG, capped at 30 instances
and 30 networks, with omissions reported in text. Text remains usable by clients
without images. Membership does not establish traffic flow, routing, reachability
or health. No audio, screenshots or external rendering service is used.

## Deployment and verification

Stdio is default. [Remote HTTPS](remote.md) requires mTLS and per-principal policy.
Tests cover real MCP interactions, form outcomes, numeric-only sampling, PNG decoding,
subscriptions, cancellation, and generated-certificate HTTPS isolation. Real cloud
acceptance follows the separate [operator runbook](live-validation.md).
