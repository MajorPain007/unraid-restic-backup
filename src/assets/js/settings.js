(function (RB) {
    'use strict';
    var h = RB.h;

    RB.views.settings = {
        render: function (container) {
            var o = RB.state.overview;
            var s = JSON.parse(JSON.stringify(o.config.settings));
            var errorSlot = h('div');
            var dataDir = RB.input(s, 'data_dir', {mono: true});

            function section(title, iconName, content, hint) {
                return h('div', {class: 'rb-form-section'}, h('h3', null, RB.icon(iconName), title),
                         hint ? h('div', {class: 'rb-hint', style: {marginBottom: '14px'}}, hint) : null, content);
            }

            var form = h('div', {class: 'rb-card', style: {maxWidth: '860px'}},
                errorSlot,
                section('Data folder', 'folder', h('div', {class: 'rb-fields'},
                    RB.field('Folder', h('div', {class: 'rb-input-group'}, dataDir,
                        RB.button('Browse...', {icon: 'folder', onclick: function () {
                            RB.pickPath({title: 'Data folder', create: true, start: s.data_dir}).then(function (p) {
                                if (p) {
                                    s.data_dir = p;
                                    dataDir.value = p;
                                }
                            });
                        }})), {wide: true})),
                    'restic\'s cache, the history of all runs, their logs and the schedule\'s memory. It has to be on a disk: ' +
                    'the cache can grow to gigabytes, and Unraid keeps /tmp in RAM. The system share on a cache pool is ideal: ' +
                    'like docker.img, nothing in it needs a backup of its own.'),
                section('Running', 'activity', h('div', {class: 'rb-fields'},
                    RB.field('CPU priority', RB.select(s, 'nice', [[0, 'Normal'], [10, 'Lower (recommended)'], [19, 'Lowest']]),
                             {hint: 'How restic shares the processor with everything else.'}),
                    RB.field('Disk priority', RB.select(s, 'io_priority', [['normal', 'Normal'], ['low', 'Lower (recommended)'], ['idle', 'Only when idle']]),
                             {hint: '"Only when idle" can stall a backup on a busy server.'}),
                    RB.field('Wait after the array starts', h('div', {class: 'rb-inline'},
                        RB.input(s, 'start_delay', {type: 'number', min: 0, max: 120}), 'minutes'),
                             {hint: 'Before missed backups catch up, so containers can start first.'}),
                    RB.field('History', h('div', {class: 'rb-inline'}, RB.input(s, 'history_days', {type: 'number', min: 7}), 'days'),
                             {hint: 'How long finished runs and their logs are listed.'}),
                    h('div', {class: 'rb-field wide'}, RB.toggle(s, 'parity_pause', 'No scheduled runs during a parity check'),
                      h('div', {class: 'rb-hint', text: 'They wait and run once it is done. Runs started by hand are not held back.'})))),
                section('Menu', 'archive', h('div', null,
                    RB.toggle(s, 'show_in_menu', 'Show "Backup" in the top menu, next to Docker and VMs'),
                    h('div', {class: 'rb-hint', style: {margin: '4px 0 0 44px'}},
                      'The page stays under Settings > Utilities either way. Takes effect when the page is loaded again.'))),
                section('Snapshots', 'layers', h('div', {class: 'rb-fields'},
                    RB.field('Host name', RB.input(s, 'host', {placeholder: o.system.server}), {optional: true,
                        hint: 'Recorded with every snapshot. Change it only if this server should continue the snapshots of another.'}))),
                h('div', {class: 'rb-card-foot'}, h('div', {class: 'rb-spacer', style: {flex: 1}}),
                  RB.button('Save settings', {kind: 'primary', icon: 'check', onclick: function (e) {
                      s.nice = Number(s.nice);
                      RB.busy(e.currentTarget, RB.api('settings_save', {settings: s})).then(function () {
                          RB.clear(errorSlot);
                          RB.toast('Settings saved.');
                          RB.refresh();
                      }).catch(function (err) {
                          RB.clear(errorSlot).appendChild(RB.errorBox(err));
                      });
                  }})));
            container.appendChild(form);

            var sys = o.system;
            container.appendChild(h('div', {class: 'rb-card', style: {maxWidth: '860px', marginTop: '16px'}},
                h('div', {class: 'rb-form-section'}, h('h3', null, RB.icon('info'), 'About'),
                  h('div', {class: 'rb-facts'},
                    fact('restic', sys.restic || 'not installed'),
                    fact('Server', sys.server),
                    fact('Time zone', sys.timezone),
                    fact('Array', sys.array_started ? 'Started' : 'Stopped')))));
        }
    };

    function fact(label, value) {
        return h('div', {class: 'rb-fact'}, h('div', {class: 'rb-label', text: label}), h('div', {class: 'rb-value', text: value}));
    }
})(window.RB);
