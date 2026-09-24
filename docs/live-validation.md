# Live OpenStack acceptance

Automated tests use fake CLI fixtures, temporary state, in-memory MCP, and loopback
HTTPS with generated certificates. They do not certify your cloud, OpenStack CLI
or client UI. Use a dedicated test project. Record the commit, Go/CLI/cloud versions,
client name and negotiated MCP version. Keep credentials and raw state private.

## Local checks

```bash
go test -race -cover ./...
go vet ./...
go build -o openstack-mcp-server ./cmd/openstack-mcp-server
```

Configure your authorized test-project CLI environment and launch stdio from the
MCP client. Give it unique `OPENSTACK_MCP_MEMORY_FILE` and `OPENSTACK_MCP_AUDIT_LOG`
paths. Compare inventory with the CLI. Query a known instance by ID and inspect
image/flavor/network fields. Duplicate names must fail ambiguously rather than
select an arbitrary instance; use IDs for precise lookup.

## Read-only features

List and read resources. Obtain every prompt without automatically executing it.
Render topology with and without PNG and compare membership. Subscribe to events;
a baseline should arrive after the first polling interval. Record unsupported
client capabilities as untested, not passed.

Call activity analysis without sampling first. Only enable both sampling permissions
if your policy permits sending aggregate counts to the client's model. Verify the
advisory label and absence of raw event/credential content. Core output must remain
usable without sampling.

## Remote acceptance

Follow `remote.md` with test certificates and separate project-scoped application
credentials. Check:

- Missing/untrusted/expired/unconfigured certificates are rejected.
- Viewer reads work, but mutations are denied; operator creation is allowed only
  within its cloud permissions, with lifecycle/deletion/memory updates denied.
- Principals cannot read one another's notes, audit or event feeds. A borrowed
  session ID grants no access. Use separate cloud projects to claim tenant isolation.
- Protected test instances are blocked by both name and ID. Do not use production
  instances for these tests. Mismatched confirmation, unsupported required forms,
  and declined forms cause no mutation. Accepted forms do not override protection.
- Replacing a certificate pin and restarting revokes the old connection and permits
  the new certificate. Confirm graceful shutdown and client reconnection behavior.

## Disposable-instance exercise

This section changes cloud state and may incur cost. Run only in an authorized
scratch project with known image/flavor/network, available quota and a cleanup owner.

1. Plan a uniquely named disposable instance and review exact arguments.
2. Create without `wait`; an ID/BUILD response means submission, not completion.
   Poll `get_instance` until ACTIVE or ERROR.
3. In a separate run, use `wait=true` and a progress token. Heartbeats show elapsed
   waiting. Cancel a wait and inspect inventory before retrying: cancellation does
   not undo a creation already accepted by OpenStack.
4. Subscribe while performing an approved test lifecycle transition. Expect updates
   within a polling interval plus CLI latency; short intermediate states may be missed.
5. Delete only the disposable instance with matching confirmation and any required
   form. Verify removal and clean up remaining resources using your normal workflow.

Do not inject failures into shared production services. Exercise outages, credential
failures and replay loss with fixtures or an isolated lab.

## Report

Share sanitized versions, completed checks, expected/actual outcomes, coverage gaps
and failing test names. Redact identities, paths, addresses, credentials and notes.
Do not attach databases, memory files, private keys or unreviewed audit logs.
