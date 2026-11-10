#!/bin/bash
set -euo pipefail

source_dir="$(cd -- "$(dirname -- "$0")" && pwd)"
target="${1:-$source_dir/dist/小小.app}"
architecture="${2:-native}"

if [[ -e "$target" || -L "$target" ]]; then
    printf '输出位置已经有文件：%s\n' "$target" >&2
    exit 1
fi
if [[ "$(uname -s)" != Darwin ]] || ! developer_dir="$(xcode-select -p 2>/dev/null)"; then
    printf '构建需要 macOS 和 Apple 命令行工具。\n' >&2
    exit 1
fi

case "$architecture" in
    native) architectures=("$(uname -m)") ;;
    universal) architectures=(arm64 x86_64) ;;
    *) printf '架构请选 native 或 universal。\n' >&2; exit 2 ;;
esac

sdk="${SDKROOT:-$developer_dir/SDKs/MacOSX.sdk}"
if [[ ! -d "$sdk" ]]; then sdk="$(xcrun --sdk macosx --show-sdk-path)"; fi
scratch="$(mktemp -d)"
trap 'rm -rf -- "$scratch"' EXIT
bundle="$scratch/小小.app"
mkdir -p "$bundle/Contents/"{MacOS,Resources}
binaries=()

for arch in "${architectures[@]}"; do
    binary="$scratch/Xiaoxiao-$arch"
    swiftc -O -module-cache-path "$scratch/modules-$arch" -sdk "$sdk" \
        -target "$arch-apple-macosx14.0" "$source_dir/Pet.swift" -o "$binary"
    binaries+=("$binary")
done
if [[ "${#binaries[@]}" == 1 ]]; then
    mv -- "${binaries[0]}" "$bundle/Contents/MacOS/Xiaoxiao"
else
    lipo -create "${binaries[@]}" -output "$bundle/Contents/MacOS/Xiaoxiao"
fi

cp "$source_dir/Info.plist" "$bundle/Contents/"
cp "$source_dir/LICENSE" "$bundle/Contents/Resources/"
cp "$source_dir"/motions/{resting,sitting,watching,grooming,belly,tail,sleeping,walking,sprinting,spinning,scaling}.png \
    "$bundle/Contents/Resources/"
codesign --force --sign - "$bundle"
codesign --verify --strict "$bundle"
mkdir -p -- "$(dirname -- "$target")"
mv -- "$bundle" "$target"
printf '已构建：%s\n' "$target"
