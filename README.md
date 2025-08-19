# portquiz

Test all outbound TCP/UDP ports for connectivity to a remote host.

Testing all ports can take 3-10 minutes depending on connection speed.

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
# Test connectivity to a server (most common use case)
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
        comma separated list of IPs to listen on (default "127.0.0.123")
  -no-iptables
        disable automatically creating iptables rules
  -password string
        magicString to use, must be the same on client/server (default "portquiz")
  -port uint
        default port to listen on which will have traffic redirected to (default 1337)
  -tcp
        start TCP server
  -timeout duration
        amount of time for each connection (default 10s)
  -udp
        start UDP server
  -verbose
        enable verbose logging
```

### Example Server

```shell
# start server
# listen on TCP and UDP ports
# listens on IPv4: 192.0.2.123 and IPv6: 2001:0DB8::1
./portquiz-server -tcp -udp -listen 192.0.2.123,2001:0DB8::1
```

## Client

The portquiz client connects to the portquiz server and tests port connectivity. By default portquiz will test all ports unless `-port` is specified.

**Note:** The client binary is named `portquiz` (cross-platform), while the server binary is `portquiz-server` (Linux only).

IPv4 can be forced with `-4` and IPv6 can be forced with `-6`. If both are provided (`-4 -6`) then each port is tested using both IPv4 and IPv6. If unspecified, only one protocol version is tested.

```shell
$ ./portquiz -h
Usage of ./portquiz:
  -4    force IPv4
  -6    force IPv6
  -closed
        print only closed ports
  -multi uint
        test multiple times to ensure larger streams work (default 1)
  -open
        print only open ports
  -parallel uint
        number of worker threads (default 20)
  -password string
        magicString to use, must be the same on client/server (default "portquiz")
  -port string
        comma separated list of ports to test
  -retry uint
        retry count (default 3)
  -tcp
        start TCP client
  -timeout duration
        amount of time for each connection (default 5s)
  -udp
        start UDP client
  -verbose
        enable verbose logging
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
- Reduce parallelism with `-parallel` flag
- Test specific ports instead of all ports
- Adjust timeout values for faster networks
