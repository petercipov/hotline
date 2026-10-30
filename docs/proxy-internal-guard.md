# Guarding against calls to internal addresses: scope and test cases

*Agreed with Peter and built, 2026-10-29. The source of truth is `proxy-spec.md` in the `hotline-strategic` repo.*

## What the feature is

Server side request forgery (SSRF) is when an attacker makes a server call something it shouldn't. The proxy is a server that calls whatever address an app names, and calls without a known integration ID may go to any host. So any app, or anyone who can reach the proxy port, could use it to call:

- the cloud's metadata address `169.254.169.254`, which hands out the machine's credentials;
- other services inside the cluster or the company network;
- the proxy itself, including its admin port, or in a loop back into its own proxy port.

This slice closes that on every ingress already built (forward proxy and vendor URL in path) and on the ones still to come.

## Requirements

1. **Its own slice, first** (Peter, 2026-10-29). It closes a gap in code already merged, so it comes before the Host header ingress, which is then guarded from the start.
2. **Block internal IPs, checked after DNS** (Peter, 2026-10-29). The check is on the IP the proxy is about to connect to, not on the name, so a vendor name that resolves to an internal IP, now or after a DNS change, is caught too. The same check covers an IP written straight into the address. Internal means:
   - loopback: `127.0.0.0/8`, `::1`;
   - link-local, which holds the cloud metadata address: `169.254.0.0/16`, `fe80::/10`;
   - private: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`;
   - shared address space, used by some clouds and Kubernetes set-ups: `100.64.0.0/10`;
   - unspecified (`0.0.0.0/8`, `::`) and multicast (`224.0.0.0/4`, `ff00::/8`);
   - the same IPv4 addresses written in IPv6 form (`::ffff:127.0.0.1`).
3. **Allow per api** (Peter, 2026-10-29). An api can set `allowInternalIPs: true` for a vendor on an internal address, such as a bank link over a VPN. It applies only to calls for that api's hosts. The `not-passed` entry can set it too, like its other settings; the built-in default is `false`.
4. **Calls without a known ID still reach public hosts** (Peter, 2026-10-29). The earlier rule stays: they go through with a minimal record.
5. **A name with both kinds of address.** When DNS gives public and internal IPs, the proxy uses only the public ones. When it gives only internal ones, the call is refused.
6. **What a refused call gets.** `403` with `Proxy-Status: hotline; error=destination_ip_prohibited` (the RFC 9209 name), and nothing is sent to the vendor. It is recorded like the proxy's other errors, with `error.type=destination_ip_prohibited`: a full record for a known integration, a minimal one otherwise.
7. **No separate loop check** (Peter, 2026-10-29). A call to the proxy itself goes to a loopback or cluster-internal IP, so the guard refuses it. This answers the loop question from the Host header ingress.

## Tests

- Feature scenarios: `src/hotline-proxy/features/internal-guard.feature`.
- The fake vendors in the tests use `192.0.2.x`, a range kept for documentation, which is not internal, so the other tests are not affected.
- End to end: the test vendor runs on `localhost`, so the end-to-end config sets `allowInternalIPs: true` on its api and on `not-passed`.

## How it is built

- The check runs in the proxy's dialer, after the DNS lookup: internal IPs are dropped from the answer, and if none is left the call fails with `403`. So it covers every ingress and every redirect of DNS.
- Calls whose api sets `allowInternalIPs: true` use their own pool of connections, so a connection opened for them is never reused by a guarded call.

## Not in this slice

A list of allowed or blocked ranges in config; blocking by host name; caller authentication; the Host header ingress.
