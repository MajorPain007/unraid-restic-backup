(function (RB) {
    'use strict';
    var h = RB.h;
    var TYPE_LABEL = {sftp: 'SFTP', local: 'Local folder', rest: 'REST server'};

    function o() {
        return RB.state.overview;
    }

    RB.views.repos = {
        render: function (container) {
            var repos = o().config.repos;
            if (!repos.length) {
                container.appendChild(h('div', {class: 'rb-empty'}, RB.icon('repo', 'big'),
                    h('h3', {text: 'No repository yet'}),
                    h('p', {text: 'A repository is where restic keeps the backups: a folder on another disk, ' +
                               'a second server reached over SSH, or a restic REST server.'}),
                    RB.button('Add repository', {icon: 'plus', kind: 'primary', onclick: function () { RB.editRepo(null); }})));
                return;
            }
            var grid = h('div', {class: 'rb-grid'});
            repos.forEach(function (cfg) {
                grid.appendChild(repoCard(cfg, RB.repoInfo(cfg.id) || {}));
            });
            grid.appendChild(h('button', {type: 'button', class: 'rb-card add', onclick: function () { RB.editRepo(null); }},
                               RB.icon('plus'), 'Add repository'));
            container.appendChild(grid);
        }
    };

    function repoCard(cfg, info) {
        var stats = info.stats;
        var jobs = o().config.jobs.filter(function (j) { return j.repo === cfg.id; });
        var card = h('div', {class: 'rb-card' + (info.running ? ' is-running' : '')});
        card.appendChild(h('div', {class: 'rb-card-head'},
            h('span', {class: 'rb-tile'}, RB.icon(RB.repoIcon(cfg.type))),
            h('div', {class: 'rb-card-title'},
              h('h3', {text: cfg.name, title: cfg.name}),
              h('div', {class: 'rb-meta', text: TYPE_LABEL[cfg.type]}),
              h('div', {class: 'rb-meta rb-mono', text: info.url, title: info.url,
                        style: {display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: '2px'}})),
            h('div', {class: 'rb-card-actions'},
              RB.button('Snapshots', {icon: 'layers', small: true, onclick: function () { RB.show('snapshots', {repo: cfg.id}); }}),
              RB.button('', {icon: 'more', small: true, kind: 'ghost', title: 'More',
                             onclick: function (e) { repoMenu(cfg, info, e.currentTarget); }}))));

        if (info.running) {
            var op = info.running;
            card.appendChild(h('div', {class: 'rb-meta', style: {marginTop: '14px'}}, RB.icon('loader', 'rb-spin'),
                op.kind === 'backup' ? 'Backup "' + op.job_name + '"' : opLabel(op.kind)));
            card.appendChild(RB.progressBlock(op));
        }

        var facts = h('div', {class: 'rb-facts'});
        facts.appendChild(fact('Size', stats ? h('span', null, RB.bytes(stats.total_size),
            stats.compression_space_saving ? h('small', {text: ' · ' + Math.round(stats.compression_space_saving) + '% saved by compression'}) : null)
            : h('span', {class: 'rb-dim', text: 'Not measured yet'})));
        facts.appendChild(fact('Snapshots', stats ? String(stats.snapshots_count) : '-'));
        var check = info.check || {};
        var prune = info.prune || {};
        facts.appendChild(fact('Last check', check.last
            ? h('span', {class: check.last.status === 'error' ? 'rb-err-text' : null, title: RB.dateTime(check.last.ended)},
                check.last.status === 'success' ? 'Passed ' + RB.relative(check.last.ended)
                    : (check.last.status === 'error' ? 'Failed ' + RB.relative(check.last.ended) : RB.relative(check.last.ended)))
            : (cfg.maintenance.check.enabled ? 'Not yet' : 'Off')));
        var nextMaint = [];
        if (cfg.maintenance.prune.enabled && !cfg.append_only && prune.next) {
            nextMaint.push(['Prune', prune.next]);
        }
        if (cfg.maintenance.check.enabled && check.next) {
            nextMaint.push(['Check', check.next]);
        }
        nextMaint.sort(function (a, b) { return a[1] - b[1]; });
        facts.appendChild(fact('Next maintenance', nextMaint.length
            ? h('span', {title: RB.dateTime(nextMaint[0][1])}, nextMaint[0][0] + ' ' + RB.relative(nextMaint[0][1])) : 'None scheduled'));
        card.appendChild(facts);

        if (!cfg.has_password) {
            card.appendChild(note('err', 'key', 'No password is stored for this repository.'));
        }
        if (check.last && check.last.status === 'error' && check.last.error) {
            card.appendChild(note('err', 'alert', h('span', null, check.last.error, ' ',
                h('span', {class: 'rb-link', text: 'Show log', onclick: function () { RB.showLog(check.last.id); }}))));
        }
        if (prune.last && prune.last.status === 'error' && prune.last.error) {
            card.appendChild(note('warn', 'alert', h('span', null, 'Prune: ' + prune.last.error, ' ',
                h('span', {class: 'rb-link', text: 'Show log', onclick: function () { RB.showLog(prune.last.id); }}))));
        }
        if (cfg.append_only) {
            card.appendChild(note('info', 'lock', 'Append-only: old snapshots are removed on the server, not from here.'));
        }
        card.appendChild(h('div', {class: 'rb-card-foot'}, h('span', {class: 'rb-meta'},
            RB.icon('archive'), jobs.length ? 'Used by ' + jobs.map(function (j) { return j.name; }).join(', ') : 'No job uses it yet')));
        return card;
    }

    function note(kind, iconName, content) {
        return h('div', {class: 'rb-card-note ' + kind}, RB.icon(iconName), h('div', null, content));
    }

    function fact(label, value) {
        return h('div', {class: 'rb-fact'}, h('div', {class: 'rb-label', text: label}), h('div', {class: 'rb-value'}, value));
    }

    function opLabel(kind) {
        return {prune: 'Removing unused data', check: 'Checking the repository', restore: 'Restoring',
                forget: 'Applying retention'}[kind] || kind;
    }

    function repoMenu(cfg, info, anchor) {
        var busy = !!info.running;
        RB.menu(anchor, [
            {label: 'Edit', icon: 'edit', onclick: function () { RB.editRepo(cfg.id); }},
            {label: 'Test connection', icon: 'refresh', onclick: function () { connect(cfg); }},
            cfg.type === 'sftp' ? {label: 'SSH key', icon: 'key', onclick: function () { sshKeyDialog(cfg, false); }} : null,
            {label: 'Show password', icon: 'eye', onclick: function () { showPassword(cfg); }},
            '-',
            {label: 'Check now', icon: 'shield', disabled: busy, onclick: function () { runMaintenance(cfg, 'check'); }},
            {label: 'Prune now', icon: 'trash', disabled: busy || cfg.append_only, onclick: function () { runMaintenance(cfg, 'prune'); }},
            {label: 'Measure size', icon: 'repo', onclick: function () {
                RB.toast('Measuring "' + cfg.name + '"...', 'info');
                RB.api('repo_stats', {id: cfg.id}).then(function () {
                    RB.toast('"' + cfg.name + '" measured.');
                    RB.refresh();
                }).catch(RB.fail);
            }},
            {label: 'Size history', icon: 'activity', onclick: function () { RB.sizeHistory({repo: cfg.id}); }},
            {label: 'Keys', icon: 'key', onclick: function () { keysDialog(cfg); }},
            {label: 'Remove stale locks', icon: 'unlock', disabled: busy, onclick: function () { unlock(cfg); }},
            '-',
            {label: 'Delete', icon: 'trash', danger: true, onclick: function () { deleteRepo(cfg); }}
        ]);
    }

    function runMaintenance(cfg, kind) {
        RB.api('repo_run', {id: cfg.id, kind: kind}).then(function () {
            RB.toast((kind === 'check' ? 'Check' : 'Prune') + ' of "' + cfg.name + '" started.');
            setTimeout(RB.refresh, 700);
        }).catch(RB.fail);
    }

    function unlock(cfg) {
        RB.confirm({title: 'Remove stale locks?',
                    text: 'Removes locks left behind by restic processes that no longer run - after a crash or a power cut. ' +
                          'Locks of running operations, on this or another computer, stay.',
                    confirm: 'Remove stale locks'}).then(function (yes) {
            if (yes) {
                RB.api('repo_unlock', {id: cfg.id}).then(function () { RB.toast('Stale locks removed.'); }).catch(RB.fail);
            }
        });
    }

    function deleteRepo(cfg) {
        return RB.confirm({title: 'Remove "' + cfg.name + '" from the plugin?',
                           text: h('div', null, h('p', {text: 'The plugin forgets this repository, its password and its SSH key.'}),
                                   h('p', null, h('b', {text: 'The backups themselves are not deleted'}),
                                     ' - they stay at ' + RB.repoInfo(cfg.id).url + '. Without the password, nobody can read them; ' +
                                     'keep it if you may need them.')),
                           confirm: 'Remove', danger: true}).then(function (yes) {
            if (!yes) {
                return false;
            }
            return RB.api('repo_delete', {id: cfg.id}).then(function () {
                RB.toast('Repository removed from the plugin.');
                RB.refresh();
                return true;
            }).catch(function (e) {
                RB.fail(e);
                return false;
            });
        });
    }

    function saveText(name, text) {
        var url = URL.createObjectURL(new Blob([text], {type: 'text/plain'}));
        var a = h('a', {href: url, download: name, style: {display: 'none'}});
        document.body.appendChild(a);
        a.click();
        setTimeout(function () { URL.revokeObjectURL(url); a.remove(); }, 1000);
    }

    function recoveryText(cfg, password) {
        var url = RB.repoInfo(cfg.id) ? RB.repoInfo(cfg.id).url : '';
        return [
            'restic repository: ' + cfg.name,
            'Location: ' + url,
            'Password: ' + password,
            '',
            'Keep this file somewhere safe and away from the server: in a password manager,',
            'or printed. Without the password the backups cannot be read - by anyone.',
            '',
            'To get at the backups without the plugin, on any computer with restic:',
            '  restic -r ' + (cfg.type === 'sftp' ? 'sftp:' + cfg.sftp.user + '@' + cfg.sftp.host + ':' + cfg.sftp.path +
                ' -o sftp.args="-p ' + cfg.sftp.port + '"' : url) + ' snapshots',
            ''
        ].join('\n');
    }

    function passwordDialog(cfg, password, firstTime) {
        return new Promise(function (resolve) {
            var stored = {ok: !firstTime};
            var cont;
            var dlg = RB.overlay({
                title: firstTime ? 'Store this password' : 'Password of "' + cfg.name + '"', icon: 'key', sticky: firstTime,
                body: [
                    firstTime ? h('div', {class: 'rb-callout warn', style: {marginBottom: '14px'}}, RB.icon('alert'),
                        h('div', null, h('b', {text: 'Without this password the backups cannot be restored. '}),
                          'The plugin keeps a copy on the flash drive, but if the server is lost, that copy is lost with it. ' +
                          'Store it in your password manager, or download the recovery file and keep it elsewhere.')) : null,
                    h('div', {class: 'rb-secret'}, h('code', {text: password}),
                      RB.button('', {icon: 'copy', small: true, title: 'Copy', onclick: function () { RB.copyText(password); }})),
                    h('div', {class: 'rb-inline', style: {marginTop: '12px'}},
                      RB.button('Download recovery file', {icon: 'download', small: true, onclick: function () {
                          saveText('restic-' + cfg.name.replace(/[^A-Za-z0-9._-]+/g, '-') + '-recovery.txt', recoveryText(cfg, password));
                      }})),
                    firstTime ? h('div', {style: {marginTop: '16px'}}, RB.toggle(stored, 'ok', 'I have stored the password somewhere safe',
                        function (v) { cont.disabled = !v; })) : null
                ],
                actions: [h('div', {class: 'rb-spacer'}), cont = RB.button(firstTime ? 'Continue' : 'Close', {kind: 'primary',
                    disabled: firstTime, onclick: function () { dlg.close(true); }})],
                beforeClose: function (r) { return !firstTime || r === true; },
                onclose: function () { resolve(); }
            });
        });
    }

    function showPassword(cfg) {
        RB.api('repo_password', {id: cfg.id}).then(function (r) {
            passwordDialog(cfg, r.password, false);
        }).catch(RB.fail);
    }

    var SERVER_KINDS = {
        ssh: {label: 'SSH server: Unraid, NAS, Linux, VPS', port: 22,
            steps: function (cfg, key) {
                return [
                    h('span', null, 'Add the key to the user on the server, for example from the Unraid terminal:',
                      cmdBox('ssh-copy-id -p ' + (cfg.sftp.port || 22) + ' -i ' + keyPath(cfg) + ' ' + (cfg.sftp.user || 'user') + '@' + (cfg.sftp.host || 'server'))),
                    'Another Unraid server takes it under Users, root, "SSH authorized keys" too. A Synology or QNAP needs SFTP switched on first ' +
                    '(Control Panel > File Services), and its paths start at the shared folder: /backups/' + o().system.server + ', without /volume1.',
                    'A relative path starts in the user\'s home folder.'
                ];
            }},
        storagebox: {label: 'Hetzner Storage Box', port: 23,
            steps: function (cfg, key) {
                return [
                    'Enable SSH support for the Storage Box (or sub-account) in the Hetzner console.',
                    h('span', null, 'In the Unraid terminal, run this once and enter the Storage Box password when asked:',
                      cmdBox('cat ' + keyPath(cfg) + ' | ssh -p 23 ' + (cfg.sftp.user || 'uXXXXXX') + '@' +
                             (cfg.sftp.host || 'uXXXXXX.your-storagebox.de') + ' install-ssh-key')),
                    'Use port 23, and a relative path such as backups/' + o().system.server + ' - it is relative to the home folder.'
                ];
            }}
    };

    function serverKind(cfg) {
        return /\.your-storagebox\.de$/i.test(cfg.sftp.host || '') || +cfg.sftp.port === 23 ? 'storagebox' : 'ssh';
    }

    function keyPath(cfg) {
        return '/boot/config/plugins/restic.backup/ssh/' + cfg.id + '.pub';
    }

    function cmdBox(text) {
        return h('div', {class: 'rb-secret', style: {marginTop: '6px'}}, h('code', {text: text}),
                 RB.button('', {icon: 'copy', small: true, title: 'Copy', onclick: function () { RB.copyText(text); }}));
    }

    function sshKeyDialog(cfg, setup) {
        return new Promise(function (resolve) {
            var kind = serverKind(cfg);
            var key = cfg.ssh_public_key || '';
            var steps = h('ol', {style: {margin: '6px 0 0', paddingLeft: '20px', display: 'grid', gap: '8px'}});
            function renderSteps() {
                RB.clear(steps);
                SERVER_KINDS[kind].steps(cfg, key).forEach(function (s) { steps.appendChild(h('li', null, s)); });
            }
            renderSteps();
            var kindSel = h('select', {style: {width: 'auto'}}, Object.keys(SERVER_KINDS).map(function (k) {
                return h('option', {value: k, selected: k === kind}, SERVER_KINDS[k].label);
            }));
            kindSel.addEventListener('change', function () {
                kind = kindSel.value;
                renderSteps();
            });
            var go = false;
            var dlg = RB.overlay({
                title: 'Let the plugin log in to ' + (cfg.sftp.host || 'the server'), icon: 'key', wide: true,
                body: [
                    h('p', {text: 'The plugin made its own SSH key for this repository. The server has to accept it - add the public key below to the user ' +
                                  cfg.sftp.user + ' on ' + cfg.sftp.host + '. No password is stored anywhere.'}),
                    RB.field('Public key', h('div', {class: 'rb-secret'}, h('code', {text: key}),
                        RB.button('', {icon: 'copy', small: true, title: 'Copy', onclick: function () { RB.copyText(key); }}))),
                    h('div', {class: 'rb-inline', style: {margin: '16px 0 4px'}}, h('b', {text: 'The server is'}), kindSel),
                    steps
                ],
                actions: [
                    RB.button('New key', {icon: 'refresh', kind: 'ghost', small: true, onclick: function (e) {
                        RB.confirm({title: 'Make a new key?', text: 'The old key stops working; the new one has to be added to the server again.',
                                    confirm: 'New key', danger: true}).then(function (yes) {
                            if (yes) {
                                RB.api('repo_keygen', {id: cfg.id}).then(function (r) {
                                    cfg.ssh_public_key = r.public_key;
                                    dlg.close();
                                    RB.refresh();
                                    sshKeyDialog(cfg, setup).then(resolve);
                                }).catch(RB.fail);
                            }
                        });
                    }}),
                    h('div', {class: 'rb-spacer'}),
                    RB.button(setup ? 'Later' : 'Close', {onclick: function () { dlg.close(); }}),
                    RB.button('Test connection', {kind: 'primary', icon: 'refresh', onclick: function () { go = true; dlg.close(); }})
                ],
                onclose: function () { resolve(go); }
            });
        });
    }

    function hostKeyDialog(cfg, hk) {
        return new Promise(function (resolve) {
            var changed = hk.status === 'changed';
            var trusted = false;
            var dlg = RB.overlay({
                title: changed ? 'The server\'s key has changed' : 'Is this the right server?', icon: changed ? 'alert' : 'shield',
                body: [
                    changed ? h('div', {class: 'rb-callout warn', style: {marginBottom: '14px'}}, RB.icon('alert'),
                        h('div', null, 'The server now shows a different key than the one confirmed before. That is expected after it was ' +
                          'reinstalled. Otherwise someone may be between this server and that one - do not accept it then.'))
                        : h('p', {text: 'This is the first connection to ' + cfg.sftp.host + '. It identifies itself with the key below. ' +
                                        'If you can, compare it with the server\'s own (ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub there).'}),
                    h('div', {class: 'rb-table-wrap'}, h('table', {class: 'rb-table'}, h('tbody', null, hk.scanned.map(function (k) {
                        return h('tr', null, h('td', {class: 'shrink', text: k.type}), h('td', {class: 'rb-mono', text: k.fingerprint}));
                    })))),
                    changed && hk.stored.length ? h('div', {class: 'rb-hint', style: {marginTop: '10px'}},
                        'Confirmed before: ' + hk.stored.map(function (k) { return k.type + ' ' + k.fingerprint; }).join(', ')) : null
                ],
                actions: [h('div', {class: 'rb-spacer'}),
                          RB.button('Cancel', {onclick: function () { dlg.close(); }}),
                          RB.button(changed ? 'Accept the new key' : 'Yes, trust it', {kind: changed ? 'danger solid' : 'primary',
                              onclick: function (e) {
                                  RB.busy(e.currentTarget, RB.api('repo_hostkey_trust', {id: cfg.id,
                                      fingerprints: hk.scanned.map(function (k) { return k.fingerprint; })})).then(function () {
                                      trusted = true;
                                      dlg.close();
                                  }).catch(RB.fail);
                              }})],
                onclose: function () { resolve(trusted); }
            });
        });
    }

    function connect(cfg) {
        RB.toast('Connecting to "' + cfg.name + '"...', 'info');
        return RB.api('repo_test', {id: cfg.id}).then(function (r) {
            if (r.status === 'hostkey') {
                return hostKeyDialog(cfg, r.hostkey).then(function (ok) { return ok ? connect(cfg) : null; });
            }
            if (r.status === 'empty') {
                return RB.confirm({title: 'Nothing there yet',
                                   text: 'There is no restic repository at ' + RB.repoInfo(cfg.id).url + '. Create one there now?',
                                   confirm: 'Initialize repository'}).then(function (yes) {
                    if (!yes) {
                        return null;
                    }
                    return RB.api('repo_init', {id: cfg.id}).then(function () {
                        RB.toast('Repository "' + cfg.name + '" is ready.');
                        RB.refresh();
                    });
                });
            }
            RB.toast('Connected: "' + cfg.name + '" opens with its password.');
            RB.api('repo_stats', {id: cfg.id}).then(RB.refresh).catch(function () {});
            return null;
        }).catch(function (e) {
            var dlg = RB.overlay({title: 'Could not open "' + cfg.name + '"', icon: 'alert',
                        body: [h('p', {text: e.message}),
                               cfg.type === 'sftp' ? h('p', {class: 'rb-hint', text: 'Check host, port, user and path, and that the server accepts the plugin\'s SSH key.'}) : null],
                        actions: [cfg.type === 'sftp' ? RB.button('Show SSH key', {icon: 'key', onclick: function () {
                                      dlg.close();
                                      sshKeyDialog(cfg, true).then(function (go) { if (go) { connect(cfg); } });
                                  }}) : null,
                                  h('div', {class: 'rb-spacer'}),
                                  RB.button('Edit repository', {icon: 'edit', onclick: function () {
                                      dlg.close();
                                      RB.editRepo(cfg.id);
                                  }})]});
        });
    }

    RB.connectRepo = connect;

    function keysDialog(cfg) {
        RB.api('repo_keys', {id: cfg.id}).then(function (r) {
            RB.overlay({title: 'Keys of "' + cfg.name + '"', icon: 'key', wide: true,
                body: [h('p', {class: 'rb-hint', text: 'Each key is a password that opens the repository. The plugin uses the one marked current.'}),
                       h('div', {class: 'rb-table-wrap'}, h('table', {class: 'rb-table'},
                         h('thead', null, h('tr', null, h('th', {text: 'ID'}), h('th', {text: 'Made by'}), h('th', {text: 'Created'}), h('th'))),
                         h('tbody', null, r.keys.map(function (k) {
                             return h('tr', null, h('td', {class: 'rb-mono', text: String(k.id).slice(0, 8)}),
                                      h('td', {text: (k.userName || '?') + '@' + (k.hostName || '?')}),
                                      h('td', {class: 'dim', text: k.created}),
                                      h('td', null, k.current ? h('span', {class: 'rb-pill accent', text: 'current'}) : null));
                         }))))]});
        }).catch(RB.fail);
    }

    function blankRepo() {
        return {
            id: '', name: '', type: 'sftp',
            sftp: {user: '', host: '', port: 22, path: 'backups/' + o().system.server},
            local: {path: ''},
            rest: {url: '', user: '', cacert: ''},
            options: {compression: 'auto', limit_upload: 0, limit_download: 0, connections: 0, extra_flags: []},
            maintenance: {
                prune: {enabled: true, schedule: {mode: 'cron', cron: '0 4 * * 0', every: 24, unit: 'hours', catch_up: true}, max_unused: '10%'},
                check: {enabled: true, schedule: {mode: 'cron', cron: '0 5 1 * *', every: 24, unit: 'hours', catch_up: true},
                        read_data: 'subset', subset: '5%'}
            },
            unlock_stale: true, append_only: false
        };
    }

    RB.editRepo = function (id) {
        var saved = id ? RB.repoConfig(id) : null;
        var repo = saved ? JSON.parse(JSON.stringify(saved)) : blankRepo();
        var isNew = !saved;
        var pw = {mode: isNew ? 'generate' : 'keep', value: '', change: false};
        var rest = {mode: 'keep', value: ''};
        var errorSlot = h('div');
        var typeSlot = h('div');
        var dlg;

        function section(title, iconName, content, hint) {
            return h('div', {class: 'rb-form-section'}, h('h3', null, RB.icon(iconName), title),
                     hint ? h('div', {class: 'rb-hint', style: {marginBottom: '14px'}}, hint) : null, content);
        }

        function renderType() {
            RB.clear(typeSlot);
            if (repo.type === 'sftp') {
                var kindSel = h('select', null, Object.keys(SERVER_KINDS).map(function (k) {
                    return h('option', {value: k, selected: k === serverKind(repo)}, SERVER_KINDS[k].label);
                }));
                var port = RB.input(repo.sftp, 'port', {type: 'number', min: 1, max: 65535});
                var host = RB.input(repo.sftp, 'host', {placeholder: 'backup.example.com or 192.168.1.20'});
                var setKind = function (k) {
                    kindSel.value = k;
                    repo.sftp.port = SERVER_KINDS[k].port;
                    port.value = repo.sftp.port;
                };
                kindSel.addEventListener('change', function () { setKind(kindSel.value); });
                host.addEventListener('input', function () {
                    if (/\.your-storagebox\.de$/i.test(host.value.trim()) && kindSel.value !== 'storagebox') {
                        setKind('storagebox');
                    }
                });
                typeSlot.appendChild(h('div', {class: 'rb-fields'},
                    RB.field('The server is', kindSel, {hint: 'Sets the usual port. Once saved, the plugin shows its SSH key and how to add it on the server.'}),
                    RB.field('Host', host),
                    RB.field('Port', port),
                    RB.field('User', RB.input(repo.sftp, 'user', {placeholder: 'root, or uXXXXXX for a Storage Box'})),
                    RB.field('Path on the server', RB.input(repo.sftp, 'path', {mono: true}), {wide: true,
                        hint: 'Relative paths start in the user\'s home folder. Created by "Initialize" if it does not exist.'})));
            } else if (repo.type === 'local') {
                var p = RB.input(repo.local, 'path', {mono: true, placeholder: '/mnt/disks/backup/restic'});
                typeSlot.appendChild(h('div', {class: 'rb-fields'}, RB.field('Folder', h('div', {class: 'rb-input-group'}, p,
                    RB.button('Browse...', {icon: 'folder', onclick: function () {
                        RB.pickPath({title: 'Repository folder', create: true, start: repo.local.path || '/mnt'}).then(function (path) {
                            if (path) {
                                repo.local.path = path;
                                p.value = path;
                            }
                        });
                    }})), {wide: true, hint: 'Best on a disk that is not part of what you back up: an unassigned device, or another pool.'})));
            } else {
                var restPw = h('input', {type: 'password', placeholder: saved && saved.has_rest_password ? 'unchanged' : '', autocomplete: 'new-password'});
                restPw.addEventListener('input', function () { rest.mode = 'set'; rest.value = restPw.value; });
                typeSlot.appendChild(h('div', {class: 'rb-fields'},
                    RB.field('Server URL', RB.input(repo.rest, 'url', {mono: true, placeholder: 'https://backup.lan:8000/' + o().system.server}), {wide: true}),
                    RB.field('User', RB.input(repo.rest, 'user'), {optional: true}),
                    RB.field('Password', restPw, {optional: true}),
                    RB.field('CA certificate', RB.input(repo.rest, 'cacert', {mono: true, placeholder: '/boot/config/plugins/restic.backup/ca.pem'}),
                             {optional: true, wide: true, hint: 'For a server with a certificate of its own making.'})));
            }
        }
        renderType();

        var pwInput = h('input', {type: 'password', autocomplete: 'new-password', placeholder: 'The repository password'});
        pwInput.addEventListener('input', function () { pw.value = pwInput.value; });
        var pwSlot = h('div', {style: {marginTop: '12px'}});
        function renderPw() {
            RB.clear(pwSlot);
            if (pw.mode === 'set') {
                pwSlot.appendChild(RB.field(isNew ? 'Password of the existing repository' : 'New stored password', pwInput));
            } else if (pw.mode === 'generate') {
                pwSlot.appendChild(h('div', {class: 'rb-hint', text: 'A random 32-character password is made and shown once you save, for you to keep.'}));
            }
        }
        var password = isNew
            ? h('div', null, RB.segmented(pw.mode, [['generate', 'Make a new password'], ['set', 'Enter an existing one']], function (v) {
                pw.mode = v;
                renderPw();
            }), pwSlot)
            : h('div', null, RB.toggle(pw, 'change', 'Change the stored password', function (on) {
                pw.mode = on ? 'set' : 'keep';
                renderPw();
            }), h('div', {class: 'rb-hint', style: {marginTop: '6px'}},
                'This changes only what the plugin uses to open the repository. To change the repository\'s own password, use restic key passwd.'), pwSlot);
        renderPw();

        var m = repo.maintenance;
        var pruneDetail = h('div', {style: {marginTop: '12px'}},
            RB.scheduleEditor(m.prune.schedule, {allowManual: false, noCatchUp: false}),
            h('div', {class: 'rb-fields', style: {marginTop: '12px'}},
              RB.field('Leave unused at most', RB.input(m.prune, 'max_unused', {placeholder: '10%'}),
                       {hint: 'Repacking costs time and traffic; a little unused space saves both.'})));
        var checkReadSel = RB.select(m.check, 'read_data', [['none', 'Structure only'], ['subset', 'Structure and part of the data'],
                                                             ['all', 'Structure and all data']]);
        var checkDetail = h('div', {style: {marginTop: '12px'}},
            RB.scheduleEditor(m.check.schedule, {allowManual: false}),
            h('div', {class: 'rb-fields', style: {marginTop: '12px'}},
              RB.field('Read', checkReadSel, {hint: 'Reading data downloads it - mind the traffic for remote repositories.'}),
              RB.field('Share of data', RB.input(m.check, 'subset', {placeholder: '5%'}), {hint: 'A different part each time.'})));
        var maintenance = h('div', null,
            RB.toggle(m.prune, 'enabled', 'Remove unused data (prune)', function (v) { pruneDetail.style.display = v ? '' : 'none'; }),
            h('div', {class: 'rb-hint', style: {margin: '4px 0 0 44px'}}, 'Retention only drops snapshots; prune frees the space they used.'),
            pruneDetail,
            h('div', {style: {height: '18px'}}),
            RB.toggle(m.check, 'enabled', 'Check the repository', function (v) { checkDetail.style.display = v ? '' : 'none'; }),
            checkDetail);
        pruneDetail.style.display = m.prune.enabled ? '' : 'none';
        checkDetail.style.display = m.check.enabled ? '' : 'none';

        var opt = repo.options;
        var flags = h('textarea', {rows: 2, spellcheck: 'false', placeholder: '--pack-size=64'});
        flags.value = opt.extra_flags.join('\n');
        flags.addEventListener('input', function () {
            opt.extra_flags = flags.value.split('\n').map(function (s) { return s.trim(); }).filter(Boolean);
        });
        var advanced = h('details', {class: 'rb-more'}, h('summary', {text: 'Advanced'}),
            h('div', {class: 'rb-fields'},
              RB.field('Compression', RB.select(opt, 'compression', [['auto', 'Auto (recommended)'], ['fastest', 'Fastest'],
                                                                      ['better', 'Better'], ['max', 'Maximum'], ['off', 'Off']])),
              RB.field('Upload limit (KiB/s)', RB.input(opt, 'limit_upload', {type: 'number', min: 0}), {hint: '0 = unlimited. 1 MB/s = 1024.'}),
              RB.field('Download limit (KiB/s)', RB.input(opt, 'limit_download', {type: 'number', min: 0}), {hint: '0 = unlimited.'}),
              RB.field('Connections', RB.input(opt, 'connections', {type: 'number', min: 0}), {hint: '0 = restic\'s default.'}),
              RB.field('Extra restic options', flags, {wide: true, optional: true}),
              h('div', {class: 'rb-field wide'}, RB.toggle(repo, 'unlock_stale', 'Remove stale locks automatically'),
                h('div', {class: 'rb-hint', text: 'When a run finds the repository locked by a process that no longer exists - after a crash or power cut.'})),
              h('div', {class: 'rb-field wide'}, RB.toggle(repo, 'append_only', 'The server keeps this repository append-only'),
                h('div', {class: 'rb-hint', text: 'Snapshots cannot be removed from here; retention and prune have to run on the server.'}))));

        var typeCards = isNew ? section('Type', 'repo', RB.optionCards(repo.type, [
            {value: 'sftp', title: 'SFTP', icon: 'server', text: 'Another server over SSH - a second Unraid, a NAS, a Storage Box, a VPS.'},
            {value: 'local', title: 'Local folder', icon: 'hdd', text: 'A disk in this server: an unassigned device or another pool.'},
            {value: 'rest', title: 'REST server', icon: 'cloud', text: 'restic\'s own server, optionally append-only against ransomware.'}
        ], function (v) {
            repo.type = v;
            renderType();
        })) : null;

        dlg = RB.overlay({
            drawer: true, title: isNew ? 'Add repository' : 'Edit "' + saved.name + '"', icon: 'repo',
            body: [errorSlot, typeCards,
                   section('Location', RB.repoIcon(repo.type), h('div', null,
                       h('div', {class: 'rb-fields', style: {marginBottom: '14px'}}, RB.field('Name', RB.input(repo, 'name', {placeholder: 'e.g. Offsite'}))),
                       typeSlot)),
                   section('Password', 'key', password, 'restic encrypts everything with it.'),
                   section('Maintenance', 'shield', maintenance),
                   h('div', {class: 'rb-form-section'}, advanced)],
            actions: [
                isNew ? null : RB.button('Delete', {icon: 'trash', kind: 'danger', onclick: function () {
                    deleteRepo(saved).then(function (gone) { if (gone) { dlg.close(); } });
                }}),
                h('div', {class: 'rb-spacer'}),
                RB.button('Cancel', {onclick: function () { dlg.close(); }}),
                RB.button(isNew ? 'Add repository' : 'Save', {kind: 'primary', icon: 'check', onclick: function (e) {
                    var params = {repo: repo, password_mode: pw.mode};
                    if (pw.mode === 'set') {
                        params.password = pw.value;
                    }
                    if (repo.type === 'rest' && rest.mode === 'set') {
                        params.rest_password_mode = 'set';
                        params.rest_password = rest.value;
                    }
                    RB.busy(e.currentTarget, RB.api('repo_save', params)).then(function (res) {
                        dlg.close();
                        RB.refresh().then(function () {
                            if (isNew) {
                                afterCreate(res.repo, res.generated_password);
                            } else {
                                RB.toast('Saved.');
                            }
                        });
                    }).catch(function (err) {
                        RB.clear(errorSlot).appendChild(RB.errorBox(err));
                        dlg.body.scrollTop = 0;
                    });
                }})
            ]
        });
    };

    function afterCreate(repo, generated) {
        var step = generated ? passwordDialog(repo, generated, true) : Promise.resolve();
        step.then(function () {
            if (repo.type === 'sftp') {
                return sshKeyDialog(repo, true).then(function (go) {
                    if (go) {
                        connect(repo);
                    } else {
                        RB.toast('Use "Test connection" in the repository\'s menu once the key is on the server.', 'info');
                    }
                });
            }
            return connect(repo);
        });
    }
})(window.RB);
