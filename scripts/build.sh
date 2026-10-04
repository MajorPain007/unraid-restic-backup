#!/bin/bash

set -euo pipefail

PLUGIN="restic.backup"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-}"
OUT_DIR="${RB_OUT_DIR:-${ROOT_DIR}/archive}"
case "$OUT_DIR" in
    /*) ;;
    *) OUT_DIR="${ROOT_DIR}/${OUT_DIR}" ;;
esac
PLG="${ROOT_DIR}/src/${PLUGIN}.plg"

if [[ ! "$VERSION" =~ ^[0-9]{4}\.[0-9]{2}\.[0-9]{2}\.[0-9]+$ ]]; then
    echo "usage: $0 YYYY.MM.DD.NN" >&2
    exit 1
fi

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
PDIR="${STAGE}/usr/local/emhttp/plugins/${PLUGIN}"
mkdir -p "${PDIR}" "${STAGE}/install"

cp "${ROOT_DIR}/src/"*.page "${ROOT_DIR}/src/README.md" "${PDIR}/"
cp -R "${ROOT_DIR}/src/include" "${ROOT_DIR}/src/scripts" "${ROOT_DIR}/src/assets" "${ROOT_DIR}/src/event" "${PDIR}/"
find "${PDIR}" -name '.DS_Store' -delete
find "${PDIR}" -type d -exec chmod 755 {} +
find "${PDIR}" -type f -exec chmod 644 {} +
chmod 755 "${PDIR}/event/"*

for s in "${PDIR}/event/"*; do
    bash -n "$s" || { echo "syntax error in $s" >&2; exit 1; }
done
if command -v php >/dev/null 2>&1; then
    while IFS= read -r f; do
        php -l "$f" >/dev/null || { echo "PHP syntax error in $f" >&2; exit 1; }
    done < <(find "${PDIR}" -name '*.php' -o -name '*.page')
fi
if command -v node >/dev/null 2>&1; then
    for f in "${PDIR}/assets/js/"*.js; do
        node --check "$f" || { echo "JavaScript syntax error in $f" >&2; exit 1; }
    done
fi

cat > "${STAGE}/install/slack-desc" << DESC
${PLUGIN}: Restic Backup
${PLUGIN}:
${PLUGIN}: Encrypted, deduplicated backups with restic to local disks,
${PLUGIN}: SFTP servers and REST servers, with schedules, retention,
${PLUGIN}: restores and ZFS snapshot support.
${PLUGIN}:
DESC

mkdir -p "${OUT_DIR}"
PKG="${PLUGIN}-${VERSION}-x86_64-1.txz"
( cd "${STAGE}" && COPYFILE_DISABLE=1 tar --owner=0 --group=0 --no-xattrs -cJf "${OUT_DIR}/${PKG}" install/ usr/ 2>/dev/null \
  || COPYFILE_DISABLE=1 tar --no-xattrs -cJf "${OUT_DIR}/${PKG}" install/ usr/ )

if command -v md5sum >/dev/null 2>&1; then
    MD5=$(md5sum "${OUT_DIR}/${PKG}" | cut -d' ' -f1)
else
    MD5=$(md5 -q "${OUT_DIR}/${PKG}")
fi

if [ -f "$PLG" ] && [ "${RB_SKIP_PLG:-0}" != "1" ]; then
    sed -e "s|<!ENTITY version   \"[^\"]*\">|<!ENTITY version   \"${VERSION}\">|" \
        -e "s|<!ENTITY md5       \"[^\"]*\">|<!ENTITY md5       \"${MD5}\">|" \
        "$PLG" > "${PLG}.tmp"
    mv "${PLG}.tmp" "$PLG"
fi
echo "${OUT_DIR#"$ROOT_DIR"/}/${PKG}  md5 ${MD5}"
