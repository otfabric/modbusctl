# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in `modbusctl`, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email **security@otfabric.com**, or open a private advisory on GitHub (the [Security](https://github.com/otfabric/modbusctl/security) tab → **Advisories** → **Report a vulnerability**), with:

- A description of the vulnerability
- Steps to reproduce the issue
- The affected version(s) (`modbusctl version` output)
- Any potential impact you've identified

We will acknowledge your report within 48 hours and aim to provide a fix or mitigation within 7 days for critical issues.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | Best effort |

## Security Considerations

### Modbus Has No Built-In Security

Modbus TCP, UDP, RTU and ASCII carry no authentication and no encryption: anyone who can reach a device can read and write it, and anyone on the path can read or alter the traffic. `modbusctl` does not change that.

- **Use it only on devices and networks you own or are authorized to test**
- **Keep Modbus traffic on isolated, trusted networks**; never expose port 502 to the Internet
- For access across untrusted networks, use a VPN or a secured gateway in front of the device

### Commands That Change Device State

- **`client write`** changes coils and holding registers. On real equipment this can start, stop or reconfigure a process. Know what an address controls before writing to it. All values are validated before anything is sent, and a write is sent exactly once (never retried)
- **`client diagnostic`** (FC08) includes sub-functions that restart communications, clear counters or force listen-only mode
- **`client scan`** and **`discover`** generate many requests. Some devices and gateways react badly to this; start with `--algo safe` and a `--delay`, and scan production equipment only inside an agreed maintenance window

### Running a Server

- `server static` and `server replay` listen on all network interfaces and accept any client. Run them on trusted networks, or restrict access with a host firewall
- Captures served by these commands may come from real equipment; treat them accordingly

### Captures and Output

- Capture files (MBCAP), exports and command output contain register values, device identification and network addresses, which may be sensitive operational data. Be careful when storing or sharing them
- `--debug` prints every Modbus frame sent and received to stderr

### Privileges

- `discover --resolve-mac` uses raw sockets for ARP and may need elevated privileges. Prefer granting the specific capability (for example `CAP_NET_RAW` on Linux) over running `modbusctl` as root
- Serial devices only need read/write access to the port (for example membership of the `dialout` group on Linux)

### Verifying Downloads

Release archives are published with SHA-256 checksums. Verify them as shown in the [installation instructions](README.md#installation) before installing.
