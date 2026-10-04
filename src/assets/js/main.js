(function (RB) {
    'use strict';
    var h = RB.h;
    var root = RB.root;
    var TABS = [
        ['jobs', 'Jobs', 'archive'],
        ['repos', 'Repositories', 'repo'],
        ['snapshots', 'Snapshots', 'layers'],
        ['activity', 'Activity', 'activity'],
        ['settings', 'Settings', 'settings']
    ];
    var header = h('div', {class: 'rb-header'});
    var banners = h('div');
    var tabs = h('div', {class: 'rb-tabs', role: 'tablist'});
    var view = h('div', {class: 'rb-view'});
    var current = RB.pref('tab') || 'jobs';
    var pollTimer = null;
    var lastRunning = '';

    RB.state.tab = current;

    RB.show = function (tab, opts) {
        if (!RB.views[tab]) {
            tab = 'jobs';
        }
        current = tab;
        RB.state.tab = tab;
        RB.pref('tab', tab);
        RB.state.viewOpts = opts || {};
        render();
        window.scrollTo({top: Math.min(window.scrollY, root.offsetTop), behavior: 'smooth'});
    };

    RB.refresh = function () {
        return RB.api('overview').then(function (data) {
            RB.state.overview = data;
            render();
            schedulePoll();
            return data;
        }).catch(function (e) {
            if (!RB.state.overview) {
                RB.clear(view).appendChild(h('div', {class: 'rb-empty'}, RB.icon('alert', 'big'),
                    h('h3', {text: 'The plugin could not load its data'}), h('p', {text: e.message}),
                    RB.button('Try again', {icon: 'refresh', onclick: RB.refresh})));
            } else {
                RB.fail(e);
            }
        });
    };

    function schedulePoll() {
        clearTimeout(pollTimer);
        var running = (RB.state.overview && RB.state.overview.running) || [];
        pollTimer = setTimeout(running.length ? pollRunning : RB.refresh, running.length ? 2000 : 20000);
    }

    function pollRunning() {
        RB.api('ops').then(function (r) {
            var key = r.running.map(function (o) { return o.id; }).sort().join(',');
            if (key !== lastRunning) {
                lastRunning = key;
                return RB.refresh();
            }
            RB.state.overview.running = r.running;
            RB.emit('progress', r.running);
            schedulePoll();
        }).catch(function () {
            schedulePoll();
        });
    }

    function overallStatus(o) {
        var running = o.running.filter(function (r) { return r.kind === 'backup'; }).length;
        var failed = 0;
        var warned = 0;
        var never = 0;
        o.jobs.forEach(function (j) {
            var job = RB.jobConfig(j.id);
            if (!job || !job.enabled) {
                return;
            }
            if (!j.last) {
                never++;
            } else if (j.last.status === 'error') {
                failed++;
            } else if (j.last.status === 'warning') {
                warned++;
            }
        });
        if (!o.config.jobs.length) {
            return ['idle', o.config.repos.length ? 'No backup jobs yet' : 'Not set up yet', null];
        }
        if (failed) {
            return ['err', RB.plural(failed, 'backup') + ' failed', 'Look at the cards marked red.'];
        }
        if (running) {
            return ['run', RB.plural(running, 'backup') + ' running', null];
        }
        if (warned) {
            return ['warn', RB.plural(warned, 'backup') + ' finished with warnings', null];
        }
        if (never === o.config.jobs.length) {
            return ['idle', 'Ready', 'No backup has run yet.'];
        }
        return ['ok', 'All backups are fine', null];
    }

    function renderHeader() {
        var o = RB.state.overview;
        RB.clear(header);
        var st = overallStatus(o);
        var sys = o.system;
        header.appendChild(h('div', {class: 'rb-header-main'},
            h('div', {class: 'rb-status-line'}, h('span', {class: 'rb-dot ' + st[0], style: {marginTop: 0}}),
              h('span', {text: st[1]}), st[2] ? h('span', {class: 'rb-sub', text: st[2]}) : null),
            h('div', {class: 'rb-sub', style: {marginTop: '3px'}},
              (sys.restic ? 'restic ' + sys.restic : 'restic not installed') + '  ·  ' + sys.server)));
        var actions = h('div', {class: 'rb-header-actions'});
        if (!o.config.repos.length) {
            actions.appendChild(RB.button('Add repository', {icon: 'plus', kind: 'primary', onclick: function () { RB.editRepo(null); }}));
        } else {
            actions.appendChild(RB.button('New job', {icon: 'plus', kind: 'primary', onclick: function () { RB.editJob(null); }}));
        }
        header.appendChild(actions);

        RB.clear(banners);
        var note = function (kind, iconName, text) {
            banners.appendChild(h('div', {class: 'rb-card-note ' + kind, style: {marginTop: 0, marginBottom: '14px'}},
                                  RB.icon(iconName), h('div', null, text)));
        };
        if (!sys.array_started) {
            note('warn', 'power', 'The array is stopped. Backups wait until it is started; missed ones run then.');
        }
        if (!sys.restic) {
            note('err', 'alert', 'restic is not installed. Reinstall the plugin, and check the plugin log for why the download failed.');
        }
        var scheduled = o.config.jobs.some(function (j) { return j.enabled && j.schedule.mode !== 'off'; });
        var fresh = !sys.scheduler_tick && sys.installed && o.now - sys.installed < 180;
        if (scheduled && !fresh && (!sys.scheduler_tick || o.now - sys.scheduler_tick > 180)) {
            note('err', 'clock', sys.scheduler_tick
                ? 'The scheduler last ran ' + RB.relative(sys.scheduler_tick) + ' - scheduled backups are not starting. ' +
                  'Reinstalling the plugin registers it with cron again.'
                : 'The scheduler has not run yet. If this stays, scheduled backups will not start - reinstalling the plugin registers it with cron again.');
        }
        if (sys.array_started && !sys.data_dir_ok && o.config.jobs.length) {
            note('warn', 'folder', 'The data folder ' + o.config.settings.data_dir +
                 ' does not exist yet. It is created with the first backup; if it is on a disk that is gone, choose another in Settings.');
        }
    }

    function renderTabs() {
        RB.clear(tabs);
        var o = RB.state.overview;
        var counts = {
            jobs: o.config.jobs.length,
            repos: o.config.repos.length,
            activity: o.running.length
        };
        TABS.forEach(function (t) {
            var failed = t[0] === 'jobs' && o.jobs.some(function (j) { return j.last && j.last.status === 'error'; });
            tabs.appendChild(h('button', {type: 'button', role: 'tab', class: 'rb-tab' + (t[0] === current ? ' active' : ''),
                                          'aria-selected': t[0] === current ? 'true' : 'false',
                                          onclick: function () { RB.show(t[0]); }},
                RB.icon(t[2]), t[1],
                counts[t[0]] ? h('span', {class: 'rb-count' + (failed ? ' err' : ''), text: String(counts[t[0]])}) : null));
        });
    }

    var shown = null;
    var waiting = false;

    RB.on('menuclosed', function () {
        if (waiting) {
            waiting = false;
            render();
        }
    });

    function render() {
        if (!RB.state.overview) {
            return;
        }
        if (RB.menuOpen()) {
            waiting = true;
            return;
        }
        renderHeader();
        renderTabs();
        var v = RB.views[current];
        var opts = RB.state.viewOpts || {};
        RB.state.viewOpts = {};
        if (v.keep && shown === current && !Object.keys(opts).length && view.firstChild) {
            if (v.update) {
                v.update();
            }
            return;
        }
        shown = current;
        var scroll = window.scrollY;
        RB.clear(view);
        v.render(view, opts);
        window.scrollTo(0, scroll);
    }

    RB.render = render;

    RB.jobConfig = function (id) {
        return (RB.state.overview.config.jobs || []).filter(function (j) { return j.id === id; })[0] || null;
    };
    RB.repoConfig = function (id) {
        return (RB.state.overview.config.repos || []).filter(function (r) { return r.id === id; })[0] || null;
    };
    RB.jobInfo = function (id) {
        return RB.state.overview.jobs.filter(function (j) { return j.id === id; })[0] || null;
    };
    RB.repoInfo = function (id) {
        return RB.state.overview.repos.filter(function (r) { return r.id === id; })[0] || null;
    };

    function boot() {
        RB.clear(root);
        root.appendChild(header);
        root.appendChild(banners);
        root.appendChild(tabs);
        root.appendChild(view);
        RB.applyTheme();
        RB.refresh();
        document.addEventListener('visibilitychange', function () {
            if (!document.hidden) {
                RB.refresh();
            }
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', boot);
    } else {
        boot();
    }
})(window.RB);
