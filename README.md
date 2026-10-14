# portquiz

Test all outbound TCP/UDP ports for connectivity to a remote host.

Testing all ports takes about 2 minutes per protocol with the default settings, which start at most 500 connections per second (`-rate`). Ports blocked by a firewall that silently drops traffic each wait out a timeout, which is kept short by adapting it to how quickly the server responds (see [Timeouts](#timeouts) and [Performance is slow](#troubleshooting)).

Can also be used to detect passive DPI Firewalls that block traffic not looking like the expected service for a given port.

## Use Cases

- **Network troubleshooting**: Identify which ports are blocked by firewalls
- **Security testing**: Test outbound connectivity from restricted networks
- **DPI detection**: Discover deep packet inspection that blocks non-standard protocols
- **Infrastructure validation**: Verify port accessibility in cloud/container environments
- **Network monitoring**: Baseline network connectivity for monitoring
- **Quick checks from any device**: the server's [browser test](#browser-test) needs nothing installed

## Installation

### [Download Latest Release](https://github.com/lanrat/portquiz/releases)

**For testing connectivity (any platform):**

- Download `portquiz` for your platform (Windows, macOS, Linux)

**For testing connectivity from an Android phone (arm64, Android 10+):**

- Install `portquiz_Android_arm64.apk`, see [android/README.md](android/README.md)

**For running a server (Linux only):**

- Download `portquiz-server` for Linux
- On Alpine Linux, install or upgrade the server with one command, see [Running as a Service on Alpine Linux](#running-as-a-service-on-alpine-linux-openrc)

### Build from source

```bash
git clone https://github.com/lanrat/portquiz.git
cd portquiz
make
```

## Quick Start

```bash
# Test all TCP ports (the default when neither -tcp nor -udp is given)
./portquiz example.com

# Test all TCP and UDP ports
./portquiz -tcp -udp example.com

# Test specific ports and ranges only
./portquiz -tcp -port 22,80,443,8000-8100 example.com

# Show only open ports
./portquiz -tcp -udp -open example.com
```

Or, with nothing installed, open `http://<server>/` in any browser for a quick check (see [Browser Test](#browser-test)).

## Server

> ⚠️ **WARNING**: The server creates iptables rules that redirect ALL incoming traffic to the listening IP. If you use the same IP for remote access (SSH, etc.), **YOU WILL BE LOCKED OUT**! Always use a dedicated IP address for the server.

**Requirements:**

- Linux system with iptables
- Root privileges (for firewall rule management)
- Dedicated IP address (separate from management/SSH access)
- If the server is behind NAT, such as a home router, every port must be forwarded to it (often called a "DMZ host" setting), and `-listen` is its own private IP. Otherwise clients only see the ports the router forwards.

```shell
$ ./portquiz-server -h
Usage of ./portquiz-server:
  -acme string
        comma separated DNS names of this server to get Let's Encrypt certificates for, to serve the browser test page over HTTPS too
  -acme-cache string
        directory to keep the certificates and Let's Encrypt account in (default "/var/lib/portquiz/acme")
  -acme-directory string
        ACME directory URL, such as Let's Encrypt's staging server for testing (default "https://acme-v02.api.letsencrypt.org/directory")
  -acme-email string
        email address for Let's Encrypt notices about the certificates (optional)
  -listen string
        comma separated list of non-loopback IPs to listen on (required)
  -no-iptables
        disable automatically creating iptables rules
  -password string
        magicString to use, must be the same on client/server (default "portquiz")
  -port uint
        default port to listen on which will have traffic redirected to (default 1337)
  -tcp
        start TCP server (both TCP and UDP start if neither is set)
  -timeout duration
        amount of time for each connection (default 10s)
  -udp
        start UDP server (both TCP and UDP start if neither is set)
  -verbose
        enable verbose logging
  -version
        show version information
  -web
        serve the browser test page, and answer its HTTP and STUN checks on every port (default true)
```

### Example Server

```shell
# start server
# serves TCP and UDP (the default when neither -tcp nor -udp is given)
# listens on IPv4: 192.0.2.123 and IPv6: 2001:0DB8::1
./portquiz-server -listen 192.0.2.123,2001:0DB8::1
```

### Running as a Service on Alpine Linux (OpenRC)

Each release includes an Alpine package with the server and its OpenRC service. Run this as root to install it, or to upgrade to the latest release:

```shell
wget -qO /tmp/portquiz-server.apk https://github.com/lanrat/portquiz/releases/latest/download/portquiz-server_$(uname -m).apk && apk add --allow-untrusted /tmp/portquiz-server.apk && rm /tmp/portquiz-server.apk
```

The package is not signed, hence `--allow-untrusted`. It installs `portquiz-server` to `/usr/bin`, the service as `/etc/init.d/portquiz` and its settings as `/etc/conf.d/portquiz`, and pulls in `iptables` and `ip6tables`. Upgrading restarts the service if it is running and keeps your settings; removing the package with `apk del portquiz-server` stops the service first so its iptables rules are removed. After the first install, set up the service:

```shell
# set PORTQUIZ_LISTEN to the IP addresses to listen on
vi /etc/conf.d/portquiz

rc-update add portquiz default
rc-service portquiz start
```

Settings in `/etc/conf.d/portquiz`:

- `PORTQUIZ_LISTEN` (required): comma separated IP addresses to listen on. Like `-listen`, all traffic to these addresses is redirected to portquiz, so use addresses dedicated to it and not the one you manage the machine through.
- `PORTQUIZ_OPTS`: extra arguments for `portquiz-server`, for example `-udp` to serve only UDP, or `-acme portquiz.example.com` to serve the [browser test](#browser-test) over HTTPS too (see [With SSL](#with-ssl-https)).
- `PORTQUIZ_BIN`: path to `portquiz-server`. By default `/usr/local/bin/portquiz-server` is used if it exists, otherwise `/usr/bin/portquiz-server` from the package.

The server's output is logged to `/var/log/portquiz.log`. Stopping the service removes the iptables rules the server added.

#### Without the package

The `service/` directory in the Linux release archives contains the same OpenRC service and an installer for it, for systems where the package cannot be used. They are plain `sh` scripts, so they work on a default Alpine install without bash.

```shell
# as root, from an extracted Linux release archive
# (or from a source checkout after running make)
./service/openrc_setup_service.sh

# set PORTQUIZ_LISTEN to the IP addresses to listen on
vi /etc/conf.d/portquiz

rc-service portquiz start
```

The installer:

- installs `iptables` and `ip6tables` with `apk` if they are missing
- copies `portquiz-server` and `portquiz` to `/usr/local/bin`
- installs the service as `/etc/init.d/portquiz` and enables it at boot
- installs `/etc/conf.d/portquiz`, keeping an existing one

Run it again from a newer release to upgrade; it restarts the service if it is configured.

#### Proxmox

In a Proxmox VM, Alpine runs with its own kernel and the steps above work unchanged. An LXC container shares the Proxmox host's kernel, which changes a few things:

- **Kernel modules:** the server's iptables rules need netfilter modules that a container cannot load itself. If the service fails to start with an iptables error in `/var/log/portquiz.log`, load them on the Proxmox host and add them to the host's `/etc/modules` so they load at boot:

  ```shell
  # on the Proxmox host
  modprobe -a nf_tables nft_chain_nat nft_compat xt_comment xt_nat
  ```

- **Proxmox firewall:** if it is enabled for the datacenter, node or container, it filters traffic before it reaches portquiz and every port looks closed. Allow all inbound TCP and UDP to the portquiz IP, or disable the firewall on that network device.
- **Dedicated IP:** give the container a second network device (`net1`) in its Proxmox network settings and use its address for `PORTQUIZ_LISTEN`. Proxmox rewrites `/etc/network/interfaces` inside the container, so do not add addresses there by hand.
- **Recovery:** if you do lose SSH access to the container, `pct enter <id>` on the host or the Proxmox console still gets you in.
- **Connection tracking:** the connection tracking table belongs to the host kernel and is shared by all guests, so heavy scans use entries the other guests also need. The client's default `-rate` keeps this moderate.

To check the service after installing:

```shell
rc-service portquiz status
iptables -t nat -S PREROUTING | grep portquiz   # expect one tcp and one udp rule
tail /var/log/portquiz.log                      # iptables errors appear here
```

Then run `portquiz -port 1-100 <ip>` from outside your network. If the rules are present but every port is closed, check the Proxmox firewall and any NAT forwarding in front of the server.

## Client

The portquiz client connects to the portquiz server and tests port connectivity. By default portquiz will test all ports unless `-port` is specified.

**Note:** The client binary is named `portquiz` (cross-platform), while the server binary is `portquiz-server` (Linux only).

The server can be given as a hostname or an IP address, such as `portquiz 192.0.2.1` or `portquiz 2001:db8::1` (brackets, as in `[2001:db8::1]`, are also accepted).

By default, if the server's name has both IPv4 and IPv6 addresses and this machine can reach both, each port is tested over both, shown as `tcp4` and `tcp6` in the results; otherwise it is tested over the one version available. IPv4 can be forced with `-4` and IPv6 can be forced with `-6`, and `-4 -6` requires both. For a dual stack server, list both of its addresses in the server's `-listen` (or `PORTQUIZ_LISTEN`), or every IPv6 port will look closed.

When run in a terminal, the address being tested is shown first (`Testing example.com at 192.0.2.1`), then a progress bar with an estimated time remaining is shown on stderr (based on the average speed so far, smoothed so it counts down steadily), followed by a summary when testing finishes. It is hidden automatically when stderr is not a terminal (for example when piped or redirected) and in `-verbose` mode, and can be turned off with `-no-progress`. Results are always printed to stdout, so `./portquiz -tcp example.com > results.txt` still shows the progress bar while saving only the results.

```shell
$ ./portquiz -h
Usage of ./portquiz:
  -4    force IPv4
  -6    force IPv6
  -adaptive
        shorten the timeout to fit the measured response time of open ports, up to -timeout (default true)
  -closed
        print only closed ports
  -diff
        compare two JSON reports instead of testing: portquiz -diff a.json b.json lists the ports whose result changed from a to b, and exits with status 1 if any did; with -json it writes the comparison as JSON
  -format string
        output format: "ranges" merges consecutive ports with the same result, "ports" prints one line per port (default "ranges")
  -json file
        when testing finishes, write a JSON report of every result to file, or to stdout with - instead of the usual results
  -label string
        a name for this run recorded in the -json report, such as "home wifi", to tell runs apart when comparing them
  -multi uint
        test multiple times to ensure larger streams work (default 1)
  -no-progress
        disable the progress bar shown when stderr is a terminal
  -open
        print only open ports
  -parallel uint
        maximum number of ports to test at once (default 500)
  -password string
        magicString to use, must be the same on client/server (default "portquiz")
  -port string
        comma separated list of ports and ranges to test, e.g. 22,80,1000-2000 (default all ports)
  -rate uint
        maximum number of connections to start per second, 0 for no limit (default 500)
  -retry times
        number of times to retry a port that did not respond (default 1, or 0 for TCP with -adaptive=false)
  -tcp
        test TCP ports (default if neither -tcp nor -udp is set)
  -timeout duration
        maximum time to wait for each response (default 2s)
  -udp
        test UDP ports
  -verbose
        enable verbose logging
  -version
        show version information
```

### Output

Results are printed to stdout as ranges of consecutive ports with the same result. Each range is printed as soon as it is final, so results appear while the scan is still running, and stopping a scan with Ctrl-C keeps everything finished so far.

```shell
$ ./portquiz -tcp -udp portquiz.example.com
CLOSED tcp 1-21
OPEN tcp 22
CLOSED tcp 23-79
OPEN udp 1-65535
OPEN tcp 80-81
CLOSED tcp 82-65535
```

Ranges for different protocols can be interleaved, as above. Ports listed with `-port` are only merged when they are consecutive, so `-port 22,80-81` prints `22` and `80-81` separately. Use `-open` or `-closed` to print only one kind of result, or `-format ports` to print one line per port instead of ranges.

### How ports are closed

A closed port is reported in one of four ways, shown in the summary at the end of a scan (`Tested 65535 in 3m10s: 120 open, 65415 closed (400 rejected, 65000 dropped, 15 interfered)`) and in the JSON report:

- **rejected**: the connection was actively refused, by a TCP reset, an ICMP unreachable or a local firewall. Firewalls set to reject traffic, and the server's own firewall, do this.
- **dropped**: nothing came back before the timeout, as when a firewall silently discards traffic. These ports are the slow ones to test.
- **interfered**: something other than the portquiz server answered, or the connection was accepted and then cut or left silent. A transparent proxy, captive portal or filter on the path does this, often on ports such as 25, 53 or 80.
- **errors**: the port could not be tested because of a local error, such as running out of sockets.

### JSON report

`-json report.json` writes a report of the whole scan when it finishes, or when it is stopped with Ctrl-C, while the usual results still go to stdout. `-json -` writes the report to stdout instead of the usual results. For each kind tested (`tcp`, `udp6` etc.) it gives the address tested, how many ports had each result, how quickly open ports answered, the timeout in use at the end, and every result as ranges of consecutive ports with the same status:

```json
{
  "version": "v1.12.0",
  "client": "cli",
  "label": "home wifi",
  "host": "portquiz.example.com",
  "started": "2026-10-13T09:00:00.000-07:00",
  "finished": "2026-10-13T09:03:10.000-07:00",
  "duration_seconds": 190,
  "tested_per_second": 344.9,
  "canceled": false,
  "options": { "ports": "1-65535", "tcp": true, "udp": false, "ipv4": false, "ipv6": false, "timeout_ms": 2000, "adaptive": true, "retries": -1, "parallel": 500, "rate": 500, "multi": 1 },
  "kinds": [
    {
      "kind": "tcp",
      "address": "192.0.2.1",
      "tested": 65535, "untested": 0,
      "open": 120, "rejected": 400, "dropped": 65000, "interfered": 15, "errors": 0,
      "late": 0, "retries": 1, "timeout_ms": 250,
      "open_rtt_ms": { "min": 21.3, "median": 24.8, "p95": 40.2, "max": 95.0 },
      "ranges": [
        { "first": 1, "last": 21, "status": "dropped" },
        { "first": 22, "last": 22, "status": "open" }
      ]
    }
  ]
}
```

`client` is the client that ran the scan (`cli`, `android` or `browser`), `label` the name given with `-label`, if any, `untested` counts the ports left when a scan is stopped early, `late` the open ports found only by a recheck with the full timeout (see below), and `options.retries` is -1 for the default.

### Comparing runs

`-diff` compares two JSON reports, such as the same network before and after a firewall change, or two networks, and lists the ports whose result changed from the first to the second. Reports from any client can be compared, including the Android app's and the browser page's. Kinds are matched by protocol and IP version, so `tcp` tested over IPv4 in one report is compared with `tcp4` in the other:

```shell
$ ./portquiz -json home.json -label "home wifi" portquiz.example.com
$ ./portquiz -json office.json -label office portquiz.example.com
$ ./portquiz -diff home.json office.json
A: portquiz.example.com, 2026-10-13 09:30, cli, "home wifi"
B: portquiz.example.com, 2026-10-14 10:00, cli, "office"
tcp: 2 opened, 340 closed, 1 changed reason, 65192 unchanged
  22 dropped -> open
  25 rejected -> interfered
  443 dropped -> open
  8000-8339 open -> dropped
```

A port is **opened** if it was closed in the first report and open in the second, **closed** if the reverse, and **changed reason** if it was closed in both but differently, such as rejected and then dropped. Ports tested in only one of the reports are counted as not compared, and a kind tested in only one is shown as `only in A` or `only in B`. The browser page cannot tell how a UDP port was closed, so its closed UDP ports only count as changed when they open.

The exit status is 0 if no port changed, 1 if some did and 2 on errors, so `-diff` can check a network against a known good report from a script. With `-json file` it also writes the comparison as JSON (`-json -` writes only the JSON, to stdout): the two runs, and per kind the counts (`opened`, `closed`, `reason_changed`, `unchanged`, `only_a`, `only_b`), the open ports and median response time of each, the changed ports as `changes` (`{"first": 8000, "last": 8339, "a": "open", "b": "dropped"}`), and every port tested in either report as `ranges` of consecutive ports with the same change, for drawing a map.

The Android app and the browser page keep their last 20 runs and compare any two of them the same way, with a map of the changes. Both can import reports from any client, so a scan from a laptop can be compared with one from a phone.

### Timeouts

A port that gets no response is reported closed once its timeout expires, so on networks that silently drop traffic the timeout decides how long a scan takes. By default the client measures how quickly open ports respond and waits a safe multiple of that, at least 250ms and at most `-timeout` (2s). Until the first open port responds, the full `-timeout` is used.

As a safety check, one in every 20 ports that time out is tested again with the full `-timeout`. If such a port turns out to be open, the adaptive timeout is raised for the rest of the scan and a warning is printed at the end, since ports tested earlier may have been reported closed too early. Use `-adaptive=false` to always wait the full `-timeout`.

Ports that time out are retried once by default, since packets can be lost. The exception is TCP with `-adaptive=false`: the operating system resends a lost connection request after about a second, which the full 2s timeout already covers, so those ports are not retried. Refused ports are never retried. Use `-retry` to change this.

### Example Client

```shell
# test UDP and TCP ports, only print open ports
./portquiz -tcp -udp -open portquiz.example.com
```

## Browser Test

The server also serves a test page for checking ports from a browser. It needs nothing installed, so it works on phones, including iPhones, and on computers where you cannot install the client. It can be used without SSL, with nothing to set up, or with SSL, given a DNS name for the server. Use `-web=false` on the server to turn it off; the client and the Android app do not need it.

### Without SSL (HTTP)

Nothing to set up: open `http://<server>/` in a browser, by name or IP address, and press Start. Any port works, such as `http://<server>:8080/`, since the server answers on every port. TCP ports are then tested with plain web requests.

Some browsers warn before opening a plain `http://` page, or refuse to when set to always use HTTPS; allow it for the server, or use HTTPS.

### With SSL (HTTPS)

The server can get a certificate from Let's Encrypt for its DNS name, renew it, and serve the page at `https://<name>/` as well, with plain HTTP still working. It needs:

- **A DNS name** for the server. Certificates are requested for names, and a browser asks for one by name, so a server reached only by IP address stays on HTTP.
- **DNS records that point at the server's `-listen` addresses.** The name's A record must be one of them. If the name also has an AAAA (IPv6) record, that address must be listened on too, since Let's Encrypt checks over IPv6 when it can.
- **The server reachable from the internet on port 80 or 443**, which Let's Encrypt connects to for its check. The server answers on every port, so this is already the case unless a firewall or router in front of it blocks those ports.

To set it up:

1. Start the server with `-acme <name>`, or on Alpine add it to `PORTQUIZ_OPTS` in `/etc/conf.d/portquiz` and restart the service:

   ```shell
   ./portquiz-server -listen 192.0.2.123 -acme portquiz.example.com
   ```

   ```shell
   # /etc/conf.d/portquiz
   PORTQUIZ_OPTS="-acme portquiz.example.com"
   ```

2. Within a few seconds the log, `/var/log/portquiz.log` for the service, shows `certificate for portquiz.example.com ready, serving https://portquiz.example.com/`. If it shows `getting a certificate for ...` with an error instead, see [Troubleshooting](#troubleshooting).
3. Open `https://<name>/` and press Start.

The certificate and the Let's Encrypt account are kept in `-acme-cache`, `/var/lib/portquiz/acme` by default, and the certificate is renewed while the server runs. `-acme-email` is optional, for Let's Encrypt's notices. Several names can be given, separated by commas. To try the setup without counting against Let's Encrypt's limits, add `-acme-directory https://acme-staging-v02.api.letsencrypt.org/directory`: it checks the same things, but browsers do not trust its certificates, so remove it, and delete `-acme-cache`, once it works.

With a certificate, the server answers HTTPS on every port as well as plain HTTP and the client's password, telling them apart by how each connection starts, so the HTTPS page can test every port with encrypted requests while `http://` keeps working. The server never tells browsers to always use HTTPS for it, so the plain page stays usable. Each page links to the other.

Once the server has a certificate, browsers such as Chrome try HTTPS first even for `http://` links, so `http://<name>/` opens the HTTPS page. To open the plain page, use `http://<name>/?http`, as the HTTPS page's link does: the server sends that address straight back to HTTP when it arrives over HTTPS, which browsers take as the sign to fall back to HTTP. Browsers never upgrade a bare IP address, so `http://<ip>/` always opens the plain page too.

### What a browser can test

A browser cannot open raw connections, so the page tests TCP ports with web requests and UDP ports with STUN requests, which browsers send for WebRTC to learn their public address. The server answers both on every port, alongside the client's password. That makes it a quick check rather than a full test:

- **TCP ports browsers refuse**: browsers refuse to connect to about 80 TCP ports, including 21 (FTP), 22 (SSH), 25 (SMTP), 53 (DNS), 110, 143, 465, 587, 993 and 995, so those cannot be tested. The page works out which ports the browser refuses while it runs, so it keeps up as browsers change their lists: the browser refuses such a port at once, for any address, so a port that fails quickly against the server is tried against a documentation address that never answers (192.0.2.1, or 2001:db8::1 for IPv6). Failing at once there too means the browser refused it. Where that address does not behave that way, the page uses the [Fetch standard's list](https://fetch.spec.whatwg.org/#bad-port) instead and says so. A network that rejects a port for every destination looks the same as the browser refusing it.
- **UDP**: choose UDP next to TCP. A UDP port either answers or not: a closed port gives a browser no sign of why, so rejected and dropped show alike as no answer. Browsers only allow a page a few hundred WebRTC connections, so the page reuses 32 of them, each pointed at one port after another, and a full UDP test of a network that drops most UDP takes a while. If no UDP port answers at all, either the network blocks UDP to the server, or the browser hides WebRTC answers, as some privacy settings and extensions do.
- **Web traffic**: it shows whether web requests on a port reach the server. A filter that inspects traffic may treat them differently from other traffic, and a proxy that passes web requests through makes a port look open.
- **Closed ports**: rejected, dropped and interfered are told apart as well as a browser allows: a refused connection and an answer that is not the server's both fail the same way, so a second request checks whether anything answered.
- **HTTP or HTTPS**: a page opened over `http://` tests TCP ports with plain web requests. A page opened over `https://` may only make HTTPS requests, so it tests TCP ports with encrypted requests instead. The HTTPS page tests something slightly different: whether encrypted web traffic gets through on each port. A proxy that steps into plain web requests cannot step into encrypted ones, so ports it would interfere with look open, and a network that intercepts encrypted traffic shows ports as rejected rather than interfered, unless the device trusts it. HTTPS requests also take one more round trip than plain ones. UDP is tested the same way from both.
- **Speed**: browsers limit how many connections a page opens, so it tests 128 TCP and 32 UDP ports at once by default, sharing the 500 connections per second limit, and they slow down background tabs, so keep the tab open.

The summary at the end matches the Android app's, and Save JSON saves the results in the same format as the client's [JSON report](#json-report), with `"client": "browser"`, the address the server saw the browser connect from in `"client_address"`, `"tcp_requests"` saying whether TCP was tested with `http` or `https` requests, a `browser` status and a `browser_blocked` count for the ports the browser refused, and how they were detected in `browser_blocked_detection`.

Past runs lists the last 20 runs, kept in the browser's storage on that device only; a page opened over `http://` and one opened over `https://` each keep their own. Check two to [compare them](#comparing-runs), name a run (such as "home wifi") with Rename, save its report again, or import reports saved by any portquiz client to compare with.

## How It Works

1. **Server Setup**: The server listens on a single port and uses iptables DNAT rules to redirect traffic from all ports to this listening port
2. **Client Testing**: The client attempts to connect to each port and sends a probe: the password (`-password`, `portquiz` by default), a zero byte and a random tag, so no two probes are alike
3. **Response Validation**: The server sends the probe back only if the password matches exactly, so the client can tell the answer is from the server and is to that probe. Clients from before tags send the bare password, which the server answers with the password, and newer clients accept that answer from servers from before tags
4. **Protocol Detection**: Can detect DPI firewalls that block connections based on protocol patterns

## Troubleshooting

**Client shows all ports as closed:**

- Verify the server is running and accessible
- Check that firewall rules are properly configured
- Ensure the magic string matches between client and server

**Server setup fails:**

- Verify you have root privileges
- Check that iptables is installed and available
- Ensure the listening IP is correctly configured on the system

**The browser test page does not load:**

- Use `http://`, not `https://`, unless the server was started with `-acme` (see [Browser Test](#browser-test))
- Check the server was not started with `-web=false`
- If the browser refuses plain `http://` pages, allow it for the server or set up [HTTPS](#with-ssl-https)

**No HTTPS certificate:**

The log shows `getting a certificate for <name>:` and Let's Encrypt's reason. The usual causes:

- The name's A or AAAA record points somewhere other than the server's `-listen` addresses; check both with `dig A <name>` and `dig AAAA <name>`
- Ports 80 and 443 are blocked before reaching the server, by a cloud firewall or a router that does not forward them
- Too many attempts: Let's Encrypt limits failed checks and certificates per name, so test with the staging server first (see [With SSL](#with-ssl-https)) and wait an hour after repeated failures

The server keeps serving plain HTTP meanwhile, and tries again on the next HTTPS visit or restart.

**Performance is slow:**

A scan is limited by `-rate`, the number of connections started per second (500 by default, about 2 minutes for all ports of one protocol), and by ports that are blocked by a firewall which silently drops traffic, since each of those waits out a timeout (see [Timeouts](#timeouts)).

- Test specific ports instead of all ports with `-port`
- Raise `-rate`, or use `-rate 0` for no limit. Every connection takes an entry in the connection tracking table of each NAT router or firewall along the way, often for a minute or two, and a full table can disrupt other traffic through it, so only raise it on networks that can handle it
- Raise `-parallel` if a scan of a filtered network is limited by waiting rather than by `-rate`
- Lower `-timeout` and `-retry` on fast, reliable networks

**Results are inconsistent between runs (especially UDP):**

- Increase `-retry`, since UDP packets can be lost
- Lower `-rate`
- Use `-adaptive=false` if a warning says the adaptive timeout missed ports
