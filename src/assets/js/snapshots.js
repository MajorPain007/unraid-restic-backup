(function (RB) {
    'use strict';
    var h = RB.h;
    var S = RB.state.snap = {repo: null, job: '', list: null, names: {}, loading: false, error: null,
                             snap: null, path: null, entries: null, listing: false, checked: {}};
    var els = {};

    function o() {
        return RB.state.overview;
    }

    RB.views.snapshots = {
        keep: true,
        render: function (container, opts) {
            var repos = o().config.repos;
            if (!repos.length) {
                container.appendChild(h('div', {class: 'rb-empty'}, RB.icon('layers', 'big'),
                    h('h3', {text: 'No repository yet'}), h('p', {text: 'Snapshots appear here once a repository is set up.'})));
                return;
            }
            if (opts.repo && opts.repo !== S.repo) {
                selectRepo(opts.repo, opts.job || '');
            } else if (opts.job !== undefined && opts.job !== S.job) {
                S.job = opts.job;
            }
            if (!S.repo || !RB.repoConfig(S.repo)) {
                selectRepo(RB.pref('snapRepo') && RB.repoConfig(RB.pref('snapRepo')) ? RB.pref('snapRepo') : repos[0].id, '');
            }
            els.container = container;
            build();
        },
        update: function () {
            renderRunning();
        }
    };

    function build() {
        var c = RB.clear(els.container);
        var repoSel = h('select', {'aria-label': 'Repository'}, o().config.repos.map(function (r) {
            return h('option', {value: r.id, selected: r.id === S.repo}, r.name);
        }));
        repoSel.addEventListener('change', function () { selectRepo(repoSel.value, ''); build(); });
        var jobSel = h('select', {'aria-label': 'Job'});
        var search = h('input', {type: 'search', placeholder: 'Find files, e.g. *.jpg', 'aria-label': 'Find files'});
        search.addEventListener('keydown', function (e) {
            if (e.key === 'Enter' && search.value.trim()) {
                find(search.value.trim());
            }
        });
        els.jobSel = jobSel;
        jobSel.addEventListener('change', function () {
            S.job = jobSel.value;
            renderList();
        });
        c.appendChild(h('div', {class: 'rb-toolbar'}, repoSel, jobSel,
            RB.button('', {icon: 'refresh', title: 'Reload the snapshot list', onclick: function (e) {
                RB.busy(e.currentTarget, loadList(true));
            }}),
            h('div', {class: 'rb-spacer'}), search,
            RB.button('Find', {icon: 'search', onclick: function () {
                if (search.value.trim()) {
                    find(search.value.trim());
                }
            }})));
        els.running = h('div');
        c.appendChild(els.running);
        els.list = h('div', {class: 'rb-snaplist'});
        els.browser = h('div', {class: 'rb-browser'});
        c.appendChild(h('div', {class: 'rb-split'}, els.list, els.browser));
        renderRunning();
        if (!S.list && !S.loading) {
            loadList(false);
        }
        renderList();
        renderBrowser();
    }

    function selectRepo(id, job) {
        if (S.repo !== id) {
            S.repo = id;
            S.list = null;
            S.snap = null;
            S.entries = null;
            S.checked = {};
            S.error = null;
        }
        S.job = job || '';
        RB.pref('snapRepo', id);
    }

    function loadList(refresh) {
        S.loading = true;
        S.error = null;
        renderList();
        var repo = S.repo;
        return RB.api('snapshots', {repo: repo, refresh: refresh ? 1 : 0}).then(function (r) {
            if (repo !== S.repo) {
                return;
            }
            S.list = r.snapshots;
            S.names = r.job_names;
            S.loading = false;
            if (S.snap && !S.list.some(function (s) { return s.id === S.snap.id; })) {
                S.snap = null;
                S.entries = null;
            }
            renderList();
            renderBrowser();
        }).catch(function (e) {
            S.loading = false;
            S.error = e.message;
            renderList();
        });
    }

    function renderRunning() {
        if (!els.running || !els.running.isConnected) {
            return;
        }
        RB.clear(els.running);
        (o().running || []).filter(function (op) { return op.kind === 'restore' && op.repo === S.repo; }).forEach(function (op) {
            els.running.appendChild(h('div', {class: 'rb-card is-running', style: {marginBottom: '14px'}},
                h('div', {class: 'rb-card-head'}, h('span', {class: 'rb-dot run'}),
                  h('div', {class: 'rb-card-title'}, h('h3', {text: 'Restore in progress'}),
                    h('div', {class: 'rb-meta', text: 'Started ' + RB.relative(op.started)})),
                  RB.button('Stop', {icon: 'stop', small: true, onclick: function (e) { RB.stopOp(op, e.currentTarget); }})),
                RB.progressBlock(op)));
        });
    }

    function jobName(s) {
        return s.job ? (S.names[s.job] || 'Job ' + s.job) : (s.tags.length ? s.tags.join(', ') : 'Not from a job');
    }

    function renderList() {
        var el = els.list;
        if (!el) {
            return;
        }
        RB.clear(el);
        var jobIds = {};
        (S.list || []).forEach(function (s) { jobIds[s.job] = true; });
        if (S.job) {
            jobIds[S.job] = true;
        }
        RB.clear(els.jobSel);
        els.jobSel.appendChild(h('option', {value: ''}, 'All jobs'));
        Object.keys(jobIds).forEach(function (id) {
            els.jobSel.appendChild(h('option', {value: id, selected: id === S.job},
                id ? (S.names[id] || (RB.jobConfig(id) || {}).name || 'Job ' + id) : 'Not from a job'));
        });
        if (S.loading && !S.list) {
            el.appendChild(h('div', {class: 'rb-hint', style: {padding: '16px'}}, RB.icon('loader', 'rb-spin'), ' Loading snapshots...'));
            return;
        }
        if (S.error) {
            el.appendChild(h('div', {class: 'rb-card-note err', style: {margin: '12px'}}, RB.icon('alert'), h('div', {text: S.error})));
            return;
        }
        var list = (S.list || []).filter(function (s) { return !S.job || s.job === S.job; });
        if (!list.length) {
            el.appendChild(h('div', {class: 'rb-hint', style: {padding: '16px'}, text: 'No snapshots yet.'}));
            return;
        }
        var lastDay = '';
        list.forEach(function (s) {
            var day = new Date(s.time * 1000).toDateString();
            if (day !== lastDay) {
                lastDay = day;
                el.appendChild(h('div', {class: 'rb-snap-day', text: RB.dayLabel(s.time)}));
            }
            el.appendChild(h('button', {type: 'button', class: 'rb-snap' + (S.snap && S.snap.id === s.id ? ' active' : ''),
                                        title: s.id, onclick: function () { openSnapshot(s); }},
                h('span', {class: 'rb-snap-time', text: RB.clock(s.time)}),
                h('span', {class: 'rb-snap-main', text: jobName(s) + (s.hostname !== o().system.server ? ' · ' + s.hostname : '')}),
                s.summary ? h('span', {class: 'rb-snap-size', title: 'Added by this snapshot', text: '+' + RB.bytes(s.summary.added)}) : null));
        });
    }

    function commonParent(paths) {
        if (!paths.length) {
            return '/';
        }
        if (paths.length === 1) {
            return paths[0];
        }
        var parts = paths.map(function (p) { return p.split('/'); });
        var out = [];
        for (var i = 0; i < parts[0].length; i++) {
            var seg = parts[0][i];
            if (parts.every(function (p) { return p[i] === seg; })) {
                out.push(seg);
            } else {
                break;
            }
        }
        return out.join('/') || '/';
    }

    function openSnapshot(s, path) {
        S.snap = s;
        S.checked = {};
        renderList();
        navigate(path || commonParent(s.paths));
    }

    function navigate(path) {
        S.path = path;
        S.listing = true;
        S.opening = false;
        S.checked = {};
        renderBrowser();
        var snap = S.snap;
        (function ask() {
            RB.api('ls', {repo: S.repo, snapshot: snap.id, path: path}).then(function (r) {
                if (S.snap !== snap || S.path !== path) {
                    return;
                }
                if (r.pending) {
                    if (!S.opening) {
                        S.opening = true;
                        renderBrowser();
                    }
                    setTimeout(ask, 600);
                    return;
                }
                S.entries = r.entries;
                S.listing = false;
                S.opening = false;
                renderBrowser();
            }).catch(function (e) {
                if (S.snap !== snap || S.path !== path) {
                    return;
                }
                S.listing = false;
                S.opening = false;
                S.entries = [];
                renderBrowser();
                RB.fail(e);
            });
        })();
    }

    function renderBrowser() {
        var el = els.browser;
        if (!el) {
            return;
        }
        RB.clear(el);
        if (!S.snap) {
            el.appendChild(h('div', {class: 'rb-empty', style: {border: 0}}, RB.icon('layers', 'big'),
                h('h3', {text: 'Choose a snapshot'}), h('p', {text: 'Its files appear here, to look through, restore or download.'})));
            return;
        }
        var s = S.snap;
        var sum = s.summary;
        el.appendChild(h('div', {class: 'rb-snap-info'},
            h('span', null, h('b', {text: RB.dateTime(s.time)})),
            h('span', {text: jobName(s)}),
            sum ? h('span', null, 'Size ', h('b', {text: RB.bytes(sum.size)})) : null,
            sum ? h('span', null, 'Added ', h('b', {text: RB.bytes(sum.added)})) : null,
            sum ? h('span', null, h('b', {text: (sum.files || 0).toLocaleString('en-GB')}), ' files') : null,
            h('span', {class: 'rb-mono', text: s.short_id}),
            h('span', {style: {flex: 1}}),
            RB.button('Compare', {icon: 'compare', small: true, kind: 'ghost', onclick: function () { compare(s); }}),
            RB.button('', {icon: 'more', small: true, kind: 'ghost', title: 'More', onclick: function (e) { snapMenu(s, e.currentTarget); }})));

        var crumbs = h('div', {class: 'rb-crumbs'});
        crumbs.appendChild(h('button', {type: 'button', title: '/', onclick: function () { navigate('/'); }}, RB.icon('home')));
        var parts = S.path.split('/').filter(Boolean);
        parts.forEach(function (p, i) {
            var target = '/' + parts.slice(0, i + 1).join('/');
            crumbs.appendChild(h('span', {class: 'rb-sep', text: '/'}));
            crumbs.appendChild(h('button', {type: 'button', text: p, onclick: function () { navigate(target); }}));
        });
        el.appendChild(h('div', {class: 'rb-browser-head'}, crumbs,
            S.path !== '/' ? RB.button('', {icon: 'up', small: true, kind: 'ghost', title: 'Up',
                onclick: function () { navigate(S.path.replace(/\/[^/]+$/, '') || '/'); }}) : null));

        var scroll = h('div', {class: 'rb-browser-scroll'});
        if (S.listing) {
            scroll.appendChild(h('div', {class: 'rb-hint', style: {padding: '16px'}}, RB.icon('loader', 'rb-spin'),
                S.opening ? ' Opening the repository for browsing. This takes a moment once; after that, folders open right away.'
                          : ' Reading the folder...'));
        } else {
            var entries = S.entries || [];
            var all = h('input', {type: 'checkbox', 'aria-label': 'Select all',
                                  checked: entries.length > 0 && entries.every(function (e) { return S.checked[e.path]; })});
            all.addEventListener('change', function () {
                S.checked = {};
                if (all.checked) {
                    entries.forEach(function (e) { S.checked[e.path] = e; });
                }
                renderBrowser();
            });
            var rows = entries.map(function (e) {
                var box = h('input', {type: 'checkbox', checked: !!S.checked[e.path], 'aria-label': 'Select ' + e.name});
                box.addEventListener('click', function (ev) { ev.stopPropagation(); });
                box.addEventListener('change', function () {
                    if (box.checked) {
                        S.checked[e.path] = e;
                    } else {
                        delete S.checked[e.path];
                    }
                    renderSelbar();
                });
                var isDir = e.type === 'dir';
                return h('tr', {class: 'clickable', onclick: function () {
                    if (isDir) {
                        navigate(e.path);
                    } else {
                        box.checked = !box.checked;
                        box.dispatchEvent(new Event('change'));
                    }
                }},
                    h('td', {class: 'shrink'}, box),
                    h('td', {class: 'name'}, RB.icon(isDir ? 'folder' : (e.type === 'symlink' ? 'link' : 'file'), isDir ? 'dir' : 'file'),
                      h('span', {text: e.name, title: e.path})),
                    h('td', {class: 'num rb-hide-narrow', text: isDir ? '' : RB.bytes(e.size)}),
                    h('td', {class: 'dim rb-hide-narrow', text: e.mtime ? RB.dateTime(e.mtime) : ''}));
            });
            scroll.appendChild(h('table', {class: 'rb-table'},
                h('thead', null, h('tr', null, h('th', {class: 'shrink'}, all), h('th', {text: 'Name'}),
                                   h('th', {class: 'num rb-hide-narrow', text: 'Size'}), h('th', {class: 'rb-hide-narrow', text: 'Modified'}))),
                h('tbody', null, rows.length ? rows : h('tr', null, h('td', {colspan: 4, class: 'dim', text: 'Empty folder.'})))));
        }
        el.appendChild(scroll);
        els.selbar = h('div', {class: 'rb-selbar'});
        el.appendChild(els.selbar);
        renderSelbar();
    }

    function renderSelbar() {
        var bar = els.selbar;
        if (!bar) {
            return;
        }
        RB.clear(bar);
        var items = Object.keys(S.checked).map(function (k) { return S.checked[k]; });
        if (!items.length) {
            bar.appendChild(h('span', {text: 'Select files and folders, or:'}));
            bar.appendChild(h('div', {class: 'rb-spacer'}));
            bar.appendChild(RB.button('Download folder', {icon: 'download', small: true, disabled: S.path === '/',
                onclick: function () { RB.download({repo: S.repo, snapshot: S.snap.id, path: S.path}); }}));
            bar.appendChild(RB.button('Restore this folder...', {icon: 'restore', small: true, kind: 'primary',
                disabled: S.path === '/', onclick: function () { restoreDialog([{path: S.path, type: 'dir', name: S.path}]); }}));
            return;
        }
        bar.appendChild(h('b', {text: RB.plural(items.length, 'item') + ' selected'}));
        bar.appendChild(h('span', {class: 'rb-link', text: 'Clear', onclick: function () { S.checked = {}; renderBrowser(); }}));
        bar.appendChild(h('div', {class: 'rb-spacer'}));
        bar.appendChild(RB.button('Download', {icon: 'download', small: true, disabled: items.length !== 1,
            title: items.length !== 1 ? 'One item at a time' : null,
            onclick: function () { RB.download({repo: S.repo, snapshot: S.snap.id, path: items[0].path}); }}));
        bar.appendChild(RB.button('Restore...', {icon: 'restore', small: true, kind: 'primary',
            onclick: function () { restoreDialog(items); }}));
    }

    function snapMenu(s, anchor) {
        RB.menu(anchor, [
            {label: 'Copy snapshot ID', icon: 'copy', onclick: function () { RB.copyText(s.id); }},
            {label: 'Restore everything...', icon: 'restore', onclick: function () {
                restoreDialog(s.paths.map(function (p) { return {path: p, type: 'dir', name: p}; }));
            }},
            '-',
            {label: 'Delete snapshot', icon: 'trash', danger: true, onclick: function () { deleteSnapshot(s); }}
        ]);
    }

    function deleteSnapshot(s) {
        RB.confirm({title: 'Delete this snapshot?',
                    text: 'The snapshot of ' + RB.dateTime(s.time) + ' (' + jobName(s) + ') is removed for good. ' +
                          'Its data is freed by the next prune, unless other snapshots share it.',
                    confirm: 'Delete snapshot', danger: true}).then(function (yes) {
            if (!yes) {
                return;
            }
            RB.api('snapshot_forget', {repo: S.repo, ids: [s.id]}).then(function () {
                RB.toast('Snapshot deleted.');
                S.snap = null;
                S.entries = null;
                loadList(true);
                renderBrowser();
            }).catch(RB.fail);
        });
    }

    var OVERWRITE = [
        ['if-changed', 'Replace files that differ from the snapshot'],
        ['if-newer', 'Replace files only where the snapshot\'s copy is newer'],
        ['never', 'Keep files that are there; only add missing ones'],
        ['always', 'Replace every file']
    ];

    function restoreDialog(items) {
        var snap = S.snap;
        var req = {target: 'original', target_path: '/mnt/user/restore', overwrite: 'if-changed', delete: false};
        var detail = h('div', {style: {marginTop: '14px'}});
        var dlg;

        function renderDetail() {
            RB.clear(detail);
            if (req.target === 'path') {
                var p = RB.input(req, 'target_path', {mono: true});
                detail.appendChild(RB.field('Restore into', h('div', {class: 'rb-input-group'}, p,
                    RB.button('Browse...', {icon: 'folder', onclick: function () {
                        RB.pickPath({title: 'Restore into', create: true, start: '/mnt/user'}).then(function (path) {
                            if (path) {
                                req.target_path = path;
                                p.value = path;
                            }
                        });
                    }})), {hint: 'Each item lands directly in this folder, which is created if needed.'}));
            }
            var ow = RB.select(req, 'overwrite', OVERWRITE);
            detail.appendChild(h('div', {style: {marginTop: '12px'}}, RB.field('Files that already exist', ow)));
            if (req.target === 'original') {
                detail.appendChild(h('div', {style: {marginTop: '14px'}},
                    RB.toggle(req, 'delete', 'Also delete files that are not in the snapshot'),
                    h('div', {class: 'rb-hint', style: {margin: '4px 0 0 44px'}},
                      'Makes the folders exactly as they were. Files created since are lost.')));
            }
        }
        renderDetail();

        var list = items.slice(0, 6).map(function (it) {
            return h('div', {class: 'rb-pathlist-item'}, RB.icon(it.type === 'dir' ? 'folder' : 'file'),
                     h('span', {class: 'rb-mono', text: it.path, title: it.path}));
        });
        if (items.length > 6) {
            list.push(h('div', {class: 'rb-hint', text: 'and ' + (items.length - 6) + ' more'}));
        }
        dlg = RB.overlay({
            title: 'Restore from ' + RB.dateTime(snap.time), icon: 'restore', wide: true,
            body: [h('div', {class: 'rb-pathlist'}, list),
                   h('div', {style: {marginTop: '18px'}}, RB.optionCards(req.target, [
                       {value: 'original', title: 'Where they came from', icon: 'restore', text: 'Puts the files back in place.'},
                       {value: 'path', title: 'Into another folder', icon: 'folder', text: 'Leaves what is there untouched, to compare first.'}
                   ], function (v) {
                       req.target = v;
                       req.overwrite = v === 'original' ? 'if-changed' : 'always';
                       renderDetail();
                   })),
                   detail],
            actions: [h('div', {class: 'rb-spacer'}),
                      RB.button('Cancel', {onclick: function () { dlg.close(); }}),
                      RB.button('Restore', {kind: 'primary', icon: 'restore', onclick: function (e) {
                          var go = function () {
                              RB.busy(e.currentTarget, RB.api('restore_start', {
                                  repo: S.repo, snapshot: snap.id, items: items.map(function (i) { return i.path; }),
                                  target: req.target, target_path: req.target_path, overwrite: req.overwrite, delete: req.delete ? 1 : 0
                              })).then(function () {
                                  dlg.close();
                                  S.checked = {};
                                  renderBrowser();
                                  RB.toast('Restore started - follow it at the top of this page.');
                                  setTimeout(RB.refresh, 800);
                              }).catch(RB.fail);
                          };
                          if (req.delete) {
                              RB.confirm({title: 'Delete files not in the snapshot?',
                                          text: 'Anything in these folders that is newer than the snapshot is deleted.',
                                          confirm: 'Restore and delete', danger: true}).then(function (yes) { if (yes) { go(); } });
                          } else {
                              go();
                          }
                      }})]
        });
    }

    function find(pattern) {
        var body = h('div', null, h('div', {class: 'rb-hint'}, RB.icon('loader', 'rb-spin'), ' Searching every snapshot...'));
        var dlg = RB.overlay({title: 'Find "' + pattern + '"', icon: 'search', wide: true, body: body});
        RB.api('find', {repo: S.repo, pattern: pattern}).then(function (r) {
            RB.clear(body);
            var total = r.results.reduce(function (n, g) { return n + g.matches.length; }, 0);
            if (!total) {
                body.appendChild(h('p', {text: 'Nothing found. Patterns match whole names: try *' + pattern + '*.'}));
                return;
            }
            var byId = {};
            (S.list || []).forEach(function (s) { byId[s.id] = s; });
            r.results.forEach(function (g) {
                if (!g.matches.length) {
                    return;
                }
                var s = byId[g.snapshot] || {id: g.snapshot, time: 0, paths: [], tags: [], job: ''};
                body.appendChild(h('div', {class: 'rb-section-head', style: {margin: '16px 0 8px'}},
                    h('h2', {text: (s.time ? RB.dateTime(s.time) : g.snapshot.slice(0, 8)) + ' · ' + RB.plural(g.matches.length, 'match', 'matches')})));
                body.appendChild(h('div', {class: 'rb-table-wrap'}, h('table', {class: 'rb-table'}, h('tbody', null,
                    g.matches.slice(0, 200).map(function (m) {
                        return h('tr', {class: 'clickable', title: 'Show it in the snapshot', onclick: function () {
                            dlg.close();
                            if (byId[g.snapshot]) {
                                openSnapshot(byId[g.snapshot], m.type === 'dir' ? m.path : m.path.replace(/\/[^/]+$/, '') || '/');
                            }
                        }}, h('td', {class: 'name'}, RB.icon(m.type === 'dir' ? 'folder' : 'file', m.type === 'dir' ? 'dir' : 'file'),
                              h('span', {class: 'rb-mono', text: m.path})),
                            h('td', {class: 'num', text: m.type === 'dir' ? '' : RB.bytes(m.size)}));
                    })))));
            });
            if (r.truncated) {
                body.appendChild(h('p', {class: 'rb-hint', text: 'Only the first 2000 matches are shown.'}));
            }
        }).catch(function (e) {
            RB.clear(body).appendChild(h('div', {class: 'rb-card-note err'}, RB.icon('alert'), h('div', {text: e.message})));
        });
    }

    function compare(s) {
        var list = (S.list || []).filter(function (x) { return x.id !== s.id; });
        if (!list.length) {
            RB.toast('There is no other snapshot to compare with.', 'info');
            return;
        }
        var older = list.filter(function (x) { return x.time < s.time && x.job === s.job; });
        var other = older.length ? older[0] : list[0];
        var sel = h('select', null, list.map(function (x) {
            return h('option', {value: x.id, selected: x.id === other.id}, RB.dateTime(x.time) + ' · ' + jobName(x));
        }));
        var out = h('div');
        var filter = h('input', {type: 'search', placeholder: 'Filter paths'});
        var data = null;

        function show() {
            RB.clear(out);
            if (!data) {
                return;
            }
            var st = data.statistics || {};
            var a = st.added || {};
            var rm = st.removed || {};
            out.appendChild(h('div', {class: 'rb-inline', style: {margin: '14px 0'}},
                h('span', {class: 'rb-pill ok', text: '+' + (a.files || 0) + ' files · ' + RB.bytes(a.bytes || 0)}),
                h('span', {class: 'rb-pill err', text: '-' + (rm.files || 0) + ' files · ' + RB.bytes(rm.bytes || 0)}),
                h('span', {class: 'rb-pill warn', text: (st.changed_files || 0) + ' changed'})));
            var q = filter.value.toLowerCase();
            var rows = data.changes.filter(function (c) { return !q || c.path.toLowerCase().indexOf(q) !== -1; });
            var box = h('div', {class: 'rb-table-wrap', style: {maxHeight: '50vh', overflowY: 'auto'}});
            rows.slice(0, 3000).forEach(function (c) {
                var kind = c.modifier === '+' ? 'add' : (c.modifier === '-' ? 'del' : 'mod');
                box.appendChild(h('div', {class: 'rb-diff-line ' + kind}, h('b', {text: c.modifier}), h('span', {text: c.path})));
            });
            if (!rows.length) {
                box.appendChild(h('div', {class: 'rb-hint', style: {padding: '12px'}, text: 'No differences.'}));
            }
            out.appendChild(box);
            if (data.total > data.changes.length) {
                out.appendChild(h('p', {class: 'rb-hint', text: 'Showing ' + data.changes.length + ' of ' + data.total + ' changes.'}));
            }
        }

        function load() {
            var otherSnap = list.filter(function (x) { return x.id === sel.value; })[0];
            var from = otherSnap.time < s.time ? otherSnap : s;
            var to = from === s ? otherSnap : s;
            RB.clear(out).appendChild(h('div', {class: 'rb-hint', style: {marginTop: '14px'}}, RB.icon('loader', 'rb-spin'), ' Comparing...'));
            RB.api('diff', {repo: S.repo, from: from.id, to: to.id}).then(function (r) {
                data = r;
                show();
            }).catch(function (e) {
                RB.clear(out).appendChild(h('div', {class: 'rb-card-note err'}, RB.icon('alert'), h('div', {text: e.message})));
            });
        }

        sel.addEventListener('change', load);
        filter.addEventListener('input', show);
        RB.overlay({title: 'Changes between snapshots', icon: 'compare', wide: true,
                    body: [h('div', {class: 'rb-fields'}, RB.field('Compare ' + RB.dateTime(s.time) + ' with', sel),
                             RB.field('Filter', filter)), out]});
        load();
    }
})(window.RB);
