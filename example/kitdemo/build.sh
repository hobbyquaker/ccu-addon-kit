#!/bin/sh
#
#   Builds dist/kitdemo-<version>.tar.gz (noarch) and its .sha256 (ccu-addon-kit, MIT):
#   vendor the kit, stamp the version, pack as root with LF endings and no ._ files.
#
#     VERSION=0.1.0 ./build.sh          KIT="npx ccu-addon-kit" by default
#     FRAME=none ./build.sh             skip the web build: vendor installs the placeholder page
#
set -e
cd "$(dirname "$0")"
ADDON=kitdemo
VERSION=${VERSION:-0.0.0-dev}
KIT=${KIT:-npx ccu-addon-kit}

# the settings page: the kit's frame with the addon's tabs, built by Vite into web/dist and
# copied into www/frame by vendor
if [ "${FRAME:-}" = none ]; then
    $KIT vendor --config kit.json --out addon_files/$ADDON
else
    npx vite build --logLevel warn --config web/vite.config.mjs
    $KIT vendor --config kit.json --out addon_files/$ADDON --frame web/dist
fi
printf 'VERSION_ADDON=%s\n' "$VERSION" > addon_files/$ADDON/versions

mkdir -p dist
out=dist/$ADDON-$VERSION.tar.gz
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
cp -a addon_files/. "$stage/"
find "$stage" \( -name '._*' -o -name '.DS_Store' \) -exec rm -f {} +
chmod 755 "$stage/update_script"
(cd "$stage" && sha256sum update_script > update_script.sha256)
COPYFILE_DISABLE=1 tar --owner=root --group=root --sort=name --mtime='2026-01-01 00:00Z' -czf "$out" -C "$stage" .
(cd dist && sha256sum "$(basename "$out")" > "$(basename "$out").sha256")
echo "built $out"
