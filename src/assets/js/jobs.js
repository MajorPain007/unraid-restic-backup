(function (RB) {
    'use strict';
    var h = RB.h;

    var progressBlocks = {};
    var rates = {};

    RB.on('progress', function (ops) {
        ops.forEach(function (op) {
            if (progressBlocks[op.id]) {
                progressBlocks[op.id](op);
            }
        });
    });

    RB.progressBlock = function (op) {
        var bar = h('div', {class: 'rb-progress-bar'});
        var track = h('div', {class: 'rb-progress-track'}, bar);
        var left = h('span');
        var right = h('span');
        var current = h('div', {class: 'rb-current'});
        var el = h('div', {class: 'rb-progress'}, track, h('div', {class: 'rb-progress-text'}, left, right), current);

        function update(op) {
            if (!el.isConnected && progressBlocks[op.id] === update && Object.keys(progressBlocks).length > 50) {
                delete progressBlocks[op.id];
            }
            var p = op.progress;
            RB.clear(left);
            RB.clear(right);
            if (!p || !p.total_bytes && !p.percent) {
                track.classList.add('indeterminate');
                left.appendChild(h('b', {text: op.phase || 'Working'}));
                right.textContent = RB.duration(RB.now() - op.started);
                current.textContent = '';
                return;
            }
            track.classList.remove('indeterminate');
            var pct = Math.max(0, Math.min(1, p.percent || 0));
            bar.style.width = (pct * 100).toFixed(1) + '%';
            left.appendChild(h('b', {text: Math.floor(pct * 100) + '%'}));
            var parts = [];
            if (p.total_bytes) {
                parts.push(RB.bytes(p.bytes_done) + ' of ' + RB.bytes(p.total_bytes));
            }
            if (p.total_files) {
                parts.push(p.files_done.toLocaleString('en-GB') + ' of ' + p.total_files.toLocaleString('en-GB') + ' files');
            }
            left.appendChild(document.createTextNode('  ' + op.phase + (parts.length ? ' · ' + parts.join(' · ') : '')));
            var r = rates[op.id];
            var t = RB.now();
            if (r && p.bytes_done >= r.bytes && t > r.at) {
                var inst = (p.bytes_done - r.bytes) / (t - r.at);
                r.rate = r.rate === null ? inst : r.rate * 0.7 + inst * 0.3;
                r.bytes = p.bytes_done;
                r.at = t;
            } else {
                rates[op.id] = r = {bytes: p.bytes_done || 0, at: t, rate: null};
            }
            var bits = [];
            if (r.rate) {
                bits.push(RB.bytes(r.rate) + '/s');
            }
            var eta = p.eta !== null && p.eta !== undefined ? p.eta
                : (r.rate && p.total_bytes ? (p.total_bytes - p.bytes_done) / r.rate : null);
            if (eta !== null && pct > 0.01) {
                bits.push(RB.duration(eta) + ' left');
            }
            if (p.errors) {
                bits.push(RB.plural(p.errors, 'error'));
            }
            right.textContent = bits.join(' · ');
            current.textContent = p.current || '';
            current.title = p.current || '';
        }

        progressBlocks[op.id] = update;
        update(op);
        return el;
    };

    var STATUS = {
        success: ['ok', 'Succeeded'],
        warning: ['warn', 'Warnings'],
        error: ['err', 'Failed'],
        cancelled: ['idle', 'Stopped']
    };

    function jobStatus(info, cfg) {
        if (info.running) {
            return ['run', 'Running'];
        }
        if (!cfg.enabled) {
            return ['idle', 'Disabled'];
        }
        if (info.queued) {
            return ['run', 'Waiting'];
        }
        if (!info.last) {
            return ['idle', 'Not run yet'];
        }
        return STATUS[info.last.status] || ['idle', info.last.status];
    }

    RB.statusPill = function (status) {
        var s = STATUS[status] || (status === 'running' ? ['run', 'Running'] : ['idle', status]);
        return h('span', {class: 'rb-pill ' + s[0]}, s[1]);
    };

    RB.repoIcon = function (type) {
        return {sftp: 'server', local: 'hdd', rest: 'cloud'}[type] || 'repo';
    };

    RB.repoChip = function (repo) {
        if (!repo) {
            return h('span', {class: 'rb-chip'}, RB.icon('alert'), 'No repository');
        }
        return h('span', {class: 'rb-chip', title: repo.name}, RB.icon(RB.repoIcon(repo.type)), repo.name);
    };

    RB.views.jobs = {
        render: function (container) {
            var o = RB.state.overview;
            if (!o.config.repos.length) {
                container.appendChild(h('div', {class: 'rb-empty'},
                    RB.icon('archive', 'big'),
                    h('h3', {text: 'Back up your server with restic'}),
                    h('p', {text: 'Backups are encrypted and deduplicated. They can go to another disk, ' +
                               'to a second server over SSH, or to a restic REST server.'}),
                    h('div', {class: 'rb-steps'},
                      h('span', null, h('b', {text: '1'}), 'Add a repository - where backups go'),
                      h('span', null, h('b', {text: '2'}), 'Create a job - what to back up, and when'),
                      h('span', null, h('b', {text: '3'}), 'Done - it runs on its own')),
                    RB.button('Add repository', {icon: 'plus', kind: 'primary', onclick: function () { RB.editRepo(null); }})));
                return;
            }
            if (!o.config.jobs.length) {
                container.appendChild(h('div', {class: 'rb-empty'},
                    RB.icon('archive', 'big'),
                    h('h3', {text: 'Create your first backup job'}),
                    h('p', {text: 'A job says which folders go into which repository, when it runs, and how many ' +
                               'old snapshots to keep.'}),
                    RB.button('New job', {icon: 'plus', kind: 'primary', onclick: function () { RB.editJob(null); }})));
                return;
            }
            var grid = h('div', {class: 'rb-grid'});
            o.config.jobs.forEach(function (cfg) {
                grid.appendChild(jobCard(cfg, RB.jobInfo(cfg.id) || {}));
            });
            grid.appendChild(h('button', {type: 'button', class: 'rb-card add', onclick: function () { RB.editJob(null); }},
                               RB.icon('plus'), 'New job'));
            container.appendChild(grid);
        }
    };

    function jobCard(cfg, info) {
        var repo = RB.repoConfig(cfg.repo);
        var st = jobStatus(info, cfg);
        var card = h('div', {class: 'rb-card' + (info.running ? ' is-running' : '') +
                                    (info.last && info.last.status === 'error' && !info.running ? ' is-failed' : '') +
                                    (cfg.enabled ? '' : ' is-disabled')});
        var actions = h('div', {class: 'rb-card-actions'});
        if (info.running) {
            actions.appendChild(RB.button('Stop', {icon: 'stop', small: true, onclick: function (e) {
                stopOp(info.running, e.currentTarget);
            }}));
        } else if (info.queued) {
            actions.appendChild(RB.button('Queued', {icon: 'clock', small: true, disabled: true,
                title: 'Waits for the repository to be free'}));
        } else {
            actions.appendChild(RB.button('Run now', {icon: 'play', small: true, kind: 'primary',
                disabled: !o().system.array_started,
                onclick: function (e) { runJob(cfg, e.currentTarget); }}));
        }
        actions.appendChild(RB.button('', {icon: 'more', small: true, kind: 'ghost', title: 'More',
            onclick: function (e) { jobMenu(cfg, info, e.currentTarget); }}));

        card.appendChild(h('div', {class: 'rb-card-head'},
            h('span', {class: 'rb-dot ' + st[0], title: st[1]}),
            h('div', {class: 'rb-card-title'},
              h('h3', {text: cfg.name, title: cfg.name}),
              h('div', {class: 'rb-meta'}, RB.repoChip(repo), h('span', {class: 'rb-sep', text: '·'}),
                h('span', {text: cfg.enabled ? info.schedule : 'Disabled'}))),
            actions));

        if (info.running) {
            card.appendChild(RB.progressBlock(info.running));
        } else {
            var last = info.last;
            var facts = h('div', {class: 'rb-facts'});
            facts.appendChild(fact('Last run', last ? h('span', {title: RB.dateTime(last.ended)},
                RB.relative(last.ended), ' ', h('small', {text: '· ' + st[1].toLowerCase()})) : 'Never'));
            facts.appendChild(fact('Next run', cfg.enabled && info.next
                ? h('span', {title: RB.dateTime(info.next)}, info.next <= RB.now() + 30 ? 'Due now' : RB.relative(info.next))
                : (cfg.enabled ? 'Only when started' : 'Disabled')));
            var s = last && last.summary;
            facts.appendChild(fact('Added', s && s.data_added_packed !== undefined
                ? h('span', null, RB.bytes(s.data_added_packed), ' ', h('small', {text: '· ' +
                    (s.files_new || 0).toLocaleString('en-GB') + ' new, ' + (s.files_changed || 0).toLocaleString('en-GB') + ' changed'}))
                : '-'));
            facts.appendChild(fact('Duration', last ? RB.duration(last.ended - last.started) : '-'));
            card.appendChild(facts);
            if (last && (last.status === 'error' || last.status === 'warning') && last.error) {
                card.appendChild(h('div', {class: 'rb-card-note ' + (last.status === 'error' ? 'err' : 'warn')},
                    RB.icon('alert'),
                    h('div', null, last.error, ' ', h('span', {class: 'rb-link', text: 'Show log',
                        onclick: function () { RB.showLog(last.id); }}))));
            }
        }
        card.appendChild(strip(info.recent || []));
        return card;
    }

    function o() {
        return RB.state.overview;
    }

    function fact(label, value) {
        return h('div', {class: 'rb-fact'}, h('div', {class: 'rb-label', text: label}),
                 h('div', {class: 'rb-value'}, value));
    }

    function strip(recent) {
        var runs = recent.slice(0, 30).reverse();
        var el = h('div', {class: 'rb-strip', 'aria-label': 'Recent runs'});
        for (var i = runs.length; i < 30; i++) {
            el.appendChild(h('span', {class: 'st-empty'}));
        }
        runs.forEach(function (r) {
            var st = STATUS[r.status] || ['idle', r.status];
            el.appendChild(h('span', {class: 'st-' + r.status, title: RB.dateTime(r.started) + ' - ' + st[1] +
                (r.added !== null && r.added !== undefined ? ' - ' + RB.bytes(r.added) + ' added' : ''),
                style: {cursor: 'pointer'}, onclick: function () { RB.showLog(r.id); }}));
        });
        return el;
    }

    function runJob(cfg, btn) {
        RB.busy(btn, RB.api('job_run', {id: cfg.id})).then(function (r) {
            RB.toast(r.queued ? '"' + cfg.name + '" waits for its repository to be free.' : 'Backup "' + cfg.name + '" started.');
            setTimeout(RB.refresh, 700);
        }).catch(RB.fail);
    }

    function stopOp(op, btn) {
        RB.confirm({title: 'Stop this operation?',
                    text: op.kind === 'backup' ? 'The backup ends without a snapshot. Containers or VMs it stopped are started again.'
                        : 'It ends as soon as restic has cleaned up.',
                    confirm: 'Stop', danger: true}).then(function (yes) {
            if (yes) {
                RB.busy(btn, RB.api('op_stop', {id: op.id})).then(function () {
                    RB.toast('Stopping...', 'info');
                    setTimeout(RB.refresh, 1500);
                }).catch(RB.fail);
            }
        });
    }
    RB.stopOp = stopOp;

    function jobMenu(cfg, info, anchor) {
        RB.menu(anchor, [
            {label: 'Edit', icon: 'edit', onclick: function () { RB.editJob(cfg.id); }},
            {label: 'Browse snapshots', icon: 'layers', onclick: function () {
                RB.show('snapshots', {repo: cfg.repo, job: cfg.id});
            }},
            {label: 'What gets backed up', icon: 'eye', onclick: function () { RB.whatGetsBackedUp(cfg); }},
            {label: 'Size history', icon: 'activity', onclick: function () { RB.sizeHistory({job: cfg.id}); }},
            info.last ? {label: 'Last log', icon: 'list', onclick: function () { RB.showLog(info.last.id); }} : null,
            {label: 'Apply retention now', icon: 'trash', disabled: !cfg.retention.enabled, onclick: function () {
                RB.api('job_forget', {id: cfg.id}).then(function () {
                    RB.toast('Removing old snapshots of "' + cfg.name + '"...');
                    setTimeout(RB.refresh, 700);
                }).catch(RB.fail);
            }},
            '-',
            {label: cfg.enabled ? 'Disable' : 'Enable', icon: 'power', onclick: function () {
                var copy = JSON.parse(JSON.stringify(cfg));
                copy.enabled = !cfg.enabled;
                RB.api('job_save', {job: copy}).then(RB.refresh).catch(RB.fail);
            }},
            {label: 'Delete', icon: 'trash', danger: true, onclick: function () { deleteJob(cfg); }}
        ]);
    }

    function deleteJob(cfg) {
        return RB.confirm({title: 'Delete "' + cfg.name + '"?',
                           text: 'The job and its schedule are removed. Its snapshots stay in the repository - ' +
                                 'they can still be browsed and restored, and removed there by hand.',
                           confirm: 'Delete job', danger: true}).then(function (yes) {
            if (!yes) {
                return false;
            }
            return RB.api('job_delete', {id: cfg.id}).then(function () {
                RB.toast('Job deleted.');
                RB.refresh();
                return true;
            }).catch(function (e) {
                RB.fail(e);
                return false;
            });
        });
    }

    var DAYS = [['1', 'Monday'], ['2', 'Tuesday'], ['3', 'Wednesday'], ['4', 'Thursday'], ['5', 'Friday'],
                ['6', 'Saturday'], ['0', 'Sunday']];

    function isNum(s) {
        return /^\d+$/.test(s);
    }

    function presetOf(s) {
        if (s.mode === 'off') {
            return {preset: 'manual'};
        }
        if (s.mode === 'interval') {
            return {preset: 'interval'};
        }
        var f = String(s.cron || '').trim().split(/\s+/);
        if (f.length === 5 && isNum(f[0]) && f[1] === '*' && f[2] === '*' && f[3] === '*' && f[4] === '*') {
            return {preset: 'hourly', minute: +f[0]};
        }
        if (f.length === 5 && isNum(f[0]) && isNum(f[1]) && f[3] === '*') {
            var time = pad(f[1]) + ':' + pad(f[0]);
            if (f[2] === '*' && f[4] === '*') {
                return {preset: 'daily', time: time};
            }
            if (f[2] === '*' && /^[0-7]$/.test(f[4])) {
                return {preset: 'weekly', time: time, day: f[4] === '7' ? '0' : f[4]};
            }
            if (isNum(f[2]) && +f[2] <= 28 && f[4] === '*') {
                return {preset: 'monthly', time: time, dom: +f[2]};
            }
        }
        return {preset: 'custom'};
    }

    function pad(n) {
        return ('0' + n).slice(-2);
    }

    RB.scheduleEditor = function (sched, opts) {
        opts = opts || {};
        var p = presetOf(sched);
        var state = {
            preset: p.preset,
            time: p.time || '03:00',
            minute: p.minute !== undefined ? p.minute : 0,
            day: p.day || '0',
            dom: p.dom || 1,
            every: sched.every || 6,
            unit: sched.unit || 'hours',
            cron: sched.cron || '0 3 * * *'
        };
        var detail = h('div', {class: 'rb-inline', style: {marginTop: '12px'}});
        var preview = h('div', {class: 'rb-next-runs'});
        var catchUp = RB.toggle(sched, 'catch_up', 'Run missed backups when the server is back on');
        var catchRow = h('div', {style: {marginTop: '12px'}}, catchUp,
            h('div', {class: 'rb-hint', style: {marginLeft: '44px'}},
              'If the server was off at the scheduled time, the backup runs once when it is back.'));
        var timer = null;

        function apply() {
            var t = (state.time || '03:00').split(':');
            var hh = +t[0] || 0;
            var mm = +t[1] || 0;
            switch (state.preset) {
                case 'manual':
                    sched.mode = 'off';
                    break;
                case 'hourly':
                    sched.mode = 'cron';
                    sched.cron = state.minute + ' * * * *';
                    break;
                case 'daily':
                    sched.mode = 'cron';
                    sched.cron = mm + ' ' + hh + ' * * *';
                    break;
                case 'weekly':
                    sched.mode = 'cron';
                    sched.cron = mm + ' ' + hh + ' * * ' + state.day;
                    break;
                case 'monthly':
                    sched.mode = 'cron';
                    sched.cron = mm + ' ' + hh + ' ' + state.dom + ' * *';
                    break;
                case 'interval':
                    sched.mode = 'interval';
                    sched.every = Math.max(1, +state.every || 1);
                    sched.unit = state.unit;
                    break;
                default:
                    sched.mode = 'cron';
                    sched.cron = state.cron;
            }
            catchRow.style.display = sched.mode === 'cron' ? '' : 'none';
            clearTimeout(timer);
            timer = setTimeout(loadPreview, 250);
            if (opts.onchange) {
                opts.onchange(sched);
            }
        }

        function loadPreview() {
            RB.clear(preview);
            if (sched.mode === 'off') {
                preview.appendChild(h('span', {class: 'rb-hint', text: opts.manualText || 'Runs only when started by hand.'}));
                return;
            }
            RB.api('schedule_preview', {schedule: sched}).then(function (r) {
                RB.clear(preview);
                if (!r.valid) {
                    preview.appendChild(h('span', {class: 'rb-error', text: r.error}));
                    return;
                }
                preview.appendChild(h('span', {class: 'rb-hint', text: 'Next: '}));
                r.next.forEach(function (ts) {
                    preview.appendChild(h('span', {class: 'rb-chip', title: RB.dateTime(ts)}, RB.icon('clock'),
                        new Date(ts * 1000).toLocaleString('en-GB', {weekday: 'short', day: 'numeric', month: 'short',
                                                                      hour: '2-digit', minute: '2-digit'})));
                });
            }).catch(function () {});
        }

        function renderDetail() {
            RB.clear(detail);
            var timeInput = function () {
                var el = h('input', {type: 'time', value: state.time});
                el.addEventListener('change', function () { state.time = el.value; apply(); });
                return el;
            };
            switch (state.preset) {
                case 'hourly':
                    var m = h('input', {type: 'number', min: 0, max: 59, value: state.minute});
                    m.addEventListener('input', function () { state.minute = Math.max(0, Math.min(59, +m.value || 0)); apply(); });
                    detail.append('At minute', m, 'of every hour');
                    break;
                case 'daily':
                    detail.append('Every day at', timeInput());
                    break;
                case 'weekly':
                    var d = h('select', null, DAYS.map(function (x) {
                        return h('option', {value: x[0], selected: x[0] === state.day}, x[1]);
                    }));
                    d.addEventListener('change', function () { state.day = d.value; apply(); });
                    detail.append('Every', d, 'at', timeInput());
                    break;
                case 'monthly':
                    var dom = h('input', {type: 'number', min: 1, max: 28, value: state.dom});
                    dom.addEventListener('input', function () { state.dom = Math.max(1, Math.min(28, +dom.value || 1)); apply(); });
                    detail.append('On day', dom, 'of every month at', timeInput());
                    break;
                case 'interval':
                    var ev = h('input', {type: 'number', min: 1, value: state.every});
                    ev.addEventListener('input', function () { state.every = +ev.value || 1; apply(); });
                    var u = h('select', null, [['hours', 'hours'], ['days', 'days']].map(function (x) {
                        return h('option', {value: x[0], selected: x[0] === state.unit}, x[1]);
                    }));
                    u.addEventListener('change', function () { state.unit = u.value; apply(); });
                    detail.append('Every', ev, u, h('span', {class: 'rb-hint', text: 'counted from the start of the last run'}));
                    break;
                case 'custom':
                    var c = h('input', {type: 'text', class: 'rb-mono', value: state.cron, placeholder: '0 3 * * *',
                                        style: {width: '220px'}, spellcheck: 'false'});
                    c.addEventListener('input', function () { state.cron = c.value; apply(); });
                    detail.append(c, h('span', {class: 'rb-hint', text: 'minute hour day-of-month month weekday'}));
                    break;
                default:
                    break;
            }
        }

        var presets = [['hourly', 'Hourly'], ['daily', 'Daily'], ['weekly', 'Weekly'], ['monthly', 'Monthly'],
                       ['interval', 'Interval'], ['custom', 'Custom']];
        if (opts.allowManual !== false) {
            presets.unshift(['manual', opts.manualLabel || 'Manual']);
        }
        var seg = RB.segmented(state.preset, presets, function (v) {
            state.preset = v;
            renderDetail();
            apply();
        });
        renderDetail();
        catchRow.style.display = sched.mode === 'cron' ? '' : 'none';
        setTimeout(loadPreview, 0);
        return h('div', null, seg, detail, preview, opts.noCatchUp ? null : catchRow);
    };

    var EXCLUDE_PRESETS = [
        ['Recycle bins', ['**/.Recycle.Bin', '**/.Trash-*', '**/$RECYCLE.BIN']],
        ['Temporary files', ['*.tmp', '*.temp', '*.part', '*.partial', '**/incomplete']],
        ['macOS clutter', ['.DS_Store', '._*', '**/.Spotlight-V100', '**/.fseventsd']],
        ['Plex caches', ['**/Plex Media Server/Cache', '**/Plex Media Server/Logs', '**/Plex Media Server/Crash Reports',
                         '**/Plex Media Server/Media/localhost/**/*.bif']],
        ['Jellyfin caches', ['**/jellyfin/cache', '**/jellyfin/log', '**/transcodes']],
        ['Logs', ['*.log', '**/logs/*.log.*']]
    ];

    var SOURCE_PRESETS = [
        ['appdata', '/mnt/user/appdata'],
        ['Flash drive', '/boot'],
        ['domains (VMs)', '/mnt/user/domains'],
        ['system', '/mnt/user/system']
    ];

    function blankJob() {
        var repos = o().config.repos;
        return {
            id: '', name: '', enabled: true, repo: repos.length === 1 ? repos[0].id : '',
            sources: [], excludes: [], iexcludes: [], exclude_caches: true, exclude_larger_than: '',
            one_file_system: false, tags: [],
            schedule: {mode: 'cron', cron: '0 3 * * *', every: 24, unit: 'hours', catch_up: true},
            retention: {enabled: true, last: 0, hourly: 0, daily: 7, weekly: 4, monthly: 12, yearly: 0, within: ''},
            consistency: {mode: 'none', containers: [], vms: [], skip: [], timeout: 120},
            hooks: {before: '', after: ''},
            notify: {success: false, warning: true, error: true, healthcheck: ''},
            advanced: {host: '', read_concurrency: 0, skip_if_unchanged: false, extra_flags: []}
        };
    }

    RB.editJob = function (id) {
        var saved = id ? RB.jobConfig(id) : null;
        var job = saved ? JSON.parse(JSON.stringify(saved)) : blankJob();
        var isNew = !saved;
        var errorSlot = h('div');
        var o_;

        function section(title, iconName, hint, content) {
            return h('div', {class: 'rb-form-section'},
                     h('h3', null, RB.icon(iconName), title),
                     hint ? h('div', {class: 'rb-hint', style: {marginBottom: '14px'}}, hint) : null,
                     content);
        }

        var repoSelect = RB.select(job, 'repo', [['', 'Choose a repository...']].concat(o().config.repos.map(function (r) {
            return [r.id, r.name + ' (' + {sftp: 'SFTP', local: 'local', rest: 'REST'}[r.type] + ')'];
        })));
        var basics = h('div', {class: 'rb-fields'},
            RB.field('Name', RB.input(job, 'name', {placeholder: 'e.g. Appdata to offsite'})),
            RB.field('Repository', repoSelect, {hint: 'Where the snapshots go.'}));

        var sourceList = h('div', {class: 'rb-pathlist'});
        function renderSources() {
            if (tree) {
                tree.refresh();
            }
            RB.clear(sourceList);
            if (!job.sources.length) {
                sourceList.appendChild(h('div', {class: 'rb-hint', text: 'No folders yet.'}));
            }
            job.sources.forEach(function (src, i) {
                sourceList.appendChild(h('div', {class: 'rb-pathlist-item'}, RB.icon('folder'),
                    h('span', {class: 'rb-mono', text: src, title: src}),
                    RB.button('', {icon: 'x', small: true, kind: 'ghost', title: 'Remove', onclick: function () {
                        job.sources.splice(i, 1);
                        renderSources();
                    }})));
            });
        }
        function addSource(path) {
            path = (path || '').trim().replace(/\/+$/, '') || '';
            if (path && job.sources.indexOf(path) === -1) {
                job.sources.push(path);
                renderSources();
            }
        }
        renderSources();
        var sources = h('div', null, sourceList,
            h('div', {class: 'rb-inline', style: {marginTop: '10px'}},
              RB.button('Add folders...', {icon: 'plus', small: true, onclick: function () {
                  RB.pickPath({title: 'Add folders to back up', confirm: 'Add', multiple: true, start: job.sources[0] || '/mnt/user'}).then(function (paths) {
                      (paths || []).forEach(addSource);
                  });
              }}),
              h('span', {class: 'rb-hint', text: 'Quick add:'}),
              SOURCE_PRESETS.map(function (p) {
                  return RB.button(p[0], {small: true, kind: 'ghost', onclick: function () { addSource(p[1]); }});
              })));

        var excl = h('textarea', {rows: 5, placeholder: '*.tmp\n**/cache\n/mnt/user/appdata/plex/Library/Application Support/Plex Media Server/Cache',
                                  spellcheck: 'false'});
        excl.value = job.excludes.join('\n');
        var tree = RB.ruleTree(job, {edit: true, onchange: function () { excl.value = job.excludes.join('\n'); }});
        var typing = null;
        function rulesChanged() {
            clearTimeout(typing);
            typing = setTimeout(tree.refresh, 350);
        }
        excl.addEventListener('input', function () {
            job.excludes = lines(excl.value);
            rulesChanged();
        });
        var excludes = h('div', null,
            RB.field('Leave out', excl, {hint: h('span', null, 'One pattern per line. ', h('code', {text: '*'}),
                ' matches within a name, ', h('code', {text: '**'}), ' across folders; a pattern starting with ',
                h('code', {text: '/'}), ' is an absolute path.')}),
            h('div', {class: 'rb-presets', style: {marginTop: '8px'}}, EXCLUDE_PRESETS.map(function (p) {
                return RB.button(p[0], {icon: 'plus', small: true, kind: 'ghost', onclick: function () {
                    p[1].forEach(function (pat) {
                        if (job.excludes.indexOf(pat) === -1) {
                            job.excludes.push(pat);
                        }
                    });
                    excl.value = job.excludes.join('\n');
                    tree.refresh();
                }});
            })),
            h('div', {class: 'rb-fields', style: {marginTop: '14px'}},
              h('div', {class: 'rb-field'}, RB.toggle(job, 'exclude_caches', 'Skip cache folders', tree.refresh),
                h('div', {class: 'rb-hint', text: 'Folders marked with a CACHEDIR.TAG file, as many apps do for theirs.'})),
              RB.field('Skip files larger than', RB.input(job, 'exclude_larger_than', {placeholder: 'e.g. 10G', oninput: rulesChanged}),
                       {optional: true})),
            h('div', {class: 'rb-field', style: {marginTop: '16px'}},
              h('label', null, 'What gets backed up'),
              tree.el,
              h('div', {class: 'rb-hint'}, 'Open the folders: ', h('span', {class: 'rb-rtree-mark in', style: {verticalAlign: 'middle'}}, RB.icon('check')),
                ' is backed up, ', h('span', {class: 'rb-rtree-mark out', style: {verticalAlign: 'middle'}}, RB.icon('x')),
                ' is left out. Click the mark to change it; the rules above follow.')),
            h('div', {class: 'rb-inline', style: {marginTop: '12px'}},
              RB.button('Measure sizes...', {icon: 'activity', small: true, onclick: function () { RB.previewJob(job); }}),
              h('span', {class: 'rb-hint', text: 'Adds up what these rules back up and leave out, folder by folder. Goes through every file, so large shares take minutes.'})));

        var schedule = RB.scheduleEditor(job.schedule);

        var r = job.retention;
        var retentionFields = h('div', {class: 'rb-fields'},
            [['last', 'Last'], ['hourly', 'Hourly'], ['daily', 'Daily'], ['weekly', 'Weekly'], ['monthly', 'Monthly'],
             ['yearly', 'Yearly']].map(function (k) {
                return RB.field(k[1], RB.input(r, k[0], {type: 'number', min: 0}));
            }),
            RB.field('Everything from the last', RB.input(r, 'within', {placeholder: 'e.g. 14d, 6m, 1y'}), {optional: true}));
        var previewBtn = RB.button('Preview', {icon: 'eye', small: true, disabled: isNew, title: isNew ? 'Save the job and run it first' : null,
            onclick: function (e) { previewRetention(job, e.currentTarget); }});
        var retention = h('div', null,
            RB.toggle(r, 'enabled', 'Remove old snapshots after each backup', function (on) {
                retentionFields.style.display = on ? '' : 'none';
            }),
            h('div', {style: {marginTop: '14px'}}, retentionFields),
            h('div', {class: 'rb-inline', style: {marginTop: '12px'}}, previewBtn,
              h('span', {class: 'rb-hint', text: 'Shows which snapshots this policy keeps and removes, without removing any.'})));
        retentionFields.style.display = r.enabled ? '' : 'none';

        var c = job.consistency;
        var consistencyDetail = h('div', {style: {marginTop: '12px'}});
        function renderConsistency() {
            RB.clear(consistencyDetail);
            if (c.mode !== 'none') {
                consistencyDetail.appendChild(RB.consistencyPicker(job));
            }
        }
        var consistency = h('div', null, RB.optionCards(c.mode, [
            {value: 'none', title: 'Back up as it is', icon: 'file',
             text: 'Fine for media and documents. Files that change during the backup, such as databases, may be caught halfway.'},
            {value: 'zfs', title: 'ZFS snapshot', icon: 'layers',
             text: 'Backs up a snapshot taken at the start. Nothing stops; databases see it like a power cut, which they recover from.'},
            {value: 'zfs-stop', title: 'ZFS snapshot, brief stop', icon: 'shield',
             text: 'Stops the containers and VMs using the data for the seconds the snapshot takes. Consistent, nearly no downtime.'},
            {value: 'stop', title: 'Stop while backing up', icon: 'stop',
             text: 'For data not on ZFS: containers and VMs using it are stopped for the whole backup and started again after.'}
        ], function (v) { c.mode = v; renderConsistency(); }, 'two'), consistencyDetail);
        renderConsistency();

        var n = job.notify;
        var notify = h('div', null,
            h('div', {style: {display: 'grid', gap: '10px'}},
              RB.toggle(n, 'error', 'When a backup fails'),
              RB.toggle(n, 'warning', 'When some files could not be read'),
              RB.toggle(n, 'success', 'When a backup succeeds')),
            h('div', {class: 'rb-hint', style: {marginTop: '8px'}},
              'Sent as Unraid notifications - to the browser, e-mail or an agent like Discord or Telegram, as set in Settings > Notifications.'),
            h('div', {style: {marginTop: '14px'}},
              RB.field('Healthcheck URL', RB.input(n, 'healthcheck', {placeholder: 'https://hc-ping.com/your-uuid', type: 'url'}),
                       {optional: true, hint: 'Pinged at the start and end of each backup (Healthchecks.io, Uptime Kuma, ...). It alerts you also when a backup does not run at all.'})));

        var before = h('textarea', {rows: 3, placeholder: '# e.g. dump a database first\ndocker exec mariadb mariadb-dump --all-databases > /mnt/user/appdata/mariadb/dump.sql',
                                    spellcheck: 'false'});
        before.value = job.hooks.before;
        before.addEventListener('input', function () { job.hooks.before = before.value; });
        var after = h('textarea', {rows: 3, placeholder: '# runs after the backup; $RB_STATUS is success, warning, error or cancelled',
                                   spellcheck: 'false'});
        after.value = job.hooks.after;
        after.addEventListener('input', function () { job.hooks.after = after.value; });
        var hooks = h('div', {class: 'rb-fields'},
            RB.field('Before the backup', before, {wide: true, optional: true,
                hint: 'Runs as root with bash. If it fails, nothing is backed up.'}),
            RB.field('After the backup', after, {wide: true, optional: true,
                hint: 'Also after a failed backup. Variables: RB_JOB_NAME, RB_STATUS, RB_SNAPSHOT, RB_LOG.'}));

        var a = job.advanced;
        var tags = h('input', {type: 'text', value: job.tags.join(' '), placeholder: 'e.g. weekly important'});
        tags.addEventListener('input', function () { job.tags = tags.value.split(/[\s,]+/).filter(Boolean); });
        var iexcl = h('textarea', {rows: 3, spellcheck: 'false'});
        iexcl.value = job.iexcludes.join('\n');
        iexcl.addEventListener('input', function () { job.iexcludes = lines(iexcl.value); });
        var flags = h('textarea', {rows: 2, spellcheck: 'false', placeholder: '--exclude-if-present=.nobackup'});
        flags.value = a.extra_flags.join('\n');
        flags.addEventListener('input', function () { a.extra_flags = lines(flags.value); });
        var advanced = h('details', {class: 'rb-more'}, h('summary', {text: 'Advanced'}),
            h('div', {class: 'rb-fields'},
              RB.field('Tags', tags, {optional: true, hint: 'Added to every snapshot of this job.'}),
              RB.field('Host name', RB.input(a, 'host', {placeholder: o().system.server}), {optional: true,
                  hint: 'Recorded with each snapshot. Leave empty for the server name.'}),
              RB.field('Files read at once', RB.input(a, 'read_concurrency', {type: 'number', min: 0}),
                       {hint: '0 = restic\'s default (2). More helps on fast SSDs.'}),
              RB.field('Leave out, ignoring case', iexcl, {wide: true, optional: true}),
              RB.field('Extra restic options', flags, {wide: true, optional: true, hint: 'One per line, as --name=value.'}),
              h('div', {class: 'rb-field'}, RB.toggle(job, 'one_file_system', 'Stay on one file system')),
              h('div', {class: 'rb-field'}, RB.toggle(a, 'skip_if_unchanged', 'No snapshot if nothing changed'))));

        var body = [
            errorSlot,
            section('Basics', 'archive', null, basics),
            section('What to back up', 'folder', null, h('div', null, sources, h('div', {style: {marginTop: '18px'}}, excludes))),
            section('Schedule', 'calendar', null, schedule),
            section('Keep snapshots', 'layers',
                    'Each number keeps that many snapshots, one per hour, day, week and so on - the newest of each. ' +
                    'The latest snapshot is always kept.', retention),
            section('Consistency', 'shield', 'For data that is written to while it is being backed up.', consistency),
            section('Notifications', 'bell', null, notify),
            section('Commands', 'terminal', null, hooks),
            h('div', {class: 'rb-form-section'}, advanced)
        ];

        var save = RB.button(isNew ? 'Create job' : 'Save', {kind: 'primary', icon: 'check', onclick: function (e) {
            RB.busy(e.currentTarget, RB.api('job_save', {job: job})).then(function (res) {
                o_.close('saved');
                RB.toast(isNew ? 'Job "' + res.job.name + '" created.' : 'Saved.');
                RB.refresh();
            }).catch(function (err) {
                RB.clear(errorSlot).appendChild(RB.errorBox(err));
                o_.body.scrollTop = 0;
            });
        }});
        o_ = RB.overlay({
            drawer: true, title: isNew ? 'New backup job' : 'Edit "' + saved.name + '"', icon: 'archive',
            body: body,
            actions: [
                isNew ? null : RB.button('Delete', {icon: 'trash', kind: 'danger', onclick: function () {
                    deleteJob(saved).then(function (gone) {
                        if (gone) {
                            o_.close('deleted');
                        }
                    });
                }}),
                h('div', {class: 'rb-spacer'}),
                RB.button('Cancel', {onclick: function () { o_.close(); }}),
                save
            ]
        });
    };

    function lines(text) {
        return text.split('\n').map(function (l) { return l.trim(); }).filter(Boolean);
    }

    function previewRetention(job, btn) {
        RB.busy(btn, RB.api('retention_preview', {job: {id: job.id, retention: job.retention}})).then(function (r) {
            var list = function (items, keep) {
                if (!items.length) {
                    return h('div', {class: 'rb-hint', text: keep ? 'None.' : 'Nothing - every snapshot stays.'});
                }
                return h('div', {class: 'rb-table-wrap'}, h('table', {class: 'rb-table'}, h('tbody', null, items.map(function (s) {
                    return h('tr', null, h('td', {class: 'rb-mono', text: s.id.slice(0, 8)}),
                             h('td', {text: RB.dateTime(Date.parse(s.time) / 1000)}),
                             h('td', {class: 'dim', text: (s.reasons || []).join(', ')}));
                }))));
            };
            RB.overlay({
                title: 'What this policy would do', icon: 'eye', wide: true,
                body: [h('p', null, h('b', {text: RB.plural(r.keep.length, 'snapshot') + ' kept'}),
                         ', ', h('b', {class: r.remove.length ? 'rb-err-text' : null, text: r.remove.length + ' removed'}),
                         '. Nothing has been removed yet - this applies with the next backup after saving.'),
                       h('h4', {text: 'Kept', style: {margin: '16px 0 8px'}}), list(r.keep, true),
                       h('h4', {text: 'Removed', style: {margin: '16px 0 8px'}}), list(r.remove, false)]
            });
        }).catch(RB.fail);
    }
})(window.RB);
