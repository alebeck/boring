# `boring` SSH tunnel manager

TODO screenshot

## Features
* No bells and whistles
* Ultra-leightweight and fast
* Compatible with `.ssh/config` and `ssh-agent`
* Local and remote tunnels
* Supports Unix sockets
* Automatic reconnection
* Human-friendly configuration via TOML

## Usage

## Configuration
`boring` reads its configuration from `~/.config/boring/config.toml`. The configuration is a simple TOML file with the following structure:

```toml
[[tunnels]]
name = "dev"
local = "8888"
remote = "localhost:8888"
host = "dev-server"  # automatically matches hosts against ~/.ssh/config
user = "root"  # optional, overrides matched value
...

[[tunnels]]
... more tunnels
```
