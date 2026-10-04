(function (RB) {
    'use strict';
    var h = RB.h;
    var SVG = 'http://www.w3.org/2000/svg';
    var DAY = 86400;

    function s(tag, attrs) {
        var el = document.createElementNS(SVG, tag);
        Object.keys(attrs || {}).forEach(function (k) {
            el.setAttribute(k, attrs[k]);
        });
        return el;
    }

    function niceStep(raw) {
        var unit = 1;
        while (raw / unit >= 1024) {
            unit *= 1024;
        }
        var m = raw / unit;
        var pow = Math.pow(10, Math.floor(Math.log10(m)));
        var step = [1, 2, 2.5, 5, 10].map(function (k) { return k * pow; }).find(function (t) { return t >= m; });
        return step >= 1000 ? 1024 * unit : step * unit;
    }

    function axis(lo, hi, fromZero) {
        hi = Math.max(hi, 1024);
        var base = fromZero || lo <= hi * 0.5 ? 0 : lo;
        var step = niceStep(Math.max(hi - base, hi * 0.02) / 4);
        var start = Math.floor(base / step) * step;
        var ticks = [];
        for (var k = 0; k === 0 || start + (k - 1) * step < hi; k++) {
            ticks.push(start + k * step);
        }
        return ticks;
    }

    RB.chart = function (points, opts) {
        var W = 720, H = opts.height || 170, L = 62, R = 12, T = 12, B = 26;
        var from = opts.from, to = opts.to;
        var pts = points.filter(function (p) { return p[0] >= from && p[0] <= to; });
        var wrap = h('div', {class: 'rb-chart'});
        if (!pts.length) {
            wrap.appendChild(h('div', {class: 'rb-chart-empty', text: opts.empty || 'Nothing recorded in this time yet.'}));
            return wrap;
        }
        var val = opts.value;
        var values = pts.map(val);
        var ticks = axis(Math.min.apply(null, values), Math.max.apply(null, values), opts.kind === 'bars' || pts.length < 2);
        var min = ticks[0];
        var max = ticks[ticks.length - 1];
        var x = function (t) { return L + (t - from) / Math.max(1, to - from) * (W - L - R); };
        var y = function (v) { return T + (1 - (v - min) / (max - min)) * (H - T - B); };
        var svg = s('svg', {viewBox: '0 0 ' + W + ' ' + H, class: 'rb-chart-svg', role: 'img', 'aria-label': opts.label || ''});

        ticks.forEach(function (v) {
            svg.appendChild(s('line', {x1: L, x2: W - R, y1: y(v), y2: y(v), class: 'rb-chart-grid'}));
            var label = s('text', {x: L - 8, y: y(v) + 4, 'text-anchor': 'end', class: 'rb-chart-label'});
            label.textContent = v === 0 ? '0' : RB.bytes(v);
            svg.appendChild(label);
        });
        var spanDays = (to - from) / DAY;
        for (var k = 0; k <= 4; k++) {
            var t = from + (to - from) * k / 4;
            var tick = s('text', {x: x(t), y: H - 7, 'text-anchor': k === 0 ? 'start' : (k === 4 ? 'end' : 'middle'), class: 'rb-chart-label'});
            tick.textContent = RB.shortDate(t, spanDays > 330);
            svg.appendChild(tick);
        }

        if (opts.kind === 'bars') {
            var gap = (to - from);
            for (var j = 1; j < pts.length; j++) {
                gap = Math.min(gap, pts[j][0] - pts[j - 1][0]);
            }
            var bw = Math.max(1.5, Math.min(18, (gap / Math.max(1, to - from)) * (W - L - R) * 0.7));
            pts.forEach(function (p) {
                var top = y(val(p));
                svg.appendChild(s('rect', {x: x(p[0]) - bw / 2, y: Math.min(top, H - B - 1), width: bw,
                                           height: Math.max(1, H - B - top), rx: Math.min(2, bw / 3), class: 'rb-chart-bar'}));
            });
        } else {
            var line = pts.map(function (p, n) { return (n ? 'L' : 'M') + x(p[0]).toFixed(1) + ' ' + y(val(p)).toFixed(1); }).join(' ');
            var first = x(pts[0][0]).toFixed(1);
            var last = x(pts[pts.length - 1][0]).toFixed(1);
            svg.appendChild(s('path', {d: line + ' L' + last + ' ' + (H - B) + ' L' + first + ' ' + (H - B) + ' Z', class: 'rb-chart-area'}));
            svg.appendChild(s('path', {d: line, class: 'rb-chart-line'}));
            if (pts.length < 60) {
                pts.forEach(function (p) {
                    svg.appendChild(s('circle', {cx: x(p[0]), cy: y(val(p)), r: 2.6, class: 'rb-chart-dot'}));
                });
            }
        }

        var guide = s('line', {y1: T, y2: H - B, class: 'rb-chart-guide', visibility: 'hidden'});
        var marker = s('circle', {r: 4, class: 'rb-chart-marker', visibility: 'hidden'});
        svg.appendChild(guide);
        svg.appendChild(marker);
        var tip = h('div', {class: 'rb-chart-tip'});
        svg.addEventListener('mousemove', function (e) {
            var box = svg.getBoundingClientRect();
            var at = from + ((e.clientX - box.left) / box.width * W - L) / (W - L - R) * (to - from);
            var near = pts.reduce(function (a, p) { return Math.abs(p[0] - at) < Math.abs(a[0] - at) ? p : a; }, pts[0]);
            var px = x(near[0]);
            var py = y(val(near));
            guide.setAttribute('x1', px);
            guide.setAttribute('x2', px);
            guide.setAttribute('visibility', 'visible');
            marker.setAttribute('cx', px);
            marker.setAttribute('cy', py);
            marker.setAttribute('visibility', 'visible');
            RB.clear(tip).appendChild(h('div', null, h('b', {text: RB.bytes(val(near))}),
                h('span', {text: '  ' + RB.dateTime(near[0])}),
                opts.detail ? h('div', {class: 'rb-hint', text: opts.detail(near)}) : null));
            tip.style.display = 'block';
            var left = px / W * box.width;
            tip.style.left = Math.min(Math.max(left, 70), box.width - 70) + 'px';
            tip.style.top = (py / H * box.height) + 'px';
        });
        svg.addEventListener('mouseleave', function () {
            guide.setAttribute('visibility', 'hidden');
            marker.setAttribute('visibility', 'hidden');
            tip.style.display = 'none';
        });
        wrap.appendChild(svg);
        wrap.appendChild(tip);
        return wrap;
    };

    var RANGES = [['30', '30 days'], ['90', '90 days'], ['365', '1 year'], ['all', 'All']];

    RB.sizeHistory = function (which) {
        var body = h('div', null, h('div', {class: 'rb-hint'}, RB.icon('loader', 'rb-spin'), ' Loading...'));
        var dlg = RB.overlay({title: 'Size history', icon: 'activity', wide: true, body: body,
                              actions: [h('div', {class: 'rb-spacer'}), RB.button('Close', {onclick: function () { dlg.close(); }})]});
        RB.api('size_history', which).then(function (d) {
            var range = {value: RB.pref('sizeRange') || '90'};
            var charts = h('div');
            function draw() {
                RB.clear(charts);
                var now = Math.floor(Date.now() / 1000);
                var all = [].concat(d.repo.points, [].concat.apply([], d.jobs.map(function (j) { return j.points; })));
                var oldest = all.length ? Math.min.apply(null, all.map(function (p) { return p[0]; })) - DAY : now - 30 * DAY;
                var from = range.value === 'all' ? oldest : Math.max(oldest, now - Number(range.value) * DAY);
                from = Math.min(from, now - DAY);
                charts.appendChild(h('div', {class: 'rb-chart-block'},
                    h('div', {class: 'rb-chart-title'}, RB.icon('repo'), h('b', {text: d.repo.name}),
                      h('span', {class: 'rb-hint', text: '  on disk'})),
                    RB.chart(d.repo.points, {from: from, to: now, kind: 'line', label: 'Repository size',
                        value: function (p) { return p[1]; },
                        detail: function (p) { return RB.plural(p[2], 'snapshot'); },
                        empty: 'Measured after the next backup - or now with Measure size on the repository.'})));
                d.jobs.forEach(function (j) {
                    charts.appendChild(h('div', {class: 'rb-chart-block'},
                        h('div', {class: 'rb-chart-title'}, RB.icon('archive'), h('b', {text: j.name}),
                          h('span', {class: 'rb-hint', text: '  backed up per run'})),
                        RB.chart(j.points, {from: from, to: now, kind: 'line', height: 150, label: 'Backed up',
                            value: function (p) { return p[1]; },
                            detail: function (p) { return Number(p[3]).toLocaleString() + ' files'; },
                            empty: 'Recorded with the next backup.'}),
                        h('div', {class: 'rb-chart-sub', text: 'Added to the repository per run'}),
                        RB.chart(j.points, {from: from, to: now, kind: 'bars', height: 110, label: 'Added',
                            value: function (p) { return p[2]; }, empty: ' '})));
                });
            }
            RB.clear(body).appendChild(h('div', null,
                h('div', {style: {marginBottom: '14px'}}, RB.segmented(range.value, RANGES, function (v) {
                    range.value = v;
                    RB.pref('sizeRange', v);
                    draw();
                })),
                charts));
            draw();
        }).catch(function (e) {
            RB.clear(body).appendChild(h('div', {class: 'rb-card-note err'}, RB.icon('alert'), h('div', {text: e.message})));
        });
        return dlg;
    };
})(window.RB);
