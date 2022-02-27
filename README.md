# weron

![Logo](./assets/logo-readme.png)

Overlay networks based on WebRTC.

⚠️ weron has not yet been audited! While we try to make weron as secure as possible, it has not yet undergone a formal security audit by a third party. Please keep this in mind if you use it for security-critical applications. ⚠️

[![hydrun CI](https://github.com/pojntfx/weron/actions/workflows/hydrun.yaml/badge.svg)](https://github.com/pojntfx/weron/actions/workflows/hydrun.yaml)
[![Docker CI](https://github.com/pojntfx/weron/actions/workflows/docker.yaml/badge.svg)](https://github.com/pojntfx/weron/actions/workflows/docker.yaml)
[![Matrix](https://img.shields.io/matrix/weron:matrix.org)](https://matrix.to/#/#weron:matrix.org?via=matrix.org)
[![Binary Downloads](https://img.shields.io/github/downloads/pojntfx/weron/total?label=binary%20downloads)](https://github.com/pojntfx/weron/releases)

## Overview

weron provides lean, fast & secure layer 2 overlay networks based on WebRTC.

It enables you too ...

- **Access to nodes behind NAT**: Because weron uses WebRTC to establish connections between nodes, it can easily traverse corporate firewalls and NATs using STUN, or even use a TURN server to tunnel traffic. This can be very useful to i.e. SSH into your homelab without forwarding any ports on your router.
- **Secure your home network**: By using the inbuilt interactive TLS verification and running the signaling server locally, weron can be used to secure traffic between nodes in a LAN without depending on any external infrastructure.
- **Join local nodes into a cloud network**: If you run e.g. a Kubernetes cluster with nodes based on cloud instances but also want to join your on-prem nodes into it, you can use weron to create a trusted network for it.

## Usage

### 1. Starting the Signaling Server

The signaling server allows agents to connect to each other by exchanging their connection information. You can either use the publicly hosted signaling server at `wss://weron.herokuapp.com/`, or host it yourself:

<details>
  <summary>Option 1: Starting the signaling server using Podman (recommended)</summary>

Run the following:

```shell
$ sudo podman run -d --restart=always --label "io.containers.autoupdate=image" -p 15325:15325 --name weron-signaler ghcr.io/pojntfx/weron /usr/local/bin/weron signal
$ sudo podman generate systemd --new weron-signaler | sudo tee /lib/systemd/system/weron-signaler.service

$ sudo systemctl daemon-reload
$ sudo systemctl enable --now weron-signaler
```

The signaling service should now be reachable on port `15325` from all network interfaces.

</details>

<details>
  <summary>Option 2: Starting the signaling server using systemd</summary>

Run the following:

```shell
$ sudo tee /lib/systemd/system/weron-signaler.service <<'EOT'
[Unit]
Description=weron-signaler

[Service]
ExecStart=/usr/local/bin/weron signal

[Install]
WantedBy=multi-user.target
EOT

$ sudo systemctl daemon-reload
$ sudo systemctl enable --now weron-signaler
```

The signaling service should now be reachable on port `15325` from all network interfaces.

</details>

<details>
  <summary>Option 3: Starting the signaling server natively</summary>

Run the following:

```shell
$ weron signal
2022/02/27 18:23:15 Signaler listening on :15325
2022/02/27 18:23:15 TLS certificate SHA-1 fingerprint: CA:BC:CA:80:C4:14:8B:46:F2:5A:43:D2:8E:BD:40:D7:EC:25:00:9A
```

The signaling service should now be reachable on port `15325` from all network interfaces.

</details>

### 2. Starting the Agent

The agent connects to the signaling server, which it uses to connect to other agents using WebRTC. Please adjust the values below to match your use case. To allocate an IP address, you can replace `weron join` with any of the following:

- `weron join ip addr add fd00::/8 dev` (allocate an IPv6 address statically using `iproute2`)
- `weron join ip addr add 10.0.0.1/8 dev` (allocate an IPv4 address statically using `iproute2`)
- `weron join avahi-autoipd` (allocate an IPv4 address dynamically using `avahi-autoipd` (IPv4LL))

<details>
  <summary>Option 1: Starting the agent using Podman (recommended)</summary>

Run the following:

```shell
$ sudo podman run -d --restart=always --label "io.containers.autoupdate=image" --name weron-agent --cap-add NET_ADMIN -e WERON_RADDR='wss://weron.herokuapp.com/' -e WERON_COMMUNITY='test' -e WERON_KEY='0123456789101112' ghcr.io/pojntfx/weron /usr/local/bin/weron join
$ sudo podman generate systemd --new weron-agent | sudo tee /lib/systemd/system/weron-agent.service

$ sudo systemctl daemon-reload
$ sudo systemctl enable --now weron-agent
```

The agent should now connect to other agents in the community.

</details>

<details>
  <summary>Option 2: Starting the agent using systemd</summary>

Run the following:

```shell
$ sudo tee /lib/systemd/system/weron-agent.service <<'EOT'
[Unit]
Description=weron-agent

[Service]
ExecStart=/usr/local/bin/weron join
Environment=WERON_RADDR='wss://weron.herokuapp.com/'
Environment=WERON_COMMUNITY='test'
Environment=WERON_KEY='0123456789101112'

[Install]
WantedBy=multi-user.target
EOT

$ sudo systemctl daemon-reload
$ sudo systemctl enable --now weron-agent
```

The agent should now connect to other agents in the community.

</details>

<details>
  <summary>Option 3: Starting the agent natively</summary>

Run the following:

```shell
$ weron join --raddr wss://weron.herokuapp.com/ --community test --key 0123456789101112
2022/02/27 19:11:57 Agent connecting to signaler wss://weron.herokuapp.com/
2022/02/27 19:12:01 Agent connected to signaler wss://weron.herokuapp.com/
```

The agent should now connect to other agents in the community.

</details>

## Installation

### Containerized

You can get the OCI image like so:

```shell
$ podman pull ghcr.io/pojntfx/weron
```

### Natively

If you prefer a native installation, static binaries are available on [GitHub releases](https://github.com/pojntfx/weron/releases).

On Linux, you can install them like so:

```shell
$ curl -L -o /tmp/weron "https://github.com/pojntfx/weron/releases/latest/download/weron.linux-$(uname -m)"
$ sudo install /tmp/weron /usr/local/bin
$ sudo setcap cap_net_admin+ep /usr/local/bin/weron # This allows rootless execution
```

On Windows, the following should work (using PowerShell as administrator; install TAP-Windows first):

```shell
PS> Invoke-WebRequest https://github.com/pojntfx/weron/releases/latest/download/weron.windows-x86_64.exe -OutFile \Windows\System32\weron.exe
```

You can find binaries for more operating systems and architectures on [GitHub releases](https://github.com/pojntfx/weron/releases).

## Reference

### Command Line Arguments

```shell
$ weron --help
weron provides lean, fast & secure layer 2 overlay networks based on WebRTC.

Find more information at:
https://github.com/pojntfx/weron.

Usage:
  weron [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  join        Join a community
  signal      Start a signaling server

Flags:
  -h, --help               help for weron
  -m, --timeout duration   Duration between reconnects and pings (default 5s)
  -v, --verbose            Enable verbose logging

Use "weron [command] --help" for more information about a command.
```

### Environment Variables

All command line arguments described above can also be set using environment variables; for example, to set `--tls-fingerprint` to `CA:BC:CA:80:C4:14:8B:46:F2:5A:43:D2:8E:BD:40:D7:EC:25:00:9A` with an environment variable, use `WERON_TLS_FINGERPRINT=CA:BC:CA:80:C4:14:8B:46:F2:5A:43:D2:8E:BD:40:D7:EC:25:00:9A`.

## Acknowledgements

- [songgao/water](https://github.com/songgao/water) provides the TAP device library for weron.
- [pion/webrtc](https://github.com/pion/webrtc) provides the WebRTC functionality.
- All the rest of the authors who worked on the dependencies used! Thanks a lot!

## Contributing

To contribute, please use the [GitHub flow](https://guides.github.com/introduction/flow/) and follow our [Code of Conduct](./CODE_OF_CONDUCT.md).

To build and start a development version of weron locally, run the following:

```shell
$ git clone https://github.com/pojntfx/weron.git
$ cd weron
$ make depend
$ make && sudo make install
$ weron signal # Starts the signaling server
# In another terminal
$ weron join --community test --key 0123456789101112 --raddr wss://localhost:15325 # Starts the first agent; append `-e=true sudo avahi-autoipd` to automatically assign a IPv4 address to the interface using IPv4LL
$ weron join --community test --key 0123456789101112 --raddr wss://localhost:15325 # Starts the second agent; append `-e=true sudo avahi-autoipd` to automatically assign a IPv4 address to the interface using IPv4LL
```

Both the signaling server and two agents should now be running and have MAC (or, if you decided to use `avahi-autoipd`, also IP) addresses.

Have any questions or need help? Chat with us [on Matrix](https://matrix.to/#/#weron:matrix.org?via=matrix.org)!

## License

weron (c) 2022 Felix Pojtinger and contributors

SPDX-License-Identifier: AGPL-3.0
