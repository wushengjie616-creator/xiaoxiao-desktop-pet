#!/bin/bash
set -euo pipefail

source_dir="$(cd -- "$(dirname -- "$0")" && pwd)"
install_dir="${XIAOXIAO_INSTALL_DIR:-$HOME/Applications}"
destination="$install_dir/小小.app"
packaged_app="$source_dir/小小.app"

fail() {
    printf '%s\n' "$1" >&2
    exit 1
}

[[ "$(uname -s)" == Darwin ]] || fail '小小需要 macOS 14 或更新版本。'
macos_version="$(/usr/bin/sw_vers -productVersion)"
[[ "${macos_version%%.*}" -ge 14 ]] || fail '小小需要 macOS 14 或更新版本。'
mkdir -p -- "$install_dir"
lock="$install_dir/.xiaoxiao-install.lock"
mkdir -- "$lock" 2>/dev/null || fail '另一个小小安装程序可能正在运行，请稍后重试。'
trap 'rm -rf -- "$lock"' EXIT

is_xiaoxiao() {
    [[ ! -L "$1" && -f "$1/Contents/Info.plist" ]] &&
        [[ "$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$1/Contents/Info.plist" 2>/dev/null)" == local.xiaoxiao.desktoppet ]] &&
        /usr/bin/codesign --verify --strict "$1"
}

if [[ -e "$destination" || -L "$destination" ]]; then
    is_xiaoxiao "$destination" || fail "安装位置已有同名文件，请先把它移开：$destination"
    printf '已安装：%s\n' "$destination"
else
    staged="$lock/小小.app"
    if [[ -d "$packaged_app" ]]; then
        is_xiaoxiao "$packaged_app" || fail '压缩包里的小小校验失败，请重新解压。'
        /usr/bin/ditto "$packaged_app" "$staged"
    elif [[ -f "$source_dir/build.sh" ]]; then
        if ! xcode-select -p >/dev/null 2>&1 || ! xcrun --find swiftc >/dev/null 2>&1; then
            fail '从源码安装前，请在终端运行 xcode-select --install。安装完成后再双击本文件。'
        fi
        /bin/bash "$source_dir/build.sh" "$staged"
    else
        fail '安装文件不完整，请先解压整个压缩包。'
    fi
    is_xiaoxiao "$staged" || fail '安装文件校验失败。'
    if [[ -e "$destination" || -L "$destination" ]]; then
        fail "安装位置出现同名文件，请先把它移开：$destination"
    fi
    mv -- "$staged" "$destination"
    printf '已安装：%s\n' "$destination"
fi

if [[ "${XIAOXIAO_SKIP_OPEN:-0}" != 1 ]]; then
    /usr/bin/open "$destination"
fi
