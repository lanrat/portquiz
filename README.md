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

## Installation

### [Download Latest Release](https://github.com/lanrat/portquiz/releases)

**For testing connectivity (any platform):**

- Download `portquiz` for your platform (Windows, macOS, Linux)

**For running a server (Linux only):**

- Download `portquiz-server` for Linux

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
```

### Example Server

```shell
# start server
# serves TCP and UDP (the default when neither -tcp nor -udp is given)
# listens on IPv4: 192.0.2.123 and IPv6: 2001:0DB8::1
./portquiz-server -listen 192.0.2.123,2001:0DB8::1
```

## Client

The portquiz client connects to the portquiz server and tests port connectivity. By default portquiz will test all ports unless `-port` is specified.

**Note:** The client binary is named `portquiz` (cross-platform), while the server binary is `portquiz-server` (Linux only).

IPv4 can be forced with `-4` and IPv6 can be forced with `-6`. If both are provided (`-4 -6`) then each port is tested using both IPv4 and IPv6. If unspecified, only one protocol version is tested.

When run in a terminal, a progress bar with an estimated time remaining is shown on stderr, followed by a summary when testing finishes. It is hidden automatically when stderr is not a terminal (for example when piped or redirected) and in `-verbose` mode, and can be turned off with `-no-progress`. Results are always printed to stdout, so `./portquiz -tcp example.com > results.txt` still shows the progress bar while saving only the results.

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

### Timeouts

A port that gets no response is reported closed once its timeout expires, so on networks that silently drop traffic the timeout decides how long a scan takes. By default the client measures how quickly open ports respond and waits a safe multiple of that, at least 250ms and at most `-timeout` (2s). Until the first open port responds, the full `-timeout` is used.

As a safety check, one in every 20 ports that time out is tested again with the full `-timeout`. If such a port turns out to be open, the adaptive timeout is raised for the rest of the scan and a warning is printed at the end, since ports tested earlier may have been reported closed too early. Use `-adaptive=false` to always wait the full `-timeout`.

Ports that time out are retried once by default, since packets can be lost. The exception is TCP with `-adaptive=false`: the operating system resends a lost connection request after about a second, which the full 2s timeout already covers, so those ports are not retried. Refused ports are never retried. Use `-retry` to change this.

### Example Client

```shell
# test UDP and TCP ports, only print open ports
./portquiz -tcp -udp -open portquiz.example.com
```

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
