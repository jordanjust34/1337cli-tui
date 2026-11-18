#!/bin/sh
# Installs 1337cli on this machine. Run from the unpacked release folder:
#   ./install.sh
set -eu
cd "$(dirname "$0")"

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  armv7l|armv6l) arch=arm ;;
  *) echo "unsupported CPU: $(uname -m)" >&2; exit 1 ;;
esac
for name in 1337cli 1337tui; do
  [ -f "bin/$name-linux-$arch" ] || { echo "missing bin/$name-linux-$arch" >&2; exit 1; }
done

# System-wide if possible (sudo may prompt for a password), otherwise per-user.
# Both binaries are independent; each reads the config file below.
if [ -w /usr/local/bin ]; then
  dir=/usr/local/bin; sudo=
elif command -v sudo >/dev/null 2>&1; then
  dir=/usr/local/bin; sudo=sudo
else
  dir="$HOME/.local/bin"; sudo=
  mkdir -p "$dir"
  case ":$PATH:" in *":$dir:"*) ;; *) echo "note: add ~/.local/bin to your PATH" ;; esac
fi
for name in 1337cli 1337tui; do
  $sudo install -m 755 "bin/$name-linux-$arch" "$dir/$name"
  echo "installed $dir/$name"
done

conf="${XDG_CONFIG_HOME:-$HOME/.config}/1337cli/config"
if [ ! -f "$conf" ]; then
  mkdir -p "$(dirname "$conf")"
  install -m 600 config.example "$conf"
  echo "created $conf - edit it to set your qBittorrent login"
else
  echo "kept existing $conf"
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found: install Docker, then run 'docker compose up -d' here to start FlareSolverr"
elif docker info >/dev/null 2>&1; then
  docker compose up -d && echo "FlareSolverr running on http://localhost:8191"
elif command -v sudo >/dev/null 2>&1; then
  # This user can't reach the Docker daemon (not in the docker group); use sudo.
  echo "no access to Docker as $(id -un); starting FlareSolverr with sudo"
  sudo docker compose up -d && echo "FlareSolverr running on http://localhost:8191"
  echo "tip: 'sudo usermod -aG docker $(id -un)' then log back in to use docker without sudo"
else
  echo "no permission to use Docker: run 'docker compose up -d' here as root to start FlareSolverr"
fi
