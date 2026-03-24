#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUNTIME_BIN="$ROOT_DIR/runtime/bin"
RUNTIME_LIB="$ROOT_DIR/runtime/lib"

mkdir -p "$RUNTIME_BIN" "$RUNTIME_LIB"

BINARIES=(
  qemu-img
  qemu-nbd
  partprobe
  lsblk
  blkid
  mount
  umount
  pvs
  lvs
  vgchange
)

copy_binary() {
  local name="$1"
  local path
  path="$(command -v "$name")"
  cp -L "$path" "$RUNTIME_BIN/$name"

  if command -v ldd >/dev/null 2>&1; then
    ldd "$path" | while read -r a b c; do
      lib=""
      if [[ "$b" == "=>" ]]; then
        lib="$c"
      elif [[ "$a" == /* ]]; then
        lib="$a"
      fi
      if [[ -n "$lib" && -f "$lib" ]]; then
        cp -Ln "$lib" "$RUNTIME_LIB/" 2>/dev/null || true
      fi
    done
  fi
}

for binary in "${BINARIES[@]}"; do
  if command -v "$binary" >/dev/null 2>&1; then
    echo "packaging $binary"
    copy_binary "$binary"
  else
    echo "skip missing $binary" >&2
  fi
done

echo "runtime packaged under $ROOT_DIR/runtime"
