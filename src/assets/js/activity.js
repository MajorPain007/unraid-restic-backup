(function (RB) {
    'use strict';
    var h = RB.h;
    var F = {job: '', repo: '', kind: '', status: ''};

    function o() {
        return RB.state.overview;
    }

    var KIND = {backup: 'Backup', forget: 'Retention', prune: 'Prune', check: 'Check', restore: 'Restore'};

    function what(r) {
        if (r.kind === 'backup' || r.kind === 'forget') {
            return (KIND[r.kind] || r.kind) + ' · ' + (r.job_name || r.job);
        }
        return (KIND[r.kind] || r.kind) + ' · ' + (r.repo_name || r.repo);
    }

    function result(r) {
        var s = r.summary || {};
        if (r.status === 'error' || r.status === 'cancelled') {
            return r.error || '';
        }
        if (r.kind === 'backup' && s.data_added_packed !== undefined) {
            var t = RB.bytes(s.data_added_packed) + ' added, ' + (s.files_new || 0).toLocaleString('en-GB') + ' new and ' +
                    (s.files_changed || 0).toLocaleString('en-GB') + ' changed files';
            if (s.forget_removed) {
                t += '; ' + RB.plural(s.forget_removed, 'old snapshot') + ' removed';
            }
            return r.error ? t + ' - ' + r.error : t;
        }
        if (r.kind === 'restore' && s.files_restored !== undefined) {
            return RB.plural(s.files_restored, 'file') + ' (' + RB.bytes(s.bytes_restored) + ') restored to ' + (s.target || '');
        }
        if (r.kind === 'forget' && s.forget_removed !== undefined) {
            return RB.plural(s.forget_removed, 'snapshot') + ' removed, ' + s.forget_kept + ' kept';
        }
        if (r.kind === 'prune' && s.total_size !== undefined) {
            return 'Repository now ' + RB.bytes(s.total_size);
        }
        if (r.kind === 'check' && s.num_errors !== undefined) {
            return s.num_errors ? RB.plural(s.num_errors, 'problem') + ' found' : 'No problems found';
        }
        return r.error || '';
    }

    RB.views.activity = {
        render: function (container) {
            var running = o().running || [];
            if (running.length) {
                container.appendChild(h('div', {class: 'rb-section-head'}, h('h2', {text: 'Running'})));
                var grid = h('div', {class: 'rb-grid', style: {marginBottom: '8px'}});
                running.forEach(function (op) {
                    grid.appendChild(h('div', {class: 'rb-card is-running'},
                        h('div', {class: 'rb-card-head'}, h('span', {class: 'rb-dot run'}),
                          h('div', {class: 'rb-card-title'}, h('h3', {text: what(op)}),
                            h('div', {class: 'rb-meta', text: 'Started ' + RB.relative(op.started) + (op.trigger === 'schedule' ? ' by the schedule' : '')})),
                          h('div', {class: 'rb-card-actions'},
                            RB.button('Log', {icon: 'list', small: true, onclick: function () { RB.showLog(op.id); }}),
                            RB.button('Stop', {icon: 'stop', small: true, onclick: function (e) { RB.stopOp(op, e.currentTarget); }}))),
                        RB.progressBlock(op)));
                });
                container.appendChild(grid);
            }

            var jobSel = h('select', {'aria-label': 'Job'}, h('option', {value: ''}, 'All jobs'),
                o().config.jobs.map(function (j) { return h('option', {value: j.id, selected: j.id === F.job}, j.name); }));
            var repoSel = h('select', {'aria-label': 'Repository'}, h('option', {value: ''}, 'All repositories'),
                o().config.repos.map(function (r) { return h('option', {value: r.id, selected: r.id === F.repo}, r.name); }));
            var kindSel = h('select', {'aria-label': 'Kind'}, h('option', {value: ''}, 'Everything'),
                Object.keys(KIND).map(function (k) { return h('option', {value: k, selected: k === F.kind}, KIND[k]); }));
            var statusSel = h('select', {'aria-label': 'Result'}, [['', 'Any result'], ['success', 'Succeeded'], ['warning', 'Warnings'],
                ['error', 'Failed'], ['cancelled', 'Stopped']].map(function (s) {
                    return h('option', {value: s[0], selected: s[0] === F.status}, s[1]);
                }));
            var table = h('div');
            [[jobSel, 'job'], [repoSel, 'repo'], [kindSel, 'kind'], [statusSel, 'status']].forEach(function (p) {
                p[0].addEventListener('change', function () { F[p[1]] = p[0].value; load(table); });
            });
            container.appendChild(h('div', {class: 'rb-section-head'}, h('h2', {text: 'History'})));
            container.appendChild(h('div', {class: 'rb-filters'}, jobSel, repoSel, kindSel, statusSel));
            container.appendChild(table);
            load(table);
        }
    };

    function load(table) {
        RB.clear(table).appendChild(h('div', {class: 'rb-hint'}, RB.icon('loader', 'rb-spin'), ' Loading...'));
        RB.api('history', {job: F.job, repo: F.repo, kind: F.kind, status: F.status, limit: 300}).then(function (r) {
            RB.clear(table);
            if (!r.history.length) {
                table.appendChild(h('div', {class: 'rb-empty'}, RB.icon('activity', 'big'), h('h3', {text: 'Nothing yet'}),
                                    h('p', {text: 'Backups, clean-ups, checks and restores appear here once they have run.'})));
                return;
            }
            table.appendChild(h('div', {class: 'rb-table-wrap'}, h('table', {class: 'rb-table'},
                h('thead', null, h('tr', null, h('th', {text: 'Result'}), h('th', {text: 'What'}), h('th', {class: 'rb-hide-narrow', text: 'Started'}),
                                   h('th', {class: 'num rb-hide-narrow', text: 'Took'}), h('th', {class: 'rb-hide-narrow', text: 'Details'}))),
                h('tbody', null, r.history.map(function (rec) {
                    return h('tr', {class: 'clickable', title: 'Show the log', onclick: function () { RB.showLog(rec.id); }},
                        h('td', {class: 'shrink'}, RB.statusPill(rec.status)),
                        h('td', null, h('div', {text: what(rec)}),
                          h('div', {class: 'rb-hint', text: (rec.trigger === 'schedule' ? 'Scheduled' : 'By hand')})),
                        h('td', {class: 'dim rb-hide-narrow', title: RB.dateTime(rec.started), text: RB.relative(rec.started)}),
                        h('td', {class: 'num rb-hide-narrow', text: RB.duration(rec.ended - rec.started)}),
                        h('td', {class: 'rb-hide-narrow', style: {maxWidth: '420px'}},
                          h('div', {class: rec.status === 'error' ? 'rb-err-text' : (rec.status === 'warning' ? 'rb-warn-text' : 'rb-dim'),
                                    style: {overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'},
                                    title: result(rec), text: result(rec)})));
                })))));
        }).catch(function (e) {
            RB.clear(table).appendChild(h('div', {class: 'rb-card-note err'}, RB.icon('alert'), h('div', {text: e.message})));
        });
    }

    RB.showLog = function (id) {
        var pre = h('pre', {class: 'rb-log', tabindex: '0'});
        var title = h('span', {text: 'Log'});
        var status = h('span');
        var offset = -1;
        var timer = null;
        var stick = true;
        var dlg = RB.overlay({title: 'Log', icon: 'list', wide: true, body: [h('div', {class: 'rb-inline', style: {marginBottom: '10px'}},
                                                                           title, status), pre],
                              actions: [RB.button('Copy', {icon: 'copy', small: true, onclick: function () { RB.copyText(pre.textContent); }}),
                                        h('div', {class: 'rb-spacer'}),
                                        RB.button('Close', {onclick: function () { dlg.close(); }})],
                              onclose: function () { clearTimeout(timer); }});
        pre.addEventListener('scroll', function () {
            stick = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 20;
        });

        function addLines(text) {
            text.split('\n').forEach(function (line) {
                if (line === '') {
                    return;
                }
                var cls = /\s==\s/.test(line) ? 'phase' : (/\s\$ restic /.test(line) ? 'cmd'
                    : (/error|failed|fatal|refused|denied/i.test(line) ? 'bad' : null));
                pre.appendChild(cls ? h('span', {class: cls, text: line + '\n'}) : document.createTextNode(line + '\n'));
            });
            if (stick) {
                pre.scrollTop = pre.scrollHeight;
            }
        }

        function poll() {
            RB.api('op_log', {id: id, offset: offset}).then(function (r) {
                var op = r.op || {};
                RB.clear(title).appendChild(h('b', {text: what(op)}));
                title.appendChild(document.createTextNode(' · ' + RB.dateTime(op.started)));
                RB.clear(status).appendChild(RB.statusPill(r.status));
                if (offset < 0 && r.offset - r.text.length > 0) {
                    pre.appendChild(h('span', {class: 'rb-dim', text: '(earlier lines left out)\n'}));
                }
                addLines(r.text);
                offset = r.offset;
                if (r.status === 'running' && pre.isConnected) {
                    timer = setTimeout(poll, 1500);
                }
            }).catch(function (e) {
                pre.textContent = e.message;
            });
        }
        poll();
        return dlg;
    };
})(window.RB);
