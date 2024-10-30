# The `boring` tunnel manager

![Static Badge](https://img.shields.io/badge/build-passing-4CC525?) ![GitHub Release](https://img.shields.io/github/v/release/alebeck/boring?color=orange) [![Go Report Card](https://goreportcard.com/badge/github.com/alebeck/boring)](https://goreportcard.com/report/github.com/alebeck/boring)
 ![Static Badge](https://img.shields.io/badge/license-MIT-blue?)

A simple & reliable command line SSH tunnel manager.

![Screenshot](./assets/dark.gif)

## Features
* Ultra lightweight and fast
* Local, remote and dynamic (SOCKS5) port forwarding
* Compatible with SSH config and `ssh-agent`
* Supports Unix sockets
* Automatic reconnection and keep-alives
* Human-friendly configuration via TOML

## Usage
```
Usage:
  boring list,l                        List tunnels
  boring open,o <name1> [<name2> ...]  Open specified tunnel(s)
  boring close,c <name1> [<name2> ...] Close specified tunnel(s)
```

## Configuration
By default, `boring` reads its configuration from `~/.boring.toml`. The configuration is a simple TOML file describing your tunnels:

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

Currently, supported options are: `name`, `local`, `remote`, `host`, `user`, `identity`, `port`, and `mode`. `host` either describes a host which to match SSH configs to, or if no matches found, the actual hostname. `mode` can be 'local' for local, 'remote' for remote and "socks" for dynamic forwarding; default is 'local'. The location of the config file can be changed by setting the `BORING_CONFIG` environment variable.


## Installation
Either
* install via homebrew: `brew install boring`,
* get one of the pre-built binaries from the [releases page](https://github.com/alebeck/boring/releases), or
* build it yourself:

  ```sh
  git clone https://github.com/alebeck/boring && cd boring
  ./build.sh
  ```

For the last two options, you then need to move the binary to a location in your `$PATH`.

<details>
  <summary>Note for Windows users</summary>
  Windows is fully supported since release 0.6.0. Users currently have to build from source, which is very easy. Make sure Go >= 1.23.0 is installed and then compile via

  ```batch
  git clone https://github.com/alebeck/boring && cd boring
  .\build_win.bat
  ```

  Then, move the executable to a location in your `%PATH%`.
</details>
