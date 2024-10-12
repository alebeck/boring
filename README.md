# The `boring` tunnel manager

![Static Badge](https://img.shields.io/badge/build-passing-4CC525?) ![GitHub Release](https://img.shields.io/github/v/release/alebeck/boring?color=orange) [![Go Report Card](https://goreportcard.com/badge/github.com/alebeck/boring)](https://goreportcard.com/report/github.com/alebeck/boring)
 ![Static Badge](https://img.shields.io/badge/license-MIT-blue?)

A simple & reliable command line SSH tunnel manager.

![Screenshot](./assets/dark.gif)

## Features
* Ultra lightweight and fast
* Local and remote tunnels
* Compatible with SSH config and `ssh-agent`
* Supports Unix sockets
* Automatic reconnection
* Human-friendly configuration via TOML

## Usage
```
Usage:
  boring list,l                        List tunnels
  boring open,o <name1> [<name2> ...]  Open specified tunnel(s)
  boring close,c <name1> [<name2> ...] Close specified tunnel(s)
```

## Configuration

The config file can be created in the locations below in order of precedence:
1. The `BORING_CONFIG` environmental variable can be used to set a custom path
2. `$XDG_CONFIG_HOME/boring/boring.toml`
3. `~/.boring.toml`

The configuration is a simple TOML file describing your tunnels:

```toml
# simple tunnel
[[tunnels]]
name = "dev"
local = "9000"
remote = "localhost:9000"
host = "dev-server"  # automatically matches host against SSH config

# example of an explicit host (no SSH config)
[[tunnels]]
name = "prod"
local = "5001"
remote = "localhost:5001"
host = "prod.example.com"
user = "root"
identity = "~/.ssh/id_prod"  # will try default ones if not set

# ... more tunnels
```

Currently, supported options are: `name`, `local`, `remote`, `host`, `user`, `identity`, `port`, and `mode`. `host` either describes a host which to match SSH configs to, or if no matches found, the actual hostname. `mode` can be 'local' for local or 'remote' for remote forwarding, default is 'local'. The location of the config file can be changed by setting the `BORING_CONFIG` environment variable.


## Installation
Get one of the pre-built binaries from the [releases page](https://github.com/alebeck/boring/releases) or build it yourself:

```sh
git clone https://github.com/alebeck/boring && cd boring
./build.sh
```
Currently only supports macOS and Linux.
