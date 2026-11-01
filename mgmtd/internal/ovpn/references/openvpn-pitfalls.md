---
title: OpenVPN service pitfalls — verified failure modes
category: openvpn-service-operations
last_verified: 2026-10-30
version: 1
---

These are the failure modes confirmed by real execution in this environment — not guesses or generic docs. Apply as rules, not as notes.

## CN injection (must gate every IssueServer / IssueClient)

Rule: `ValidateCN` must be called before `IssueServer` and `IssueClient`. Pattern: `^[A-Za-z0-9_.-]{1,64}$`.
Why: CN enters the `.ovpn` filename, `client-config-dir` file path, `crl-dir` serialization, and `common_name` display. A newline in CN injects `push/script-security` directives into the server config via `client-connect` hooks. Confirmed: `alice\npush` passes `x509.CreateCertificate` with no validation; the conf writer treats it as part of a path string.
Verification: tests with bad names (`../etc/passwd`, `../../evil`, `alice\npush ...`, `>64 chars`) must fail before cert creation; legal names (`alice`, `bob-01`, `site_2.a`) pass.

## Status parsing must read real `--status` file format, not only management interface

Rule: `ParseClientList` must handle both formats: (a) `>CLIENT_LIST` / `>ROUTING_TABLE` / `>STATUS` with `HEADER` lines (management interface); (b) bare header line + bare data lines (e.g. `Common Name,Real Address,...`) in `status.log`.
Why: openvpn writes `status.log` continuously; `management` only returns interactive replies. A parser that only recognizes (a) always returns 0 clients on a real server. The failure was hidden because tests only used format (a); real-world verification required format (b) with the exact `ROUTING_TABLE` header (`Virtual Address,Common Name,Real Address,Last Ref` — no `routing` word).
Verification: `TestParseStatusFileFormat` uses the verbatim `status.log` from the running server on this host; `ParseClientList` returns 1 client with `virtual_address=10.8.0.2`, bytes matching the original line (`5059`/`7748`), and `ConnectedSince` formatted from the epoch.

## `ParseClientList` vs `ParseStatus` contract

Rule: `ParseClientList` returns `[]Client`; `ParseStatus` returns `Status` (aggregate fields). The consumer (`vpn_servers.go` `handleGetOpenVPNStatus`) must assemble `Status` explicitly: `resp := Status{UpSince: st2.UpSince, TotalBytesIn: st2.TotalBytesIn, TotalBytesOut: st2.TotalBytesOut, Clients: clients}`.
Why: writing the response as the bare `[]Client` (before the fix) produces `{...}` missing `.clients`; the frontend reads `.clients`, so the table appears empty even when clients exist.

## Systemctl restart success is not proof of service health

Rule: after `runCommands` with `systemctl restart openvpn-server@router-re`, poll `serviceActive(..., "openvpn-server@router-re")` up to 3 seconds; if not `"active"`, set `applied=false`, include the last `systemctl` error line, and check the journal (`journalctl -n 20 -u openvpn-server@router-re`) for the real cause (`Cannot open TUN/TAP`, `Cannot pre-load keyfile`).
Why: `systemctl restart` exits 0 when the restart job is submitted — the daemon exits within seconds and the job still reports success briefly. This is the exact same failure class as the `wc -l` `|| echo 0` silent-zero issue cited in AGENTS.md (`|| echo 0` turns errors into zeros that propagate). Confirmed: after restart, `ss -lun | grep 1194` must show; otherwise `applied` is false.

## PKI paths must be unified (`/var/lib/router-re/ovpn/pki` only)

Rule: `LoadOrCreate`, `IssueServer`, `IssueClient`, `List`, `ValidateCN`, `server.conf` (via `RenderOpenVPNServerConf`), `.ovpn` (`client-conf` endpoint), and the `rr-*` server files (`ca`, `cert`, `key`, `tls-crypt`) all point to one dir.
Why: using `/etc/openvpn/server/rr-*.crt` for server files while `/var/lib/router-re/ovpn/pki/` for clients creates different CA instances → the `.ovpn` cert is signed by a different CA than the one in `rr-ca.crt` → TLS handshake fails with `TLS handshake failed` or `incoming plaintext read error`. Confirmed: before fix, the `.ovpn` `.crt` matched `/var/lib/router-re/ovpn/pki/ca.crt` and the server `.crt` matched `/etc/openvpn/server/rr-server.crt`; they were different files with different CA subjects. Unifying to `/var/lib/router-re/ovpn/pki/` fixes this.

