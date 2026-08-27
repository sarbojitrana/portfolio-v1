(function () {
  "use strict";

  var el = document.getElementById("page-data");
  if (!el) return;
  var DATA = JSON.parse(el.textContent);
  var reduced = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var $ = function (id) { return document.getElementById(id); };

  function spark(id, series, bands) {
    var svg = $(id);
    if (!svg || !series.length) return;
    var W = 600, H = 150, PL = 4, PR = 40, PT = 12, PB = 16;
    var lo = Math.min.apply(null, series), hi = Math.max.apply(null, series);
    var pad = Math.max(60, (hi - lo) * 0.18);
    lo = Math.floor((lo - pad) / 50) * 50;
    hi = Math.ceil((hi + pad) / 50) * 50;
    var x = function (i) { return PL + (i / Math.max(1, series.length - 1)) * (W - PL - PR); };
    var y = function (v) { return PT + (1 - (v - lo) / (hi - lo)) * (H - PT - PB); };
    var out = "";
    bands.forEach(function (b) {
      if (b[0] < lo || b[0] > hi) return;
      var yy = y(b[0]).toFixed(1);
      out += '<line x1="' + PL + '" y1="' + yy + '" x2="' + (W - PR) + '" y2="' + yy +
        '" stroke="currentColor" stroke-width="1" stroke-dasharray="3 4" opacity=".34"/>' +
        '<text x="' + (W - PR + 5) + '" y="' + (+yy + 3.4) +
        '" font-family="JetBrains Mono, monospace" font-size="9" fill="currentColor" opacity=".62">' + b[0] + "</text>";
    });
    var pts = series.map(function (v, i) { return x(i).toFixed(1) + "," + y(v).toFixed(1); });
    out += '<polygon points="' + PL + "," + (H - PB) + " " + pts.join(" ") + " " + (W - PR) + "," + (H - PB) +
      '" fill="currentColor" opacity=".13"/>';
    out += '<polyline points="' + pts.join(" ") + '" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/>';
    var pk = series.indexOf(hi === lo ? series[0] : Math.max.apply(null, series));
    out += '<rect x="' + (x(pk) - 4).toFixed(1) + '" y="' + (y(series[pk]) - 4).toFixed(1) + '" width="8" height="8" fill="currentColor"/>';
    var last = series.length - 1;
    out += '<rect x="' + (x(last) - 3).toFixed(1) + '" y="' + (y(series[last]) - 3).toFixed(1) +
      '" width="6" height="6" fill="none" stroke="currentColor" stroke-width="2"/>';
    svg.innerHTML = out;
  }

  function hist(id, rows) {
    var e = $(id);
    if (!e || !rows.length) return;
    var max = rows.reduce(function (m, r) { return Math.max(m, r[1]); }, 1);
    e.innerHTML = rows.map(function (r) {
      return '<i>' + r[0] + '</i><span class="b-wrap"><span class="b" style="width:' +
        Math.max(1, Math.round((r[1] / max) * 100)) + '%"></span></span><em>' + r[1] + "</em>";
    }).join("");
  }

  spark("cf-spark", DATA.codeforces.ratings, DATA.codeforces.bands);
  spark("lc-spark", DATA.leetcode.ratings, DATA.leetcode.bands);
  hist("cf-hist", DATA.codeforces.histogram);
  hist("lc-hist", DATA.leetcode.histogram);

  var bar = $("langbar"), key = $("langkey");
  if (bar && key) {
    var total = DATA.languages.reduce(function (a, l) { return a + l[1]; }, 0);
    var fade = function (i) { return (1 - i * 0.125).toFixed(3); };
    bar.innerHTML = DATA.languages.map(function (l, i) {
      return '<span style="flex:' + l[1] + ";background:var(--ink);opacity:" + fade(i) +
        ';border-right:1px solid var(--ground)"></span>';
    }).join("");
    key.innerHTML = DATA.languages.map(function (l, i) {
      return "<span><b style=\"background:var(--ink);opacity:" + fade(i) + '"></b>' + l[0] + " " +
        Math.round((l[1] / total) * 100) + "%</span>";
    }).join("");
  }

  var heatEl = $("heat"), monthsEl = $("heat-months");
  if (heatEl && monthsEl && DATA.contributions) {
    var H = DATA.contributions;
    var B36 = "0123456789abcdefghijklmnopqrstuvwxyz";
    var MON = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
    var d0 = new Date(H.start + "T00:00:00Z"), cells = [], marks = [], lastMonth = -1;
    for (var i = 0; i < H.levels.length; i++) {
      var d = new Date(d0.getTime() + i * 86400000);
      var n = B36.indexOf(H.counts.charAt(i));
      var label = (n === 0 ? "No contributions" : n + (n === 1 ? " contribution" : " contributions")) +
        " on " + MON[d.getUTCMonth()] + " " + d.getUTCDate() + ", " + d.getUTCFullYear();
      cells.push('<i data-l="' + H.levels.charAt(i) + '" title="' + label + '"></i>');
      if (d.getUTCDay() === 0 && d.getUTCMonth() !== lastMonth) {
        lastMonth = d.getUTCMonth();
        marks.push('<span style="grid-column:' + (Math.floor(i / 7) + 1) + '">' + MON[lastMonth] + "</span>");
      }
    }
    heatEl.innerHTML = cells.join("");
    monthsEl.innerHTML = marks.join("");
  }

  var rowsEl = $("os-rows");
  if (rowsEl) {
    var PRS = DATA.prs, LIMIT = DATA.prPageSize || 5, offset = 0;
    var pages = Math.ceil(PRS.length / LIMIT);
    var render = function () {
      rowsEl.innerHTML = PRS.slice(offset, offset + LIMIT).map(function (r) {
        var url = "https://github.com/" + r.repo + "/pull/" + r.number;
        return '<tr><td class="repo">' + r.repo + '</td><td class="num">#' + r.number + "</td>" +
          '<td class="ttl"><a href="' + url + '" target="_blank" rel="noopener">' + r.title + "</a></td>" +
          '<td class="df">' + r.diff + '</td><td class="dt">' + r.merged + "</td></tr>";
      }).join("");
      $("os-read").textContent = "offset " + offset + " · limit " + LIMIT + " · " + PRS.length + " rows";
      $("os-page").textContent = "page " + (offset / LIMIT + 1) + " / " + pages;
      $("os-prev").disabled = offset === 0;
      $("os-next").disabled = offset + LIMIT >= PRS.length;
    };
    $("os-prev").addEventListener("click", function () { offset = Math.max(0, offset - LIMIT); render(); });
    $("os-next").addEventListener("click", function () {
      if (offset + LIMIT < PRS.length) { offset += LIMIT; render(); }
    });
    render();
  }

  var chain = $("chain");
  if (!chain) return;

  var RP = DATA.requestPath, CAP = RP.capacity, REFILL = RP.refillMs, TTL = RP.cacheTtlMs;
  var tokens = CAP, lastRefill = Date.now(), cacheUntil = 0, seq = 0, running = false, queue = 0;
  var stages = {};
  Array.prototype.forEach.call(chain.children, function (s) { stages[s.dataset.k] = s; });
  var tokEl = $("tokens"), bucketN = $("bucket-n"), bucketR = $("bucket-r"),
      rlNote = $("rl-note"), cacheNote = $("cache-note"), verdict = $("verdict"), log = $("log");

  tokEl.innerHTML = new Array(CAP).fill('<span class="tok"></span>').join("");
  var toks = tokEl.children;

  function paintBucket() {
    var n = Math.floor(tokens);
    for (var i = 0; i < CAP; i++) toks[i].className = "tok" + (i < n ? " on" : "");
    bucketN.textContent = n + (n === 1 ? " token" : " tokens");
    rlNote.textContent = n + " / " + CAP;
    var wait = (REFILL - (Date.now() - lastRefill)) / 1000;
    bucketR.textContent = n >= CAP ? "bucket full" : "refill " + Math.max(0, wait).toFixed(1) + "s";
  }
  setInterval(function () {
    if (tokens < CAP && Date.now() - lastRefill >= REFILL) {
      tokens = Math.min(CAP, tokens + 1);
      lastRefill = Date.now();
    }
    paintBucket();
  }, 100);
  paintBucket();

  function clearChain() {
    Object.keys(stages).forEach(function (k) {
      stages[k].removeAttribute("data-on");
      stages[k].removeAttribute("data-state");
      stages[k].querySelector(".ms").textContent = "—";
    });
  }
  function line(html) {
    var d = document.createElement("div");
    d.innerHTML = html;
    log.appendChild(d);
    log.scrollTop = log.scrollHeight;
    while (log.children.length > 40) log.removeChild(log.firstChild);
  }
  var sleep = function (ms) { return new Promise(function (r) { setTimeout(r, reduced ? 0 : ms); }); };
  function light(k, ms, note) {
    var s = stages[k];
    if (!s) return;
    s.setAttribute("data-on", "1");
    s.querySelector(".ms").textContent = ms.toFixed(1) + "ms";
    if (note) s.querySelector(".note").textContent = note;
  }

  async function send() {
    if (running) { queue++; return; }
    running = true;
    clearChain();
    var id = String(++seq).padStart(3, "0"), t = 0;
    verdict.textContent = "in flight";

    await sleep(70); t += 0.2; light("edge", 0.2);
    await sleep(70);
    if (tokens < 1) {
      var s = stages.rl;
      s.setAttribute("data-on", "1");
      s.setAttribute("data-state", "halt");
      s.querySelector(".ms").textContent = "0.4ms";
      rlNote.textContent = "0 / " + CAP;
      verdict.textContent = "429 rejected";
      line('<b>429</b> <span class="c2">req-' + id + " · bucket empty · retry-after 1s</span>");
      running = false;
      if (queue > 0) { queue--; setTimeout(send, 140); }
      return;
    }
    tokens -= 1;
    if (tokens === CAP - 1) lastRefill = Date.now();
    paintBucket();
    t += 0.6; light("rl", 0.6);

    await sleep(70); t += 0.9; light("auth", 0.9);
    await sleep(70); t += 1.4; light("rbac", 1.4);
    await sleep(70); t += 0.3; light("handler", 0.3);
    await sleep(70); t += 0.5; light("service", 0.5);

    var hit = Date.now() < cacheUntil;
    await sleep(70);
    if (hit) {
      t += 0.4; light("cache", 0.4, "HIT");
      cacheNote.textContent = "HIT";
      verdict.textContent = "200 · cache";
      line('<span class="c2">200</span> req-' + id + ' <span class="c2">GET /posts</span> ' +
        t.toFixed(1) + 'ms <span class="c2">· redis hit</span>');
    } else {
      t += 0.7; light("cache", 0.7, "MISS");
      cacheNote.textContent = "MISS";
      await sleep(70); t += 0.6; light("repo", 0.6);
      var q = 11 + Math.random() * 8;
      await sleep(110); t += q; light("pg", q);
      cacheUntil = Date.now() + TTL;
      verdict.textContent = "200 · from pg";
      line('<span class="c2">200</span> req-' + id + ' <span class="c2">GET /posts</span> ' +
        t.toFixed(1) + 'ms <span class="c2">· pg → set ttl ' + (TTL / 1000) + "s</span>");
    }
    running = false;
    if (queue > 0) { queue--; setTimeout(send, 140); }
  }

  $("send").addEventListener("click", send);
  $("spam").addEventListener("click", function () {
    for (var i = 0; i < 8; i++) setTimeout(send, i * 130);
  });
  $("reset").addEventListener("click", function () {
    tokens = CAP; cacheUntil = 0; queue = 0; seq = 0;
    clearChain(); paintBucket();
    cacheNote.textContent = "cold";
    verdict.textContent = "idle";
    log.innerHTML = '<div class="c2">// access log</div>';
  });
})();
