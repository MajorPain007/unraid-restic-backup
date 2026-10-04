#!/bin/bash
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1
fail=0
section() { printf '\n\033[1m%s\033[0m\n' "$1"; }

section "PHP syntax"
n=0
while IFS= read -r f; do
    n=$((n + 1))
    out=$(php -l "$f" 2>&1) || { echo "  $out"; fail=1; }
done < <(find src tests -name '*.php' -o -name '*.page')
echo "  $n files"

for t in tests/php/*_test.php; do
    section "$(basename "$t" .php)"
    php "$t" | grep -v '^$' | grep -v '^\s*\x1b\[1m' || true
    php "$t" >/dev/null 2>&1 || fail=1
done

section "Static checks"
php tests/php/static_check.php || fail=1
node tests/js_check.js || fail=1

section "Pages and manifest"
for p in src/*.page; do
    if ! grep -q '^Markdown="false"' "$p"; then
        echo "  FAIL $p lacks Markdown=\"false\": Unraid would run it through Markdown"
        fail=1
    fi
done
if php -r 'exit(simplexml_load_file($argv[1]) === false ? 1 : 0);' src/restic.backup.plg 2>/dev/null; then
    echo "  PASS the manifest is well-formed XML"
else
    echo "  FAIL src/restic.backup.plg is not well-formed XML (a < in CHANGES?)"
    fail=1
fi
for e in src/event/*; do
    bash -n "$e" || fail=1
done
echo "  PASS event hooks parse"

echo
[ "$fail" = 0 ] && printf '\033[32mAll passed\033[0m\n' || printf '\033[31mSomething failed\033[0m\n'
exit "$fail"