## Container tun limitation (132 = LXC, no `/dev/net/tun`)

Rule: verify `tunUsable()` (open `/dev/net/tun` with `O_RDWR`) before claiming tunnel works. If false, show the alert on the page (`lxc.cgroup2.devices.allow: c 10:200 rwm`, `lxc.mount.entry: /dev/net/tun`), and do NOT rely on `systemctl` status alone.
Why: `systemd-detect-virt` = `lxc`, `CapEff` has `cap_net_admin`, `mknod /dev/net/tun` succeeds but `open` returns `EPERM`. The service restarts but openvpn exits `Cannot open TUN/TAP`. The `notInstalled` / `tun` alert must display the LXC config snippet, and verification must be done on a machine with real tun (e.g., 10.100.1.18) with an independent `netns` client to avoid the local hairpin (same-host `tunrrs` + `tunrrc` causes `ping` to deliver locally without traversing the tunnel).
Verification pattern: `netns add rrc`; `veth10` + `veth11`; `openvpn --config ... --daemon`; `netns exec rrc ping -c 3 -W 2 10.8.0.1`; confirm 3/3 with non-zero RTT; server `status.log` shows client row; clean up.

## Key files
- `docs/ovpn-design-20261030.md` — design, endpoints, verification criteria, environment constraints (updated with verified tunnel evidence and cleanup notes).
- `mgmtd/internal/ovpn/pki.go` — PKI package (with validation fix 6067eda).
- `mgmtd/internal/ovpn/status.go` — parser (with real-file fix 7ff05be).
- `mgmtd/internal/ovpn/status_test.go` — `TestParseStatusFileFormat` (verbatim real `status.log`).
- `mgmtd/internal/ovpn/pki_test.go` — `TestCNValidationRejectsInjection`.
- `mgmtd/internal/server/vpn_servers.go` — consumer adapter (`ParseClientList` returns `Status`, `client-conf` uses per-client cert).
- `mgmtd/internal/server/vpn_tun_test.go` — `TestTunUsableMatchesSyscall`.
- `docs/openvpn-pkitls.md` (optional) — reference for config rendering details.

## Verification checklist (must all be true before saying done)
- [x] `go build ./...` passes (all packages).
- [x] `go test ./internal/ovpn/ ./internal/vpn/ ./internal/server/ -count=1` passes.
- [x] Independent `openssl verify -CAfile ca.crt alice.crt server.crt` passes; `EKU` = ClientAuth; `Serial` random; `NotAfter` 3 years; private key `0600`; `CN` passes `ValidateCN`.
- [x] Real `status.log` (`/tmp/status_copy.log`) parsed by `ParseClientList` returns clients=1, `virt=10.8.0.2`, `in=5059`, `out=7748`, `up_since` correct.
- [x] Real tunnel (132 with tun enabled) from independent `netns`: `ping -I tunrrc 10.8.0.1` = 3/3; server `status.log` shows client; `openssl verify` on `.ovpn` cert passes.
- [x] Invalid `CN` (`../evil`, newline, >64 chars) rejected at `IssueClient` with 422 error.
- [x] `tunnel` verification requires real `netns`, not same-host hairpin (documented, test pattern saved).
- [x] Page renders: `PPTP / L2TP / OpenVPN` tab → OpenVPN card → client form + table; `tun` false shows alert; `client-conf` button generates `.ovpn`; `status` reflects real data (`alice3`, `10.8.0.2`, bytes growing).
- [x] No fabricated data: all claims backed by tool output (`openvpn` binary version, `systemctl` status, `journalctl` error lines, `ss`, `ping`, `openssl`, `curl` response bodies, `go test`, `git diff --stat`, browser text).
