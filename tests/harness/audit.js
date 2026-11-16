window.rbAudit = async function () {
    const wait = ms => new Promise(r => setTimeout(r, ms));
    const report = [];
    const onError = e => report.push('script error: ' + (e.message || e.reason || e));
    window.addEventListener('error', onError);
    window.addEventListener('unhandledrejection', onError);
    const still = document.head.appendChild(document.createElement('style'));
    still.textContent = '#rb-app * { animation: none !important; transition: none !important; }';
    const scrolls = (el, panel) => {
        for (let p = el.parentElement; p && p !== panel; p = p.parentElement) {
            if (['auto', 'scroll'].includes(getComputedStyle(p).overflowX)) {
                return true;
            }
        }
        return false;
    };
    const check = label => {
        document.querySelectorAll('#rb-app *').forEach(el => {
            const cs = getComputedStyle(el);
            if (el.clientWidth === 0 || cs.display === 'none' || el.closest('svg')) {
                return;
            }
            const clipped = cs.overflowX !== 'visible' || cs.textOverflow === 'ellipsis';
            if (!clipped && el.scrollWidth > el.clientWidth + 2 && el.children.length === 0) {
                report.push(label + ': text wider than its box: ' + (el.className || el.tagName) + ' "' +
                            el.textContent.slice(0, 60) + '" ' + el.scrollWidth + '>' + el.clientWidth);
            }
        });
        if (document.documentElement.scrollWidth > document.documentElement.clientWidth + 1) {
            report.push(label + ': the page scrolls sideways (' + document.documentElement.scrollWidth + ')');
        }
        const top = Array.from(document.querySelectorAll('#rb-app .rb-overlay')).pop();
        (top ? top.querySelectorAll('.rb-panel-foot button') : []).forEach(b => {
            const r = b.getBoundingClientRect();
            if (!r.width) {
                return;
            }
            for (const [x, y] of [[r.left + r.width / 2, r.top + r.height / 2], [r.left + 3, r.bottom - 3], [r.right - 3, r.bottom - 3]]) {
                const hit = document.elementFromPoint(x, y);
                if (!hit || (!b.contains(hit) && !hit.contains(b))) {
                    report.push(label + ': button "' + b.textContent.trim() + '" is ' +
                                (hit ? 'covered by ' + (hit.id ? '#' + hit.id : hit.className || hit.tagName) : 'outside the window'));
                    break;
                }
            }
        });
        document.querySelectorAll('#rb-app .rb-drawer, #rb-app .rb-modal').forEach(panel => {
            panel.querySelectorAll('*').forEach(el => {
                const r = el.getBoundingClientRect();
                const p = panel.getBoundingClientRect();
                if (r.width && (r.right > p.right + 1 || r.left < p.left - 1) && !scrolls(el, panel)) {
                    report.push(label + ': sticks out of its dialog: ' + (el.className || el.tagName) + ' "' +
                                (el.textContent || '').slice(0, 40) + '"');
                }
            });
        });
    };
    const closeAll = async () => {
        for (let i = 0; i < 6 && document.querySelector('#rb-app .rb-overlay'); i++) {
            const discard = [...document.querySelectorAll('#rb-app .rb-overlay:last-child button')].find(b => b.textContent.trim() === 'Discard');
            if (discard) {
                discard.click();
            } else {
                (document.activeElement || document.body).dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}));
            }
            await wait(150);
        }
        if (document.querySelector('#rb-app .rb-overlay')) {
            report.push('a dialog did not close, later checks are unreliable');
        }
    };
    const tab = async name => {
        const t = [...document.querySelectorAll('#rb-app .rb-tab')].find(x => x.textContent.includes(name));
        if (!t) {
            report.push(name + ': no such tab, not checked');
            return;
        }
        t.click();
        await wait(1500);
        check(name);
    };
    const visible = sel => [...document.querySelectorAll(sel)].filter(x => !x.disabled && x.getClientRects().length);
    const open = async (text, label, ms = 1200) => {
        const b = visible('#rb-app button').find(x => x.textContent.trim().startsWith(text));
        if (!b) {
            report.push(label + ': no "' + text + '" button, not checked');
            return false;
        }
        b.click();
        await wait(ms);
        check(label);
        return true;
    };
    const fromMenu = async (card, item, label, ms = 1200) => {
        const more = document.querySelector('#rb-app ' + card + ' button[title="More"]');
        if (!more) {
            report.push(label + ': no card with a menu, not checked');
            return false;
        }
        more.click();
        await wait(300);
        return open(item, label, ms);
    };
    const options = async label => {
        for (const name of ['ZFS snapshot, brief', 'Stop while']) {
            const card = [...document.querySelectorAll('#rb-app .rb-option')].find(b => b.textContent.includes(name));
            if (!card) {
                report.push(label + ': no "' + name + '" option, not checked');
                continue;
            }
            card.click();
            await wait(1500);
            check(label + ' / ' + name);
        }
    };

    for (const name of ['Jobs', 'Repositories', 'Snapshots', 'Activity', 'Settings']) {
        await tab(name);
    }
    await tab('Snapshots');
    const snap = document.querySelector('#rb-app .rb-snap');
    if (snap) {
        snap.click();
        await wait(2500);
        check('snapshot browser');
        if (await open('Restore', 'restore dialog', 800)) {
            await closeAll();
        }
    } else {
        report.push('snapshot browser: no snapshot, not checked');
    }
    await tab('Jobs');
    for (const [item, label, ms] of [['Edit', 'job editor', 1500], ['What gets backed up', 'preview', 3500], ['Size history', 'size history', 1500], ['Last log', 'log', 1200]]) {
        if (await fromMenu('.rb-card', item, label, ms)) {
            if (label === 'job editor') {
                await options(label);
            }
            await closeAll();
        }
    }
    if (await open('New job', 'new job', 800)) {
        await options('new job');
        if (await open('Add folder', 'folder picker', 800)) {
            const firstDir = document.querySelector('#rb-app .rb-overlay:last-child tr.clickable td.name');
            if (firstDir) {
                firstDir.click();
                await wait(800);
            }
            [...document.querySelectorAll('#rb-app .rb-overlay:last-child td.pick input')].slice(0, 2).forEach(cb => cb.click());
            await wait(300);
            check('folder picker / selected');
        }
        await closeAll();
    }
    await tab('Settings');
    if (await open('Browse...', 'data folder picker', 800)) {
        await open('New folder', 'data folder picker / new folder', 300);
        await closeAll();
    }
    await tab('Repositories');
    for (const [item, label] of [['Edit', 'repo editor'], ['Keys', 'keys'], ['Show password', 'show password']]) {
        if (await fromMenu('.rb-card', item, label)) {
            await closeAll();
        }
    }
    if (await open('Add repository', 'new repo', 800)) {
        await closeAll();
    }
    await tab('Jobs');
    still.remove();
    window.removeEventListener('error', onError);
    window.removeEventListener('unhandledrejection', onError);
    return [...new Set(report)];
};
