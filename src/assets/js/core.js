window.RB = (function () {
    'use strict';

    var root = document.getElementById('rb-app');
    var RB = {root: root, views: {}, state: {}};

    function h(tag, props) {
        var el = tag === 'svg' ? document.createElementNS('http://www.w3.org/2000/svg', 'svg') : document.createElement(tag);
        if (props) {
            Object.keys(props).forEach(function (k) {
                var v = props[k];
                if (v === null || v === undefined || v === false) {
                    return;
                }
                if (k.slice(0, 2) === 'on' && typeof v === 'function') {
                    el.addEventListener(k.slice(2), v);
                } else if (k === 'text') {
                    el.textContent = v;
                } else if (k === 'dataset') {
                    Object.keys(v).forEach(function (d) { el.dataset[d] = v[d]; });
                } else if (k === 'style' && typeof v === 'object') {
                    Object.keys(v).forEach(function (s) { el.style[s] = v[s]; });
                } else if (k === 'value' || k === 'checked' || k === 'disabled' || k === 'selected') {
                    el[k] = v;
                } else {
                    el.setAttribute(k, v === true ? '' : v);
                }
            });
        }
        for (var i = 2; i < arguments.length; i++) {
            append(el, arguments[i]);
        }
        return el;
    }

    function append(el, child) {
        if (child === null || child === undefined || child === false) {
            return;
        }
        if (Array.isArray(child)) {
            child.forEach(function (c) { append(el, c); });
        } else if (child instanceof Node) {
            el.appendChild(child);
        } else {
            el.appendChild(document.createTextNode(String(child)));
        }
    }

    function clear(el) {
        while (el.firstChild) {
            el.removeChild(el.firstChild);
        }
        return el;
    }

    var ICONS = {
        play: '<path d="M7 4.5v15l12-7.5z"/>',
        stop: '<rect x="6" y="6" width="12" height="12" rx="1.5"/>',
        edit: '<path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4z"/>',
        trash: '<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v6M14 11v6"/>',
        folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
        file: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
        link: '<path d="M10 14a4 4 0 0 0 6 0l3-3a4 4 0 0 0-6-6l-1 1"/><path d="M14 10a4 4 0 0 0-6 0l-3 3a4 4 0 0 0 6 6l1-1"/>',
        server: '<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
        hdd: '<path d="M2 13h20"/><path d="M5.5 5h13l3.5 8v5a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-5z"/><path d="M6 16.5h.01M10 16.5h.01"/>',
        cloud: '<path d="M7 18.5a4.5 4.5 0 0 1-.6-9 6 6 0 0 1 11.6 1.4 3.8 3.8 0 0 1-.5 7.6z"/>',
        check: '<path d="M20 6 9 17l-5-5"/>',
        x: '<path d="M18 6 6 18M6 6l12 12"/>',
        alert: '<path d="M10.3 4 2 18a2 2 0 0 0 1.7 3h16.6a2 2 0 0 0 1.7-3L13.7 4a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>',
        info: '<circle cx="12" cy="12" r="9.5"/><path d="M12 16v-4.5M12 8h.01"/>',
        clock: '<circle cx="12" cy="12" r="9.5"/><path d="M12 7v5l3.5 2"/>',
        refresh: '<path d="M20 12a8 8 0 0 1-14 5.3L4 15"/><path d="M4 20v-5h5"/><path d="M4 12a8 8 0 0 1 14-5.3L20 9"/><path d="M20 4v5h-5"/>',
        search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
        download: '<path d="M12 3v12M7 10l5 5 5-5"/><path d="M4 16v3a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3"/>',
        restore: '<path d="M3.5 12a8.5 8.5 0 1 0 2.6-6.1L3.5 8.5"/><path d="M3.5 3.5v5h5"/>',
        key: '<circle cx="8" cy="15" r="4.5"/><path d="m11.2 11.8 9.3-9.3M17 6l2.5 2.5M14.5 8.5l2 2"/>',
        lock: '<rect x="4.5" y="11" width="15" height="10" rx="2"/><path d="M8 11V7.5a4 4 0 0 1 8 0V11"/>',
        unlock: '<rect x="4.5" y="11" width="15" height="10" rx="2"/><path d="M8 11V7.5a4 4 0 0 1 7.6-1.7"/>',
        more: '<circle cx="5" cy="12" r="1.3"/><circle cx="12" cy="12" r="1.3"/><circle cx="19" cy="12" r="1.3"/>',
        right: '<path d="m9 18 6-6-6-6"/>',
        up: '<path d="m18 15-6-6-6 6"/>',
        plus: '<path d="M12 5v14M5 12h14"/>',
        repo: '<ellipse cx="12" cy="5.5" rx="8" ry="3"/><path d="M4 5.5v13c0 1.7 3.6 3 8 3s8-1.3 8-3v-13"/><path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
        calendar: '<rect x="3.5" y="5" width="17" height="16" rx="2"/><path d="M8 3v4M16 3v4M3.5 10h17"/>',
        activity: '<path d="M21 12h-4l-3 8-4-16-3 8H3"/>',
        settings: '<path d="M5 21v-7M5 10V3M12 21v-9M12 8V3M19 21v-5M19 12V3M2 14h6M9 8h6M16 16h6"/>',
        layers: '<path d="m12 3 9 4.5-9 4.5-9-4.5z"/><path d="m3 12 9 4.5 9-4.5"/><path d="m3 16.5 9 4.5 9-4.5"/>',
        copy: '<rect x="8.5" y="8.5" width="12" height="12" rx="2"/><path d="M15.5 8.5V5.5a2 2 0 0 0-2-2h-8a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h3"/>',
        eye: '<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
        compare: '<path d="M7 3v14M7 17l-3-3M7 17l3-3"/><path d="M17 21V7M17 7l-3 3M17 7l3 3"/>',
        shield: '<path d="M12 21.5s8-3.8 8-10V5.2L12 2.5 4 5.2v6.3c0 6.2 8 10 8 10z"/>',
        terminal: '<path d="m5 7 5 5-5 5M12 18h7"/>',
        bell: '<path d="M18 9a6 6 0 0 0-12 0c0 7-3 8.5-3 8.5h18S18 16 18 9"/><path d="M13.7 21a2 2 0 0 1-3.4 0"/>',
        archive: '<rect x="2.5" y="3.5" width="19" height="5" rx="1"/><path d="M4.5 8.5V19a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2V8.5M10 12.5h4"/>',
        loader: '<path d="M21 12a9 9 0 1 1-6.2-8.6"/>',
        home: '<path d="m3 10.5 9-7 9 7V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M9.5 21v-6h5v6"/>',
        power: '<path d="M12 3v8"/><path d="M6.4 6.6a8 8 0 1 0 11.2 0"/>',
        list: '<path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01"/>',
        folderPlus: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M12 10.5v5M9.5 13h5"/>'
    };

    function icon(name, cls) {
        var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg.setAttribute('viewBox', '0 0 24 24');
        svg.setAttribute('class', 'rb-i' + (cls ? ' ' + cls : ''));
        svg.setAttribute('aria-hidden', 'true');
        svg.innerHTML = ICONS[name] || '';
        return svg;
    }

    function button(label, opts) {
        opts = opts || {};
        var cls = 'rb-btn' + (opts.kind ? ' ' + opts.kind : '') + (opts.small ? ' sm' : '') + (label ? '' : ' icon');
        var b = h('button', {type: 'button', class: cls, title: opts.title || null, disabled: !!opts.disabled,
                             onclick: opts.onclick || null},
                  opts.icon ? icon(opts.icon) : null, label || null);
        return b;
    }

    function busy(btn, promise) {
        var original = Array.prototype.slice.call(btn.childNodes);
        btn.disabled = true;
        var spinner = icon('loader', 'rb-spin');
        if (btn.firstChild && btn.firstChild.nodeName.toLowerCase() === 'svg') {
            btn.replaceChild(spinner, btn.firstChild);
        } else {
            btn.insertBefore(spinner, btn.firstChild);
        }
        var restore = function () {
            if (!btn.isConnected) {
                return;
            }
            clear(btn);
            original.forEach(function (n) { btn.appendChild(n); });
            btn.disabled = false;
        };
        return Promise.resolve(promise).then(function (v) { restore(); return v; },
                                             function (e) { restore(); throw e; });
    }

    var LOCALE = 'en-GB';

    function bytes(n) {
        if (n === null || n === undefined || isNaN(n)) {
            return '-';
        }
        var units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
        var i = 0;
        n = Number(n);
        while (n >= 1024 && i < units.length - 1) {
            n /= 1024;
            i++;
        }
        return (i === 0 ? n : n.toFixed(n < 10 ? 1 : 0)) + ' ' + units[i];
    }

    function duration(s) {
        if (s === null || s === undefined || isNaN(s)) {
            return '-';
        }
        s = Math.max(0, Math.round(s));
        if (s < 60) {
            return s + ' s';
        }
        if (s < 3600) {
            return Math.round(s / 60) + ' min';
        }
        if (s < 86400) {
            var hh = Math.floor(s / 3600);
            var mm = Math.round((s % 3600) / 60);
            return hh + ' h' + (mm ? ' ' + mm + ' min' : '');
        }
        var d = Math.floor(s / 86400);
        var rh = Math.round((s % 86400) / 3600);
        return d + ' d' + (rh ? ' ' + rh + ' h' : '');
    }

    function now() {
        return Date.now() / 1000;
    }

    function relative(ts) {
        if (!ts) {
            return '-';
        }
        var diff = ts - now();
        var abs = Math.abs(diff);
        if (abs < 45) {
            return diff < 0 ? 'just now' : 'now';
        }
        var text;
        if (abs < 3600) {
            text = Math.round(abs / 60) + ' min';
        } else if (abs < 86400) {
            text = Math.round(abs / 3600) + ' h';
        } else if (abs < 86400 * 7) {
            var dt = new Date(ts * 1000);
            return dayName(dt, diff) + ' ' + clock(ts);
        } else {
            return dateTime(ts);
        }
        return diff < 0 ? text + ' ago' : 'in ' + text;
    }

    function dayName(dt, diff) {
        var today = new Date();
        var d0 = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime();
        var d1 = new Date(dt.getFullYear(), dt.getMonth(), dt.getDate()).getTime();
        var days = Math.round((d1 - d0) / 86400000);
        if (days === -1) {
            return 'yesterday';
        }
        if (days === 1) {
            return 'tomorrow';
        }
        return dt.toLocaleDateString(LOCALE, {weekday: 'short'});
    }

    function clock(ts) {
        return new Date(ts * 1000).toLocaleTimeString(LOCALE, {hour: '2-digit', minute: '2-digit'});
    }

    function dateTime(ts) {
        if (!ts) {
            return '-';
        }
        return new Date(ts * 1000).toLocaleString(LOCALE, {day: 'numeric', month: 'short', year: 'numeric',
                                                           hour: '2-digit', minute: '2-digit'});
    }

    function shortDate(ts, withYear) {
        return new Date(ts * 1000).toLocaleDateString(LOCALE, withYear ? {day: 'numeric', month: 'short', year: '2-digit'}
                                                                       : {day: 'numeric', month: 'short'});
    }

    function dayLabel(ts) {
        var dt = new Date(ts * 1000);
        var label = dayName(dt, ts - now());
        if (label === 'yesterday' || label === 'tomorrow') {
            return label.charAt(0).toUpperCase() + label.slice(1);
        }
        var today = new Date();
        if (dt.toDateString() === today.toDateString()) {
            return 'Today';
        }
        return dt.toLocaleDateString(LOCALE, {weekday: 'long', day: 'numeric', month: 'long',
                                              year: dt.getFullYear() === today.getFullYear() ? undefined : 'numeric'});
    }

    function plural(n, one, many) {
        return n + ' ' + (n === 1 ? one : (many || one + 's'));
    }

    var API = root.dataset.api;

    function csrf() {
        return (typeof window.csrf_token !== 'undefined' && window.csrf_token) || root.dataset.csrf || '';
    }

    function api(action, params) {
        var body = new URLSearchParams();
        body.append('action', action);
        body.append('csrf_token', csrf());
        Object.keys(params || {}).forEach(function (k) {
            var v = params[k];
            if (v === undefined || v === null) {
                return;
            }
            body.append(k, typeof v === 'object' ? JSON.stringify(v) : String(v));
        });
        return fetch(API, {method: 'POST', body: body, credentials: 'same-origin'}).then(function (res) {
            return res.text().then(function (text) {
                var data = null;
                try {
                    data = JSON.parse(text);
                } catch (e) {
                    var why = text === '' ? 'The server sent an empty answer (HTTP ' + res.status + '). ' +
                        'Reload the page - the security token may have expired.'
                        : 'The server sent something unexpected (HTTP ' + res.status + ').';
                    throw new Error(why);
                }
                if (!data.ok) {
                    var err = new Error(data.error || 'Something went wrong.');
                    err.errors = data.errors || [];
                    throw err;
                }
                return data;
            });
        });
    }

    function download(params) {
        var frame = document.getElementById('rb-download-frame');
        if (!frame) {
            frame = h('iframe', {id: 'rb-download-frame', name: 'rb-download-frame', style: {display: 'none'}});
            document.body.appendChild(frame);
        }
        var form = h('form', {method: 'POST', action: API, target: 'rb-download-frame', style: {display: 'none'}});
        var all = Object.assign({action: 'download', csrf_token: csrf()}, params);
        Object.keys(all).forEach(function (k) {
            form.appendChild(h('input', {type: 'hidden', name: k, value: all[k]}));
        });
        document.body.appendChild(form);
        form.submit();
        setTimeout(function () { form.remove(); }, 1000);
    }

    var toastBox = null;

    function toast(message, kind) {
        if (!toastBox || !toastBox.isConnected) {
            toastBox = h('div', {class: 'rb-toasts', role: 'status', 'aria-live': 'polite'});
            root.appendChild(toastBox);
        }
        var t = h('div', {class: 'rb-toast ' + (kind || 'ok')},
                  icon(kind === 'err' ? 'alert' : (kind === 'info' ? 'info' : 'check')), h('div', null, message));
        toastBox.appendChild(t);
        setTimeout(function () { t.remove(); }, kind === 'err' ? 9000 : 4500);
    }

    function fail(err) {
        toast(err && err.message ? err.message : String(err), 'err');
    }

    var openOverlays = [];

    function overlay(opts) {
        var isDrawer = opts.drawer;
        var head = h('div', {class: 'rb-panel-head'},
                     opts.icon ? icon(opts.icon) : null,
                     h('h2', {text: opts.title}),
                     button('', {icon: 'x', kind: 'ghost', title: 'Close', onclick: function () { close(); }}));
        var body = h('div', {class: 'rb-panel-body'});
        var foot = h('div', {class: 'rb-panel-foot'});
        var panel = h('div', {class: isDrawer ? 'rb-drawer' : 'rb-modal' + (opts.wide ? ' wide' : ''),
                              role: 'dialog', 'aria-modal': 'true'}, head, body, foot);
        var back = h('div', {class: 'rb-overlay' + (isDrawer ? '' : ' center')}, panel);
        var closed = false;

        function close(result) {
            if (closed) {
                return;
            }
            if (opts.beforeClose && opts.beforeClose(result) === false) {
                return;
            }
            closed = true;
            back.remove();
            openOverlays = openOverlays.filter(function (o) { return o !== handle; });
            if (opts.onclose) {
                opts.onclose(result);
            }
        }

        back.addEventListener('mousedown', function (e) {
            if (e.target === back && !opts.sticky) {
                close();
            }
        });
        var handle = {el: panel, body: body, foot: foot, close: close, head: head};
        openOverlays.push(handle);
        root.appendChild(back);
        if (opts.body) {
            append(body, opts.body);
        }
        if (opts.actions) {
            append(foot, opts.actions);
        } else {
            foot.remove();
        }
        setTimeout(function () {
            var f = panel.querySelector('input:not([type=hidden]):not([type=checkbox]), select, textarea');
            (f || head.querySelector('button')).focus();
        }, 30);
        return handle;
    }

    document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape' && openOverlays.length) {
            closeMenu();
            openOverlays[openOverlays.length - 1].close();
        }
    });

    function confirm(opts) {
        return new Promise(function (resolve) {
            var o;
            var yes = button(opts.confirm || 'OK', {kind: opts.danger ? 'danger solid' : 'primary', onclick: function () {
                o.close(true);
            }});
            o = overlay({
                title: opts.title,
                icon: opts.danger ? 'alert' : null,
                body: typeof opts.text === 'string' ? h('p', {text: opts.text}) : opts.text,
                actions: [h('div', {class: 'rb-spacer'}), button('Cancel', {onclick: function () { o.close(false); }}), yes],
                onclose: function (r) { resolve(r === true); }
            });
            setTimeout(function () { yes.focus(); }, 40);
        });
    }

    var menuEl = null;

    function closeMenu() {
        if (menuEl) {
            menuEl.remove();
            menuEl = null;
            emit('menuclosed');
        }
    }

    function menuOpen() {
        return !!(menuEl && menuEl.isConnected);
    }

    function menu(anchor, items) {
        closeMenu();
        menuEl = h('div', {class: 'rb-menu', role: 'menu'}, items.filter(Boolean).map(function (it) {
            if (it === '-') {
                return h('hr');
            }
            return h('button', {type: 'button', role: 'menuitem', class: it.danger ? 'danger' : null, disabled: !!it.disabled,
                                onclick: function () { closeMenu(); it.onclick(); }},
                     icon(it.icon || 'right'), it.label);
        }));
        root.appendChild(menuEl);
        var r = anchor.getBoundingClientRect();
        var w = menuEl.offsetWidth;
        var top = r.bottom + 4;
        if (top + menuEl.offsetHeight > window.innerHeight - 8) {
            top = Math.max(8, r.top - menuEl.offsetHeight - 4);
        }
        menuEl.style.top = top + 'px';
        menuEl.style.left = Math.max(8, Math.min(r.right - w, window.innerWidth - w - 8)) + 'px';
        var first = menuEl.querySelector('button');
        if (first) {
            first.focus();
        }
    }

    document.addEventListener('mousedown', function (e) {
        if (menuEl && !menuEl.contains(e.target)) {
            closeMenu();
        }
    });
    window.addEventListener('scroll', closeMenu, true);
    window.addEventListener('resize', closeMenu);

    function field(label, control, opts) {
        opts = opts || {};
        return h('div', {class: 'rb-field' + (opts.wide ? ' wide' : '')},
                 label ? h('label', null, label, opts.optional ? h('span', {class: 'opt', text: ' (optional)'}) : null) : null,
                 control,
                 opts.hint ? h('div', {class: 'rb-hint'}, opts.hint) : null);
    }

    function input(obj, key, opts) {
        opts = opts || {};
        var type = opts.type || 'text';
        var el = h('input', {type: type, value: obj[key] === undefined || obj[key] === null ? '' : obj[key],
                             placeholder: opts.placeholder || null, min: opts.min, max: opts.max, step: opts.step,
                             autocomplete: 'off', spellcheck: 'false', class: opts.mono ? 'rb-mono' : null,
                             'data-lpignore': 'true', 'data-1p-ignore': 'true'});
        el.addEventListener('input', function () {
            obj[key] = type === 'number' ? (el.value === '' ? 0 : Number(el.value)) : el.value;
            if (opts.oninput) {
                opts.oninput(obj[key]);
            }
        });
        return el;
    }

    function select(obj, key, options, onchange) {
        var el = h('select', null, options.map(function (o) {
            return h('option', {value: o[0], selected: String(obj[key]) === String(o[0])}, o[1]);
        }));
        el.addEventListener('change', function () {
            obj[key] = el.value;
            if (onchange) {
                onchange(el.value);
            }
        });
        return el;
    }

    function toggle(obj, key, label, onchange) {
        var box = h('input', {type: 'checkbox', checked: !!obj[key]});
        box.addEventListener('change', function () {
            obj[key] = box.checked;
            if (onchange) {
                onchange(box.checked);
            }
        });
        return h('label', {class: 'rb-switch'}, box, h('span'), label);
    }

    function segmented(current, options, onchange) {
        var wrap = h('div', {class: 'rb-seg', role: 'radiogroup'});
        options.forEach(function (o) {
            var b = h('button', {type: 'button', role: 'radio', class: o[0] === current ? 'active' : null,
                                 'aria-checked': o[0] === current ? 'true' : 'false'}, o[1]);
            b.addEventListener('click', function () {
                Array.prototype.forEach.call(wrap.children, function (c) {
                    c.classList.remove('active');
                    c.setAttribute('aria-checked', 'false');
                });
                b.classList.add('active');
                b.setAttribute('aria-checked', 'true');
                onchange(o[0]);
            });
            wrap.appendChild(b);
        });
        return wrap;
    }

    function optionCards(current, options, onchange, cls) {
        var wrap = h('div', {class: 'rb-options' + (cls ? ' ' + cls : ''), role: 'radiogroup'});
        options.forEach(function (o) {
            var b = h('button', {type: 'button', role: 'radio', class: 'rb-option' + (o.value === current ? ' active' : ''),
                                 'aria-checked': o.value === current ? 'true' : 'false'},
                      h('b', null, o.icon ? icon(o.icon) : null, o.title), h('span', {text: o.text}));
            b.addEventListener('click', function () {
                Array.prototype.forEach.call(wrap.children, function (c) {
                    c.classList.remove('active');
                    c.setAttribute('aria-checked', 'false');
                });
                b.classList.add('active');
                b.setAttribute('aria-checked', 'true');
                onchange(o.value);
            });
            wrap.appendChild(b);
        });
        return wrap;
    }

    function errorBox(err) {
        var list = (err && err.errors) || [];
        return h('div', {class: 'rb-errors', role: 'alert'},
                 h('div', null, h('b', {text: err.message || 'Please check the form.'})),
                 list.length ? h('ul', null, list.map(function (e) { return h('li', {text: e}); })) : null);
    }

    function copyText(text) {
        var done = function () { toast('Copied to the clipboard'); };
        if (navigator.clipboard && window.isSecureContext) {
            navigator.clipboard.writeText(text).then(done, function () { legacyCopy(text) && done(); });
        } else if (legacyCopy(text)) {
            done();
        } else {
            toast('Copying is blocked here - select the text and copy it by hand.', 'err');
        }
    }

    function legacyCopy(text) {
        var ta = h('textarea', {style: {position: 'fixed', top: '-1000px'}});
        ta.value = text;
        document.body.appendChild(ta);
        ta.select();
        var ok = false;
        try {
            ok = document.execCommand('copy');
        } catch (e) {
            ok = false;
        }
        ta.remove();
        return ok;
    }

    function pickPath(opts) {
        opts = opts || {};
        return new Promise(function (resolve) {
            var current = opts.start || '/mnt';
            var chosen = null;
            var picked = [];
            var list = h('div', {class: 'rb-table-wrap rb-browser-scroll', style: {maxHeight: '50vh'}});
            var crumbs = h('div', {class: 'rb-crumbs'});
            var pathInput = h('input', {type: 'text', class: 'rb-mono', value: current, autocomplete: 'off', spellcheck: 'false'});
            var newSlot = h('div');
            var pickedBox = h('div', {class: 'rb-picked'});
            var newBtn = opts.create ? button('New folder', {icon: 'folderPlus', small: true, kind: 'ghost', onclick: newFolder}) : null;
            var label = opts.confirm || 'Choose';
            var ok = button(label, {kind: 'primary', onclick: function () {
                var typed = pathInput.value.trim() || current;
                chosen = opts.multiple ? (picked.length ? picked.slice() : [typed]) : typed;
                o.close();
            }});
            var o;

            var first = true;

            function renderPicked() {
                clear(pickedBox);
                if (!picked.length) {
                    pickedBox.appendChild(h('span', {class: 'rb-hint', text: 'Tick folders to add several at once, from different places too.'}));
                }
                picked.forEach(function (p) {
                    pickedBox.appendChild(h('span', {class: 'rb-picked-item', title: p}, h('span', {class: 'rb-mono', text: p}),
                        h('button', {type: 'button', title: 'Remove', onclick: function () { tick(p, false); }}, icon('x'))));
                });
                ok.lastChild.textContent = picked.length > 1 ? label + ' ' + picked.length + ' folders' : label;
                list.querySelectorAll('input[type=checkbox]').forEach(function (cb) {
                    cb.checked = picked.indexOf(cb.value) !== -1;
                });
            }

            function tick(path, on) {
                var i = picked.indexOf(path);
                if (on && i === -1) {
                    picked.push(path);
                } else if (!on && i !== -1) {
                    picked.splice(i, 1);
                }
                renderPicked();
            }

            function newFolder() {
                clear(newSlot);
                var name = h('input', {type: 'text', placeholder: 'Name of the new folder', autocomplete: 'off', spellcheck: 'false'});
                var create = button('Create', {small: true, kind: 'primary', onclick: go});
                function go() {
                    var n = name.value.trim();
                    if (!n) {
                        name.focus();
                        return;
                    }
                    busy(create, api('mkdir', {path: current, name: n})).then(function (r) {
                        clear(newSlot);
                        load(r.path);
                    }).catch(fail);
                }
                name.addEventListener('keydown', function (e) {
                    if (e.key === 'Enter') {
                        e.preventDefault();
                        go();
                    } else if (e.key === 'Escape') {
                        e.stopPropagation();
                        clear(newSlot);
                    }
                });
                newSlot.appendChild(h('div', {class: 'rb-newfolder'}, icon('folderPlus'), name, create,
                    button('Cancel', {small: true, kind: 'ghost', onclick: function () { clear(newSlot); }})));
                name.focus();
            }

            function load(path) {
                api('browse', {path: path, files: opts.files ? 1 : 0}).then(function (r) {
                    current = r.path;
                    if (!first || r.path === path) {
                        pathInput.value = current === '/' ? '' : current;
                    }
                    first = false;
                    if (newBtn) {
                        newBtn.disabled = current.indexOf('/mnt/') !== 0;
                    }
                    clear(crumbs);
                    var parts = current.split('/').filter(Boolean);
                    crumbs.appendChild(h('button', {type: 'button', onclick: function () { load('/'); }}, icon('home')));
                    parts.forEach(function (p, i) {
                        var target = '/' + parts.slice(0, i + 1).join('/');
                        crumbs.appendChild(h('span', {class: 'rb-sep', text: '/'}));
                        crumbs.appendChild(h('button', {type: 'button', text: p, onclick: function () { load(target); }}));
                    });
                    clear(list);
                    var rows = r.entries.map(function (e) {
                        var cb = null;
                        if (opts.multiple && r.path !== '/' && (e.dir || opts.files)) {
                            cb = h('input', {type: 'checkbox', value: e.path, checked: picked.indexOf(e.path) !== -1});
                            cb.addEventListener('change', function () { tick(e.path, cb.checked); });
                        }
                        return h('tr', {class: 'clickable', onclick: function () {
                            if (e.dir) {
                                load(e.path);
                            } else {
                                pathInput.value = e.path;
                            }
                        }}, opts.multiple ? h('td', {class: 'pick', onclick: function (ev) {
                            ev.stopPropagation();
                            if (cb && ev.target !== cb) {
                                cb.checked = !cb.checked;
                                tick(e.path, cb.checked);
                            }
                        }}, cb) : null,
                        h('td', {class: 'name'}, icon(e.dir ? 'folder' : 'file', e.dir ? 'dir' : 'file'), h('span', {text: e.name})));
                    });
                    list.appendChild(h('table', {class: 'rb-table'}, h('tbody', null,
                        rows.length ? rows : h('tr', null, h('td', {class: 'dim', colspan: 2, text: 'No folders here.'})))));
                    list.classList.add('rb-browser');
                }).catch(fail);
            }

            o = overlay({
                title: opts.title || 'Choose a folder',
                icon: 'folder',
                wide: !!opts.multiple,
                body: [h('div', {class: 'rb-browser-head', style: {padding: '0 0 10px', border: 0}}, crumbs, newBtn), newSlot, list,
                       opts.multiple ? pickedBox : null,
                       h('div', {style: {marginTop: '12px'}}, field('Path', pathInput))],
                actions: [h('div', {class: 'rb-spacer'}),
                          button('Cancel', {onclick: function () { o.close(); }}),
                          ok],
                onclose: function () { resolve(chosen); }
            });
            if (opts.multiple) {
                renderPicked();
            }
            load(current);
        });
    }

    function applyTheme() {
        var el = root.parentElement;
        var bg = null;
        while (el && el !== document.documentElement) {
            var c = getComputedStyle(el).backgroundColor;
            if (c && c !== 'transparent' && !/rgba\(.*,\s*0\)$/.test(c)) {
                bg = c;
                break;
            }
            el = el.parentElement;
        }
        bg = bg || getComputedStyle(document.body).backgroundColor || 'rgb(255,255,255)';
        var m = bg.match(/\d+(\.\d+)?/g) || [255, 255, 255];
        var lum = (0.2126 * m[0] + 0.7152 * m[1] + 0.0722 * m[2]) / 255;
        root.classList.toggle('rb-light', lum > 0.55);
    }

    function pref(key, value) {
        try {
            if (value === undefined) {
                return localStorage.getItem('rb.' + key);
            }
            localStorage.setItem('rb.' + key, value);
        } catch (e) {
            return null;
        }
        return value;
    }

    var listeners = {};

    function on(name, fn) {
        (listeners[name] = listeners[name] || []).push(fn);
    }

    function emit(name, data) {
        (listeners[name] || []).forEach(function (fn) { fn(data); });
    }

    Object.assign(RB, {
        h: h, clear: clear, icon: icon, button: button, busy: busy,
        bytes: bytes, duration: duration, relative: relative, clock: clock, dateTime: dateTime, dayLabel: dayLabel, shortDate: shortDate,
        plural: plural, now: now,
        api: api, download: download, toast: toast, fail: fail,
        overlay: overlay, confirm: confirm, menu: menu, closeMenu: closeMenu, menuOpen: menuOpen,
        field: field, input: input, select: select, toggle: toggle, segmented: segmented, optionCards: optionCards,
        errorBox: errorBox, copyText: copyText, pickPath: pickPath, applyTheme: applyTheme, pref: pref,
        on: on, emit: emit
    });
    return RB;
})();
