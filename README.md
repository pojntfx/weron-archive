# weron

Overlay networks based on WebRTC.

## Overview

🚧 This project is a work-in-progress! Instructions will be added as soon as it is usable. 🚧

## Installation

### Containerized

You can get the Docker container like so:

```shell
$ docker pull pojntfx/weron
```

### Natively

If you prefer a native installation, static binaries are also available on [GitHub releases](https://github.com/pojntfx/weron/releases).

You can install them like so:

```shell
$ curl -L -o /tmp/weron https://github.com/pojntfx/weron/releases/latest/download/weron.linux-$(uname -m)
$ sudo install /tmp/weron /usr/local/bin
$ sudo setcap cap_net_admin+ep /usr/local/bin/weron # This allows rootless execution
```

## License

weron (c) 2021 Felicitas Pojtinger and contributors

SPDX-License-Identifier: AGPL-3.0
