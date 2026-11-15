#!/bin/bash
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1
fail=0
section() { printf '\n\033[1m%s\033[0m\n' "$1"; }

section "Go"
GO=$(command -v go || echo /usr/local/go/bin/go)
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
    echo "  FAIL not gofmt-formatted: $unformatted"
    fail=1
else
    echo "  PASS gofmt"
fi
GOOS=linux "$GO" vet ./... || fail=1
"$GO" test ./internal/rb/ -count=1 || fail=1

section "PHP syntax"
n=0
while IFS= read -r f; do
    n=$((n + 1))
    out=$(php -l "$f" 2>&1) || { echo "  $out"; fail=1; }
done < <(find src tests -name '*.php' -o -name '*.page')
echo "  $n files"

section "Static checks"
( cd web && { [ -d node_modules ] || npm ci --silent; } && npm run --silent check ) || fail=1

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

section "No comments in PHP and shell"
found=""
if command -v php >/dev/null 2>&1; then
    while IFS= read -r f; do
        found="$found$(php -r 'foreach (token_get_all(file_get_contents($argv[1])) as $t) { if (!is_array($t)) continue; if ($t[0] === T_COMMENT || $t[0] === T_DOC_COMMENT || ($t[0] === T_INLINE_HTML && preg_match("~/\*|<!--~", $t[1]))) echo $argv[1], ":", $t[2], "\n"; }' "$f")"
    done < <(find src tests -name '*.php' -o -name '*.page' | sort)
else
    echo "  SKIP php not installed, PHP files not checked"
fi
while IFS= read -r f; do
    found="$found$(awk -v f="$f" 'NR > 1 && /^[[:space:]]*#/ && !/^[[:space:]]*#!/ { print f ":" NR }' "$f")"
done < <(find scripts tests src/event -type f \( -name '*.sh' -o -path 'src/event/*' \) | sort)
if [ -n "$found" ]; then
    echo "  FAIL comments found:"
    echo "$found" | sed 's/^/    /'
    fail=1
else
    echo "  PASS no comments in PHP and shell"
fi

echo
[ "$fail" = 0 ] && printf '\033[32mAll passed\033[0m\n' || printf '\033[31mSomething failed\033[0m\n'
exit "$fail"
