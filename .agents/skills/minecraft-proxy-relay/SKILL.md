---
name: minecraft-proxy-relay
description: Use for the desktop relay lifecycle, including frp TCP/UDP forwarding, ephemeral GCE VMs, Cloudflare DNS, and cross-platform OAuth.
---

# Minecraft Proxy relay

Use [the design](../../../docs/design.md) for the public contract, lifecycle, configuration, and acceptance tests. Use current primary references to confirm changing flags, scopes, and field names.

- [Google Go Style](https://google.github.io/styleguide/go/) explains Go error handling and package boundaries. Keep provider operations cancellable with `context.Context` and reconcile partial state on retry. Time out network and subprocess operations.
- [GCE insert](https://docs.cloud.google.com/compute/docs/reference/rest/v1/instances/insert) and [automatic termination](https://docs.cloud.google.com/compute/docs/instances/limit-vm-runtime) define retries and server-side expiry. Reuse the request ID for retries of the same create operation. For recovery and deletion, check the deterministic instance name, resource ID when known, and ownership labels.
- [frp TCP/UDP](https://gofrp.org/en/docs/features/tcp-udp/), [authentication](https://gofrp.org/en/docs/features/common/authentication/), and [TLS](https://gofrp.org/en/docs/features/common/network/network-tls/) govern the control connection. Pin frpc and frps together and verify release assets. Built-in frp TLS encrypts by default, and server identity verification requires a trusted CA or explicit trust configuration.
- [Cloudflare OAuth clients](https://developers.cloudflare.com/fundamentals/oauth/create-an-oauth-client/) define available flows, scopes, and publication. Publish a DNS-only record after the relay answers, and check its ownership before cleanup. Use the Cloudflare batch API when replacing conflicting records.
- Use OAuth and disposable test inputs for app validation. Keep production tokens, game data, and deployment-specific settings out of commits and snapshots. Testing a live relay must use the intended project and domain, and confirm deletion afterward.
