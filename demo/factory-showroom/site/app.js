/* Factory Showroom - rendering and assembly line animation.
   Vanilla JS, no dependencies, no network requests. */
(function () {
  'use strict';

  var C = window.SHOWROOM_CONTENT;
  if (!C) { return; }

  var reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  /* ---------- helpers ---------- */
  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) { node.className = className; }
    if (text !== undefined && text !== null) { node.textContent = text; }
    return node;
  }

  function pick(path) {
    return path.split('.').reduce(function (acc, key) {
      return acc == null ? undefined : acc[key];
    }, C);
  }

  /* ---------- simple data bindings ---------- */
  Array.prototype.forEach.call(document.querySelectorAll('[data-bind]'), function (node) {
    var value = pick(node.getAttribute('data-bind'));
    if (typeof value === 'string' || typeof value === 'number') {
      node.textContent = String(value);
    }
  });
  document.title = C.meta.title + ' - ' + C.person.name + ', ' + C.person.role;

  /* ---------- stats ---------- */
  var statsHost = document.getElementById('stats');
  C.person.stats.forEach(function (stat) {
    var cell = el('div');
    cell.appendChild(el('dt', null, stat.label));
    var dd = el('dd', null, '0' + (stat.suffix || ''));
    dd.setAttribute('data-target', String(stat.value));
    dd.setAttribute('data-suffix', stat.suffix || '');
    cell.appendChild(dd);
    statsHost.appendChild(cell);
  });

  /* ---------- stages ---------- */
  var stagesHost = document.getElementById('stages');
  var lineWidget = document.getElementById('line-widget');
  var stageNodes = C.stages.map(function (stage, index) {
    var li = el('li', 'stage');
    li.id = 'stage-' + stage.id;
    li.setAttribute('data-index', String(index));
    li.appendChild(el('span', 'stage-code', stage.code));
    li.appendChild(el('span', 'stage-name', stage.name));
    li.appendChild(el('span', 'stage-caption', stage.caption));
    stagesHost.appendChild(li);
    return li;
  });

  /* ---------- skills ---------- */
  var skillHost = document.getElementById('skill-grid');
  C.skillGroups.forEach(function (group) {
    var card = el('article', 'skill-card reveal');
    card.appendChild(el('h3', null, group.name));
    var list = el('ul');
    group.items.forEach(function (item) {
      var row = el('li', 'skill-row');
      var top = el('div', 'skill-top');
      top.appendChild(el('span', null, item.label));
      top.appendChild(el('span', null, item.level + ' / 100'));
      row.appendChild(top);
      var meter = el('div', 'meter');
      meter.setAttribute('role', 'img');
      meter.setAttribute('aria-label', item.label + ': ' + item.level + ' out of 100');
      var fill = el('i');
      fill.setAttribute('data-level', String(item.level));
      meter.appendChild(fill);
      row.appendChild(meter);
      list.appendChild(row);
    });
    card.appendChild(list);
    skillHost.appendChild(card);
  });

  /* ---------- experience ---------- */
  var timelineHost = document.getElementById('timeline');
  C.experience.forEach(function (job) {
    var li = el('li', 'tl-item reveal');
    var head = el('div', 'tl-head');
    head.appendChild(el('span', 'tl-period', job.period));
    head.appendChild(el('span', 'tl-stage', 'Stage: ' + job.stage));
    li.appendChild(head);
    li.appendChild(el('h3', null, job.role));
    li.appendChild(el('p', 'tl-org', job.org));
    var notes = el('ul', 'tl-notes');
    job.notes.forEach(function (note) { notes.appendChild(el('li', null, note)); });
    li.appendChild(notes);
    timelineHost.appendChild(li);
  });

  /* ---------- projects ---------- */
  var projectHost = document.getElementById('project-grid');
  C.projects.forEach(function (project) {
    var card = el('article', 'card reveal');
    card.appendChild(el('p', 'card-kind', project.kind));
    card.appendChild(el('h3', null, project.name));
    card.appendChild(el('p', null, project.summary));
    var tags = el('ul', 'tags');
    project.tags.forEach(function (tag) { tags.appendChild(el('li', 'tag', tag)); });
    card.appendChild(tags);
    projectHost.appendChild(card);
  });

  /* ---------- contact ---------- */
  var contactHost = document.getElementById('contact-list');
  C.contact.channels.forEach(function (channel) {
    var cell = el('div');
    cell.appendChild(el('dt', null, channel.label));
    var dd = el('dd');
    if (channel.href) {
      var link = el('a', null, channel.value);
      link.href = channel.href;
      link.rel = 'nofollow noopener';
      dd.appendChild(link);
    } else {
      dd.textContent = channel.value;
    }
    cell.appendChild(dd);
    contactHost.appendChild(cell);
  });

  /* ---------- scroll reveal, meters, counters ---------- */
  function runMeters(root) {
    Array.prototype.forEach.call(root.querySelectorAll('.meter > i'), function (fill) {
      fill.style.width = fill.getAttribute('data-level') + '%';
    });
  }

  function countUp(node) {
    var target = parseFloat(node.getAttribute('data-target')) || 0;
    var suffix = node.getAttribute('data-suffix') || '';
    if (reduceMotion.matches) {
      node.textContent = target + suffix;
      return;
    }
    var start = performance.now();
    var duration = 1100;
    function tick(now) {
      var t = Math.min(1, (now - start) / duration);
      var eased = 1 - Math.pow(1 - t, 3);
      node.textContent = Math.round(target * eased) + suffix;
      if (t < 1) { requestAnimationFrame(tick); }
    }
    requestAnimationFrame(tick);
  }

  var revealTargets = document.querySelectorAll('.reveal');
  if ('IntersectionObserver' in window) {
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) { return; }
        entry.target.classList.add('is-visible');
        runMeters(entry.target);
        observer.unobserve(entry.target);
      });
    }, { threshold: 0.2, rootMargin: '0px 0px -40px 0px' });
    Array.prototype.forEach.call(revealTargets, function (node) { observer.observe(node); });

    var statObserver = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) { return; }
        Array.prototype.forEach.call(entry.target.querySelectorAll('dd[data-target]'), countUp);
        statObserver.unobserve(entry.target);
      });
    }, { threshold: 0.4 });
    statObserver.observe(statsHost);
  } else {
    Array.prototype.forEach.call(revealTargets, function (node) { node.classList.add('is-visible'); });
    runMeters(document);
    Array.prototype.forEach.call(statsHost.querySelectorAll('dd[data-target]'), countUp);
  }

  /* ---------- assembly line animation ---------- */
  var car = document.getElementById('car');
  var viewport = lineWidget.querySelector('.line-viewport');
  var readoutStage = document.getElementById('readout-stage');
  var readoutNote = document.getElementById('readout-note');
  var toggle = document.getElementById('line-toggle');
  var hint = document.getElementById('line-hint');

  var TRAVEL = 1900;   /* ms between stages */
  var DWELL = 2200;    /* ms parked at a stage */
  var EXIT = 1600;     /* ms to leave the line and re-enter */

  var positions = [];
  var enterX = 0;
  var exitX = 0;

  function measure() {
    var width = viewport.clientWidth;
    var carWidth = car.offsetWidth;
    var bay = width / C.stages.length;
    positions = C.stages.map(function (_, i) {
      var center = bay * (i + 0.5);
      return Math.max(6, Math.min(width - carWidth - 6, center - carWidth / 2));
    });
    enterX = -carWidth - 20;
    exitX = width + 20;
  }

  var state = { phase: 'dwell', index: 0, t0: 0, paused: false, x: 0 };

  function setStage(index) {
    var stage = C.stages[index];
    lineWidget.setAttribute('data-stage', stage.id);
    readoutStage.textContent = stage.name + ' (' + stage.code + ')';
    readoutNote.textContent = stage.caption;
    stageNodes.forEach(function (node, i) {
      node.classList.toggle('is-active', i === index);
      node.classList.toggle('is-done', i < index);
      if (i === index) {
        node.setAttribute('aria-current', 'step');
      } else {
        node.removeAttribute('aria-current');
      }
    });
  }

  function place(x) {
    state.x = x;
    car.style.transform = 'translateX(' + x + 'px)';
    var center = x + car.offsetWidth / 2;
    lineWidget.style.setProperty('--scan-x', center + 'px');
    lineWidget.style.setProperty('--spot-x', center + 'px');
  }

  function easeInOut(t) {
    return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
  }

  var rafId = null;

  function frame(now) {
    rafId = null;
    if (state.paused) { return; }
    if (!state.t0) { state.t0 = now; }
    var elapsed = now - state.t0;

    if (state.phase === 'dwell') {
      place(positions[state.index]);
      if (elapsed >= DWELL) {
        state.t0 = now;
        state.phase = state.index === C.stages.length - 1 ? 'exit' : 'travel';
      }
    } else if (state.phase === 'travel') {
      var from = positions[state.index];
      var to = positions[state.index + 1];
      var p = Math.min(1, elapsed / TRAVEL);
      place(from + (to - from) * easeInOut(p));
      if (p >= 1) {
        state.index += 1;
        state.phase = 'dwell';
        state.t0 = now;
        setStage(state.index);
      }
    } else if (state.phase === 'exit') {
      var pe = Math.min(1, elapsed / EXIT);
      place(positions[C.stages.length - 1] + (exitX - positions[C.stages.length - 1]) * easeInOut(pe));
      if (pe >= 1) {
        state.phase = 'enter';
        state.t0 = now;
        state.index = 0;
        setStage(0);
        place(enterX);
      }
    } else if (state.phase === 'enter') {
      var pi = Math.min(1, elapsed / EXIT);
      place(enterX + (positions[0] - enterX) * easeInOut(pi));
      if (pi >= 1) {
        state.phase = 'dwell';
        state.t0 = now;
      }
    }

    rafId = requestAnimationFrame(frame);
  }

  function start() {
    if (rafId === null && !state.paused) {
      state.t0 = 0;
      rafId = requestAnimationFrame(frame);
    }
  }

  function stop() {
    if (rafId !== null) {
      cancelAnimationFrame(rafId);
      rafId = null;
    }
  }

  function applyStaticMode() {
    stop();
    state.paused = true;
    state.index = C.stages.length - 1;
    state.phase = 'dwell';
    measure();
    setStage(state.index);
    place(positions[state.index]);
    lineWidget.classList.add('is-paused');
    toggle.hidden = true;
    hint.textContent = 'Animation disabled: reduced motion is enabled in your system settings.';
  }

  function applyMotionMode() {
    toggle.hidden = false;
    hint.textContent = 'Motion respects your reduced-motion setting.';
    lineWidget.classList.remove('is-paused');
    state.paused = false;
    state.phase = 'dwell';
    state.index = 0;
    measure();
    setStage(0);
    place(positions[0]);
    toggle.textContent = 'Pause line';
    toggle.setAttribute('aria-pressed', 'false');
    start();
  }

  toggle.addEventListener('click', function () {
    state.paused = !state.paused;
    toggle.setAttribute('aria-pressed', state.paused ? 'true' : 'false');
    toggle.textContent = state.paused ? 'Resume line' : 'Pause line';
    lineWidget.classList.toggle('is-paused', state.paused);
    if (state.paused) {
      stop();
    } else {
      start();
    }
  });

  var resizeTimer = null;
  window.addEventListener('resize', function () {
    if (resizeTimer) { clearTimeout(resizeTimer); }
    resizeTimer = setTimeout(function () {
      measure();
      if (state.paused || state.phase === 'dwell') { place(positions[state.index]); }
    }, 120);
  });

  document.addEventListener('visibilitychange', function () {
    if (document.hidden) {
      stop();
    } else if (!state.paused && !reduceMotion.matches) {
      start();
    }
  });

  function syncMotionPreference() {
    if (reduceMotion.matches) { applyStaticMode(); } else { applyMotionMode(); }
  }

  if (typeof reduceMotion.addEventListener === 'function') {
    reduceMotion.addEventListener('change', syncMotionPreference);
  } else if (typeof reduceMotion.addListener === 'function') {
    reduceMotion.addListener(syncMotionPreference);
  }

  syncMotionPreference();
})();
