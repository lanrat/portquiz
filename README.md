# portquiz

Test all outbound TCP/UDP ports for connectivity to a remote host.

Testing all ports takes a few minutes when most ports are reachable. Each port blocked by a firewall that silently drops traffic waits out the timeout, so a full scan of a heavily filtered network can take up to about 45 minutes per protocol with the default settings (see [Performance is slow](#troubleshooting)).

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

# Test specific ports only
./portquiz -tcp -port 22,80,443 example.com

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
  -closed
        print only closed ports
  -multi uint
        test multiple times to ensure larger streams work (default 1)
  -no-progress
        disable the progress bar shown when stderr is a terminal
  -open
        print only open ports
  -parallel uint
        number of ports to test at once (default 100)
  -password string
        magicString to use, must be the same on client/server (default "portquiz")
  -port string
        comma separated list of ports to test
  -retry uint
        number of times to retry a port that did not respond (default 1)
  -tcp
        test TCP ports (default if neither -tcp nor -udp is set)
  -timeout duration
        amount of time for each connection (default 2s)
  -udp
        test UDP ports
  -verbose
        enable verbose logging
  -version
        show version information
```

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

Ports that are blocked by a firewall which silently drops traffic are the slow ones: each takes `-timeout` × (`-retry` + 1) to be reported as closed (4 seconds with the defaults).

- Test specific ports instead of all ports with `-port`
- Lower `-timeout` and `-retry` on fast, reliable networks
- Raise `-parallel` to test more ports at once; very high values can cause UDP replies to be dropped or trip firewall rate limits

**Results are inconsistent between runs (especially UDP):**

- Increase `-retry`, since UDP packets can be lost
- Reduce `-parallel`
