'use strict';
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const dir = path.join(__dirname, '..', 'src', 'assets', 'js');
const page = fs.readFileSync(path.join(__dirname, '..', 'src', 'include', 'page.php'), 'utf8');
const order = (page.match(/array\(([^)]*)\) as \$rbScript/) || [])[1].split(',').map(s => s.trim().replace(/'/g, ''));
let failed = 0;
const fail = msg => { failed++; console.log('  \x1b[31mFAIL\x1b[0m ' + msg); };

const files = fs.readdirSync(dir).filter(f => f.endsWith('.js')).sort();
for (const f of files) {
    if (!order.includes(f.replace(/\.js$/, ''))) {
        fail(`${f} is not loaded by include/page.php`);
    }
}
for (const name of order) {
    if (!files.includes(name + '.js')) {
        fail(`include/page.php loads ${name}.js, which does not exist`);
    }
}
if (order[order.length - 1] !== 'main' || order[0] !== 'core') {
    fail('core.js has to load first and main.js last');
}

const defined = new Set(['root', 'views', 'state']);
const used = [];
for (const f of files) {
    const code = fs.readFileSync(path.join(dir, f), 'utf8');
    try {
        new vm.Script(code, {filename: f});
    } catch (e) {
        fail(`${f}: ${e.message}`);
        continue;
    }
    for (const m of code.matchAll(/\bRB\.([A-Za-z_]\w*)\s*=(?!=)/g)) {
        defined.add(m[1]);
    }
    const assign = code.match(/Object\.assign\(RB,\s*\{([\s\S]*?)\}\);/);
    if (assign) {
        for (const m of assign[1].matchAll(/([A-Za-z_]\w*)\s*:/g)) {
            defined.add(m[1]);
        }
    }
    const lines = code.split('\n');
    lines.forEach((line, i) => {
        for (const m of line.matchAll(/\bRB\.([A-Za-z_]\w*)/g)) {
            used.push([m[1], f, i + 1]);
        }
    });
}
let count = 0;
for (const [name, f, line] of used) {
    count++;
    if (!defined.has(name)) {
        fail(`RB.${name} used in ${f}:${line} is defined nowhere`);
    }
}
if (!failed) {
    console.log(`  \x1b[32mPASS\x1b[0m ${files.length} scripts parse; all ${count} uses of RB.* resolve`);
}
process.exit(failed ? 1 : 0);
