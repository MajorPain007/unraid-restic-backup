window.rbAudit = async function () {
    const wait = ms => new Promise(r => setTimeout(r, ms));
    const report = [];
    const onError = e => report.push('script error: ' + (e.message || e.reason || e));
    window.addEventListener('error', onError);
    window.addEventListener('unhandledrejection', onError);
    const still = document.head.appendChild(document.createElement('style'));
    still.textContent = '#rb-app * { animation: none !important; transition: none !important; }';
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
                if (r.width && (r.right > p.right + 1 || r.left < p.left - 1) && !el.closest('.rb-browser-scroll')) {
                    report.push(label + ': sticks out of its dialog: ' + (el.className || el.tagName) + ' "' +
                                (el.textContent || '').slice(0, 40) + '"');
                }
            });
        });
    };
    const closeAll = async () => {
        for (let i = 0; i < 4; i++) {
            document.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape'}));
            await wait(80);
        }
    };
    for (const tab of ['jobs', 'repos', 'snapshots', 'activity', 'settings']) {
        RB.show(tab);
        await wait(1500);
        check(tab);
    }
    RB.show('snapshots');
    await wait(1500);
    const snap = document.querySelector('#rb-app .rb-snap');
    if (snap) {
        snap.click();
        await wait(2500);
        check('snapshot browser');
        const restore = [...document.querySelectorAll('#rb-app .rb-selbar button')].pop();
        if (restore) {
            restore.click();
            await wait(800);
            check('restore dialog');
            await closeAll();
        }
    }
    RB.show('jobs');
    await wait(800);
    const job = RB.state.overview.config.jobs[0];
    if (job) {
        RB.editJob(job.id);
        await wait(1500);
        check('job editor');
        for (const name of ['ZFS snapshot, brief', 'Stop while']) {
            const card = [...document.querySelectorAll('#rb-app .rb-option')].find(b => b.textContent.includes(name));
            if (card) {
                card.click();
                await wait(1500);
                check('job editor / ' + name);
            }
        }
        await closeAll();
    }
    RB.editJob(null);
    await wait(800);
    check('new job');
    for (const name of ['ZFS snapshot, brief', 'Stop while']) {
        const card = [...document.querySelectorAll('#rb-app .rb-option')].find(b => b.textContent.includes(name));
        if (card) {
            card.click();
            await wait(1500);
            check('new job / ' + name);
        }
    }
    await closeAll();
    for (const repo of RB.state.overview.config.repos) {
        RB.editRepo(repo.id);
        await wait(800);
        check('repo editor ' + repo.type);
        await closeAll();
    }
    RB.editRepo(null);
    await wait(800);
    check('new repo');
    await closeAll();
    const recent = RB.state.overview.recent[0];
    if (recent) {
        RB.showLog(recent.id);
        await wait(1200);
        check('log');
        await closeAll();
    }
    if (job) {
        RB.previewJob(job);
        await wait(3500);
        check('preview');
        await closeAll();
        RB.sizeHistory({job: job.id});
        await wait(1500);
        check('size history');
        await closeAll();
    }
    RB.pickPath({title: 'Folder picker', multiple: true, create: true, start: '/mnt'});
    await wait(800);
    const firstDir = document.querySelector('#rb-app .rb-overlay:last-child tr.clickable td.name');
    if (firstDir) {
        firstDir.click();
        await wait(800);
    }
    const newBtn = [...document.querySelectorAll('#rb-app .rb-overlay:last-child .rb-browser-head button')]
        .find(b => b.textContent.trim() === 'New folder' && !b.disabled);
    if (newBtn) {
        newBtn.click();
        await wait(300);
    }
    [...document.querySelectorAll('#rb-app .rb-overlay:last-child td.pick input')].slice(0, 2).forEach(cb => cb.click());
    await wait(300);
    check('folder picker');
    await closeAll();
    RB.show('jobs');
    still.remove();
    window.removeEventListener('error', onError);
    window.removeEventListener('unhandledrejection', onError);
    return [...new Set(report)];
};
