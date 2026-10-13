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
- `PORTQUIZ_OPTS`: extra arguments for `portquiz-server`, for example `-udp` to serve only UDP.
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
  -format string
        output format: "ranges" merges consecutive ports with the same result, "ports" prints one line per port (default "ranges")
  -json file
        when testing finishes, write a JSON report of every result to file, or to stdout with - instead of the usual results
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
  "version": "v1.6.0",
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

`untested` counts the ports left when a scan is stopped early, `late` the open ports found only by a recheck with the full timeout (see below), and `options.retries` is -1 for the default.

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

The server also serves a test page: open `http://<server>/` in any browser, on any port, and press Start. It needs nothing installed, so it works on phones, including iPhones, and on computers where you cannot install the client. Use `-web=false` on the server to turn it off; the client and the Android app do not need it.

A browser cannot open raw connections, so the page tests TCP ports with web requests and UDP ports with STUN requests, which browsers send for WebRTC to learn their public address. The server answers both on every port, alongside the client's password. That makes it a quick check rather than a full test:

- **TCP ports browsers refuse**: browsers refuse to connect to about 80 TCP ports, including 21 (FTP), 22 (SSH), 25 (SMTP), 53 (DNS), 110, 143, 465, 587, 993 and 995, so those cannot be tested. The page works out which ports the browser refuses while it runs, so it keeps up as browsers change their lists: the browser refuses such a port at once, for any address, so a port that fails quickly against the server is tried against a documentation address that never answers (192.0.2.1, or 2001:db8::1 for IPv6). Failing at once there too means the browser refused it. Where that address does not behave that way, the page uses the [Fetch standard's list](https://fetch.spec.whatwg.org/#bad-port) instead and says so. A network that rejects a port for every destination looks the same as the browser refusing it.
- **UDP**: choose UDP next to TCP. A UDP port either answers or not: a closed port gives a browser no sign of why, so rejected and dropped show alike as no answer. Browsers only allow a page a few hundred WebRTC connections, so the page reuses 32 of them, each pointed at one port after another, and a full UDP test of a network that drops most UDP takes a while. If no UDP port answers at all, either the network blocks UDP to the server, or the browser hides WebRTC answers, as some privacy settings and extensions do.
- **Web traffic**: it shows whether web requests on a port reach the server. A filter that inspects traffic may treat them differently from other traffic, and a proxy that passes web requests through makes a port look open.
- **Closed ports**: rejected, dropped and interfered are told apart as well as a browser allows: a refused connection and an answer that is not the server's both fail the same way, so a second request checks whether anything answered.
- **Plain HTTP**: the page must be opened over `http://`, since a secure page cannot make plain requests, and browsers set to always upgrade to HTTPS need an exception for the server.
- **Speed**: browsers limit how many connections a page opens, so it tests 128 TCP and 32 UDP ports at once by default, sharing the 500 connections per second limit, and they slow down background tabs, so keep the tab open.

The summary at the end matches the Android app's, and Save JSON saves the results in the same format as the client's [JSON report](#json-report), with `"client": "browser"`, a `browser` status and a `browser_blocked` count for the ports the browser refused, and how they were detected in `browser_blocked_detection`.

## How It Works

1. **Server Setup**: The server listens on a single port and uses iptables DNAT rules to redirect traffic from all ports to this listening port
2. **Client Testing**: The client attempts to connect to each port and sends a magic string
3. **Response Validation**: The server responds with the same magic string if the connection is successful
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
