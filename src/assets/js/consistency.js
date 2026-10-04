(function (RB) {
    'use strict';
    var h = RB.h;

    RB.consistencyPicker = function (job) {
        var c = job.consistency;
        ['containers', 'vms', 'skip'].forEach(function (k) {
            c[k] = c[k] || [];
        });
        var box = h('div');
        var firstTime = !c.containers.length && !c.vms.length && !c.skip.length;

        function load() {
            RB.clear(box).appendChild(h('div', {class: 'rb-hint'}, RB.icon('loader', 'rb-spin'), ' Looking at what uses these folders...'));
            RB.api('consistency_detect', {sources: job.sources}).then(render).catch(function (e) {
                RB.clear(box).appendChild(h('div', {class: 'rb-card-note err'}, RB.icon('alert'), h('div', {text: e.message})));
            });
        }

        function setChoice(name, kind, on) {
            var list = kind === 'vm' ? c.vms : c.containers;
            var i = list.indexOf(name);
            if (on && i === -1) {
                list.push(name);
            } else if (!on && i !== -1) {
                list.splice(i, 1);
            }
            var s = c.skip.indexOf(name);
            if (on && s !== -1) {
                c.skip.splice(s, 1);
            } else if (!on && s === -1) {
                c.skip.push(name);
            }
        }

        function itemList(items, kind) {
            var listed = kind === 'vm' ? c.vms : c.containers;
            var names = items.map(function (x) { return x.name; });
            listed.forEach(function (n) {
                if (names.indexOf(n) === -1) {
                    items.push({name: n, running: null, path: null});
                }
            });
            if (!items.length) {
                return h('div', {class: 'rb-hint', text: kind === 'vm' ? 'No VM has a disk in these folders.' : 'No container uses these folders.'});
            }
            return h('div', {class: 'rb-pathlist'}, items.map(function (x) {
                if (firstTime && c.skip.indexOf(x.name) === -1) {
                    setChoice(x.name, kind, true);
                }
                var box = h('input', {type: 'checkbox', checked: listed.indexOf(x.name) !== -1});
                box.addEventListener('change', function () { setChoice(x.name, kind, box.checked); });
                return h('label', {class: 'rb-pathlist-item', style: {cursor: 'pointer'}},
                         box, RB.icon(kind === 'vm' ? 'server' : 'archive'),
                         h('span', {style: {flex: 1, minWidth: 0}}, h('b', {text: x.name}),
                           x.path ? h('span', {class: 'rb-hint', text: '  ' + x.path}) : h('span', {class: 'rb-hint', text: '  no longer uses these folders'})),
                         x.running === null ? null : h('span', {class: 'rb-pill' + (x.running ? ' ok' : ''), text: x.running ? 'running' : 'stopped'}));
            }));
        }

        function broadList(items) {
            return h('div', {class: 'rb-pathlist'}, items.map(function (x) {
                var cb = h('input', {type: 'checkbox', checked: c.containers.indexOf(x.name) !== -1});
                cb.addEventListener('change', function () {
                    var i = c.containers.indexOf(x.name);
                    if (cb.checked && i === -1) {
                        c.containers.push(x.name);
                    } else if (!cb.checked && i !== -1) {
                        c.containers.splice(i, 1);
                    }
                });
                return h('label', {class: 'rb-pathlist-item', style: {cursor: 'pointer'}},
                         cb, RB.icon('archive'),
                         h('span', {style: {flex: 1, minWidth: 0}}, h('b', {text: x.name}), h('span', {class: 'rb-hint', text: '  ' + x.path})),
                         h('span', {class: 'rb-pill' + (x.running ? ' ok' : ''), text: x.running ? 'running' : 'stopped'}));
            }));
        }

        function datasetSummary(names) {
            return names.filter(function (n) {
                return !names.some(function (o) { return n.indexOf(o + '/') === 0; });
            }).map(function (top) {
                var inside = names.filter(function (n) { return n.indexOf(top + '/') === 0; }).length;
                return top + (inside ? ' (+' + inside + ' inside)' : '');
            }).join(', ');
        }

        function render(d) {
            RB.clear(box);
            var mode = c.mode;
            if (mode === 'zfs' || mode === 'zfs-stop') {
                var live = d.zfs.live || [];
                box.appendChild(d.zfs.ok
                    ? h('div', {class: 'rb-callout', style: {marginBottom: '14px'}}, RB.icon('check'),
                        h('div', null, 'Each backup snapshots ', h('b', {text: datasetSummary(d.zfs.datasets)}),
                          ' first and reads from the snapshot. The snapshot is removed afterwards.',
                          live.length ? h('div', {style: {marginTop: '6px'}}, 'Not on ZFS, so read as they are: ',
                              live.map(function (l) { return l.path + ' (' + l.why + ')'; }).join(', ') + '.') : null))
                    : h('div', {class: 'rb-callout warn', style: {marginBottom: '14px'}}, RB.icon('alert'), h('div', {text: d.zfs.error})));
            }
            if (mode === 'zfs-stop' || mode === 'stop') {
                box.appendChild(h('div', {class: 'rb-field-label', style: {margin: '4px 0 8px'},
                    text: mode === 'stop' ? 'Stopped while the backup runs' : 'Stopped for the snapshot'}));
                if (!d.docker) {
                    box.appendChild(h('div', {class: 'rb-hint', style: {marginBottom: '8px'}, text: 'Docker is not running, so containers could not be looked at.'}));
                }
                box.appendChild(itemList(d.containers.slice(), 'container'));
                if (d.broad && d.broad.length) {
                    box.appendChild(h('div', {class: 'rb-field-label', style: {margin: '14px 0 4px'},
                        text: 'Also have access, through a parent folder'}));
                    box.appendChild(h('div', {class: 'rb-hint', style: {marginBottom: '8px'},
                        text: 'File managers and the like, with all of /mnt or a whole share mounted. They keep running unless you tick them.'}));
                    box.appendChild(broadList(d.broad));
                }
                if (d.libvirt || c.vms.length) {
                    box.appendChild(h('div', {style: {height: '8px'}}));
                    box.appendChild(itemList(d.vms.slice(), 'vm'));
                }
                if (d.vms.length || c.vms.length) {
                    box.appendChild(h('div', {class: 'rb-fields', style: {marginTop: '12px'}},
                        RB.field('Wait for VMs to shut down', h('div', {class: 'rb-inline'},
                            RB.input(c, 'timeout', {type: 'number', min: 10, max: 3600}), 'seconds'),
                            {hint: 'If a VM is not off by then, the backup is skipped and everything is started again.'})));
                }
                box.appendChild(h('div', {class: 'rb-hint', style: {marginTop: '10px'},
                    text: 'Only what is running is stopped, and started again in Unraid\'s autostart order. Containers get as long ' +
                          'to stop as set in Settings > Docker, as when the array stops.'}));
            }
            firstTime = false;
            box.appendChild(h('div', {style: {marginTop: '10px'}},
                RB.button('Look again', {icon: 'refresh', small: true, kind: 'ghost', onclick: load})));
        }

        load();
        return box;
    };
})(window.RB);
