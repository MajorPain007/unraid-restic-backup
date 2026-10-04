(function (RB) {
    'use strict';
    var h = RB.h;

    var AUTO_RULES = {
        zfs: 'ZFS snapshot folders (.zfs)',
        plugin: 'Kept out by the plugin: ',
        caches: 'Cache folders (with a CACHEDIR.TAG)',
        otherfs: 'Other file systems',
        size: 'Files larger than '
    };

    function count(n) {
        return Number(n).toLocaleString();
    }

    function files(n) {
        return count(n) + (Number(n) === 1 ? ' file' : ' files');
    }

    RB.previewJob = function (job) {
        var body = h('div');
        var id = null;
        var timer = null;
        var finished = false;
        var dlg = RB.overlay({
            title: 'Preview' + (job.name ? ' of "' + job.name + '"' : ''), icon: 'eye', wide: true,
            body: body,
            actions: [h('div', {class: 'rb-spacer'}), RB.button('Close', {onclick: function () { dlg.close(); }})],
            onclose: function () {
                clearTimeout(timer);
                if (id && !finished) {
                    RB.api('job_preview_stop', {id: id}).catch(function () {});
                }
            }
        });

        var figure = h('b', {text: 'Starting...'});
        var current = h('div', {class: 'rb-current'});
        RB.clear(body).appendChild(h('div', null,
            h('p', {text: 'Going through the folders with the job\'s rules. Only names and sizes are read; nothing is backed up.'}),
            h('div', {class: 'rb-progress'},
              h('div', {class: 'rb-progress-track indeterminate'}, h('div', {class: 'rb-progress-bar'})),
              h('div', {class: 'rb-progress-text'}, h('span', null, figure),
                RB.button('Stop', {small: true, kind: 'ghost', onclick: function () {
                    if (id) {
                        RB.api('job_preview_stop', {id: id}).catch(RB.fail);
                    }
                }})),
              current)));

        function failed(message) {
            finished = true;
            RB.clear(body).appendChild(h('div', {class: 'rb-card-note err'}, RB.icon('alert'), h('div', {text: message})));
        }

        function poll() {
            RB.api('job_preview_status', {id: id}).then(function (s) {
                if (s.status === 'starting' || s.status === 'running') {
                    if (s.progress) {
                        figure.textContent = RB.bytes(s.progress.bytes) + ' in ' + count(s.progress.files) + ' files so far';
                        current.textContent = s.progress.current || '';
                    }
                    timer = setTimeout(poll, 1000);
                    return;
                }
                if (s.status === 'error') {
                    failed(s.error || 'The preview failed.');
                    return;
                }
                finished = true;
                RB.clear(body).appendChild(result(s.result));
            }).catch(function (e) {
                failed(e.message);
            });
        }

        RB.api('job_preview', {job: job}).then(function (r) {
            id = r.id;
            poll();
        }).catch(function (e) {
            failed(e.message);
        });
        return dlg;
    };

    function fact(label, value) {
        return h('div', {class: 'rb-fact'}, h('div', {class: 'rb-label', text: label}), h('div', {class: 'rb-value', text: value}));
    }

    function heading(text) {
        return h('h3', {class: 'rb-preview-head', text: text});
    }

    function result(r) {
        var box = h('div', {class: 'rb-preview'});
        if (r.cancelled) {
            box.appendChild(h('div', {class: 'rb-callout warn', style: {marginBottom: '14px'}}, RB.icon('alert'),
                h('div', {text: 'Stopped: the numbers cover only the part looked at so far.'})));
        }
        box.appendChild(h('div', {class: 'rb-facts four'},
            fact('Would be backed up', RB.bytes(r.total.bytes)),
            fact('Files', count(r.total.files)),
            fact('Folders', count(r.total.dirs)),
            fact('Looked through in', RB.duration(Math.max(1, Math.round(r.seconds))))));
        r.sources.forEach(function (s) {
            if (s.missing) {
                box.appendChild(h('div', {class: 'rb-card-note err', style: {marginTop: '12px'}}, RB.icon('alert'),
                    h('div', {text: s.path + ' does not exist.'})));
            }
        });

        box.appendChild(heading('Left out'));
        box.appendChild(rules(r.rules));

        var trees = r.sources.filter(function (s) { return s.tree && s.bytes > 0; });
        if (trees.length) {
            box.appendChild(heading('Where the space is'));
            trees.forEach(function (s) {
                box.appendChild(h('div', {class: 'rb-tree'}, tree(s.tree, s.bytes, 0, s.path)));
            });
        }

        if (r.largest.length) {
            box.appendChild(heading('Largest files'));
            box.appendChild(h('div', {class: 'rb-table-wrap'}, h('table', {class: 'rb-table'}, h('tbody', null,
                r.largest.map(function (f) {
                    return h('tr', null, h('td', {class: 'rb-mono', text: f.path, style: {wordBreak: 'break-all'}}),
                             h('td', {class: 'num', text: RB.bytes(f.bytes)}));
                })))));
        }

        if (r.error_count) {
            box.appendChild(h('div', {class: 'rb-callout warn', style: {marginTop: '14px'}}, RB.icon('alert'),
                h('div', null, RB.plural(r.error_count, 'folder') + ' could not be read; a backup would warn about them too.',
                  h('div', {class: 'rb-mono', style: {marginTop: '6px', fontSize: '12px'}}, r.errors.join('\n')))));
        }
        return box;
    }

    function rules(list) {
        var unmatched = false;
        var rows = list.map(function (rule) {
            var label;
            if (rule.kind === 'pattern' || rule.kind === 'ipattern') {
                label = h('span', null, h('code', {text: rule.label}),
                          rule.kind === 'ipattern' ? h('span', {class: 'rb-hint', text: '  ignoring case'}) : null);
            } else {
                label = h('span', {text: AUTO_RULES[rule.kind] + (rule.kind === 'size' || rule.kind === 'plugin' ? rule.label : '')});
            }
            var nothing = rule.files === 0 && rule.dirs === 0;
            unmatched = unmatched || (nothing && (rule.kind === 'pattern' || rule.kind === 'ipattern'));
            var what;
            if (nothing) {
                what = h('span', {class: 'dim', text: 'matches nothing'});
            } else if (!rule.measure) {
                what = h('span', {class: 'dim', text: RB.plural(rule.dirs, 'folder') + ', not counted'});
            } else {
                what = h('span', null, h('b', {text: RB.bytes(rule.bytes)}), '  ' + files(rule.files));
            }
            return h('tr', {title: rule.examples.length ? 'For example:\n' + rule.examples.join('\n') : ''},
                     h('td', null, label), h('td', {class: 'num'}, what));
        });
        if (!rows.length) {
            return h('div', {class: 'rb-hint', text: 'Nothing: the job has no exclude rules.'});
        }
        return h('div', null,
            h('div', {class: 'rb-table-wrap'}, h('table', {class: 'rb-table'}, h('tbody', null, rows))),
            unmatched ? h('div', {class: 'rb-hint', style: {marginTop: '6px'},
                text: 'A rule that matches nothing may be misspelled - or there is simply nothing of that kind.'}) : null);
    }

    var OUT_LABEL = {
        zfs: 'ZFS snapshots',
        plugin: 'the plugin\'s own files',
        caches: 'cache folder',
        otherfs: 'other file system',
        size: 'larger than '
    };

    function exact(path) {
        return path.replace(/[\\*?[\]]/g, '\\$&');
    }

    function outText(out) {
        if (out.kind === 'pattern' || out.kind === 'ipattern') {
            return out.label + (out.kind === 'ipattern' ? ' (any case)' : '');
        }
        return OUT_LABEL[out.kind] + (out.kind === 'size' ? out.label : '');
    }

    RB.ruleTree = function (job, opts) {
        opts = opts || {};
        var open = {};
        var gen = 0;
        var top = h('div', {class: 'rb-rtree-body'});
        var el = h('div', {class: 'rb-rtree'}, top);

        function change(fn) {
            fn();
            if (opts.onchange) {
                opts.onchange();
            }
            refresh();
        }

        function remove(list, pattern) {
            for (var i = list.length - 1; i >= 0; i--) {
                if (list[i] === pattern) {
                    list.splice(i, 1);
                }
            }
        }

        function action(e) {
            if (!opts.edit) {
                return null;
            }
            var p = exact(e.path);
            if (!e.out) {
                if (job.excludes.indexOf('!' + p) !== -1) {
                    return {title: 'Leave out again', run: function () { remove(job.excludes, '!' + p); }};
                }
                return {title: 'Leave out', run: function () { job.excludes.push(p); }};
            }
            if (e.out.via) {
                return null;
            }
            if (e.out.kind === 'pattern' && e.out.label === p) {
                return {title: 'Back up again', run: function () { remove(job.excludes, p); }};
            }
            if (e.out.kind === 'pattern') {
                return {title: 'Back up anyway, despite ' + e.out.label, run: function () { job.excludes.push('!' + p); }};
            }
            return null;
        }

        function mark(e) {
            var act = action(e);
            var cls = 'rb-rtree-mark ' + (e.out ? 'out' : 'in') + (e.out && e.out.via ? ' via' : '');
            var why = e.out ? (e.out.via ? 'Left out with ' + e.out.via + ' (' + outText(e.out) + ')' : 'Left out: ' + outText(e.out))
                            : (e.mount ? 'Backed up as an empty folder: another file system is mounted here' : 'Backed up');
            var icon = RB.icon(e.out ? 'x' : 'check');
            if (!act) {
                return h('span', {class: cls, title: why}, icon);
            }
            return h('button', {type: 'button', class: cls + ' act', title: why + '. Click: ' + act.title.toLowerCase() + '.',
                                'aria-label': act.title + ' ' + e.name, onclick: function (ev) {
                ev.stopPropagation();
                change(act.run);
            }}, icon);
        }

        function rowFor(e, depth, myGen) {
            var isOpen = !!open[e.path];
            var kids = h('div', {class: 'rb-tree-children'});
            var name = h('span', {class: 'rb-rtree-name', text: e.name, title: e.path});
            var row = h('div', {class: 'rb-rtree-row' + (e.open ? ' opens' : '') + (e.out ? ' out' : '') + (e.out && e.out.via ? ' via' : ''),
                                role: e.open ? 'button' : null, tabindex: e.open ? '0' : null,
                                'aria-expanded': e.open ? String(isOpen) : null},
                h('span', {class: 'rb-tree-name', style: {paddingLeft: (depth * 18) + 'px'}},
                  e.open ? RB.icon('right', 'rb-tree-caret') : h('span', {class: 'rb-tree-caret'}),
                  e.type === 'missing' ? h('span', {class: 'rb-rtree-mark out', title: 'Does not exist'}, RB.icon('alert')) : mark(e),
                  RB.icon(e.type === 'dir' ? 'folder' : (e.type === 'symlink' ? 'link' : 'file'), e.type === 'dir' ? 'dir' : null),
                  name),
                h('span', {class: 'rb-rtree-rule'},
                  e.type === 'missing' ? h('span', {class: 'rb-rtree-error', text: 'does not exist'})
                  : (e.out && !e.out.via ? (e.out.kind === 'pattern' && e.out.label === exact(e.path)
                        ? h('span', {text: 'by its path', title: e.out.label})
                        : h(/pattern$/.test(e.out.kind) ? 'code' : 'span', {text: outText(e.out), title: outText(e.out)}))
                     : (e.mount ? h('span', {text: 'mount point, kept empty'}) : null))),
                h('span', {class: 'num', text: e.size !== null ? RB.bytes(e.size) : ''}));
            var node = h('div', {class: 'rb-tree-node' + (isOpen ? ' open' : '')}, row, kids);
            if (e.open) {
                var flip = function () {
                    if (open[e.path]) {
                        delete open[e.path];
                        node.classList.remove('open');
                    } else {
                        open[e.path] = true;
                        node.classList.add('open');
                        fill(kids, e.path, depth + 1, gen);
                    }
                    row.setAttribute('aria-expanded', String(!!open[e.path]));
                };
                row.addEventListener('click', flip);
                row.addEventListener('keydown', function (ev) {
                    if ((ev.key === 'Enter' || ev.key === ' ') && ev.target === row) {
                        ev.preventDefault();
                        flip();
                    }
                });
                if (isOpen) {
                    fill(kids, e.path, depth + 1, myGen);
                }
            }
            return node;
        }

        function fill(box, path, depth, myGen) {
            if (!box.firstChild) {
                box.appendChild(h('div', {class: 'rb-hint rb-rtree-wait', style: {paddingLeft: (depth * 18 + 30) + 'px'}},
                                  RB.icon('loader', 'rb-spin'), ' Reading...'));
            }
            RB.api('job_tree', {job: job, path: path}).then(function (r) {
                if (myGen !== gen) {
                    return;
                }
                RB.clear(box);
                if (!r.entries.length) {
                    box.appendChild(h('div', {class: 'rb-hint', style: {padding: '4px 8px 4px ' + (depth * 18 + 30) + 'px'},
                                              text: path === '' ? 'No folders yet.' : 'Empty.'}));
                }
                r.entries.forEach(function (e) {
                    box.appendChild(rowFor(e, depth, myGen));
                });
                if (r.more) {
                    box.appendChild(h('div', {class: 'rb-hint', style: {padding: '4px 8px 4px ' + (depth * 18 + 30) + 'px'},
                        text: 'and ' + RB.plural(r.more, 'more entry', 'more entries') + (r.more_out ? ', ' + r.more_out + ' of them left out' : '')}));
                }
            }).catch(function (err) {
                if (myGen !== gen) {
                    return;
                }
                RB.clear(box).appendChild(h('div', {class: 'rb-hint rb-rtree-error', style: {padding: '4px 8px 4px ' + (depth * 18 + 30) + 'px'},
                                                    text: err.message}));
            });
        }

        function refresh() {
            gen++;
            fill(top, '', 0, gen);
        }

        refresh();
        return {el: el, refresh: refresh};
    };

    RB.whatGetsBackedUp = function (job) {
        var tree = RB.ruleTree(job);
        var dlg = RB.overlay({
            title: 'What "' + job.name + '" backs up', icon: 'eye', wide: true,
            body: h('div', null,
                h('p', {class: 'rb-hint', style: {marginTop: 0}},
                  'Open the folders to see what the backup takes and what its rules leave out. To change the rules, edit the job.'),
                tree.el),
            actions: [RB.button('Measure sizes...', {icon: 'activity', onclick: function () { RB.previewJob(job); }}),
                      h('div', {class: 'rb-spacer'}),
                      RB.button('Close', {onclick: function () { dlg.close(); }})]
        });
        return dlg;
    };

    function tree(node, total, depth, title) {
        var kids = node.children || [];
        var open = kids.length > 0 || node.more;
        var share = total > 0 ? Math.max(0.5, node.bytes / total * 100) : 0;
        var row = h(open ? 'button' : 'div', {type: open ? 'button' : null, class: 'rb-tree-row', title: title || null},
            h('span', {class: 'rb-tree-name', style: {paddingLeft: (depth * 18) + 'px'}},
              open ? RB.icon('right', 'rb-tree-caret') : h('span', {class: 'rb-tree-caret'}),
              RB.icon('folder', 'dir'), h('span', {text: node.name})),
            h('span', {class: 'rb-tree-bar'}, h('i', {style: {width: share + '%'}})),
            h('span', {class: 'num', text: RB.bytes(node.bytes)}),
            h('span', {class: 'num files', text: files(node.files)}));
        var el = h('div', {class: 'rb-tree-node'}, row);
        if (open) {
            var children = h('div', {class: 'rb-tree-children'});
            var built = false;
            row.addEventListener('click', function () {
                if (!built) {
                    built = true;
                    kids.forEach(function (k) {
                        children.appendChild(tree(k, total, depth + 1));
                    });
                    if (node.more) {
                        children.appendChild(h('div', {class: 'rb-tree-row more'},
                            h('span', {class: 'rb-tree-name dim', style: {paddingLeft: ((depth + 1) * 18 + 20) + 'px'},
                              text: RB.plural(node.more.count, 'more folder')}),
                            h('span'), h('span', {class: 'num', text: RB.bytes(node.more.bytes)}), h('span')));
                    }
                }
                el.classList.toggle('open');
            });
            el.appendChild(children);
            if (depth === 0) {
                row.click();
            }
        }
        return el;
    }
})(window.RB);
