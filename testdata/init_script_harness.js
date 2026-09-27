'use strict';
// Executes the real injected init script against a WhatsApp-Web-shaped DOM.
// Two invariants are asserted:
//   1. no single failure locks the user out of Settings;
//   2. the spreadsheet preview's HTML sanitizer neutralizes attribute-breakout
//      payloads carried in cell values.
//
// Usage: node init_script_harness.js <path-to-init-script.js>
// Exits 0 on pass. Prints a JSON {"skipped": "..."} line and exits 0 when jsdom
// is unavailable, so the Go test can skip instead of failing.
const fs = require('fs');

let JSDOM, VirtualConsole;
try {
  ({ JSDOM, VirtualConsole } = require('jsdom'));
} catch (e) {
  console.log(JSON.stringify({ skipped: 'jsdom is not installed in this environment' }));
  process.exit(0);
}

const scriptPath = process.argv[2];
if (!scriptPath) {
  console.log(JSON.stringify({ skipped: 'no script path given' }));
  process.exit(0);
}
const script = fs.readFileSync(scriptPath, 'utf8');

// Two DOM shapes. The bare one has no header, so the injected toolbar control
// cannot mount and the last-resort button takes over. The header variant looks
// like real WhatsApp Web, which is the shape that exposed the duplicate-gear
// bug: toolbar control present AND rail fallback present simultaneously.
const HEAD_BARE = '<!doctype html><html><head></head><body><div id="app"><div id="side"></div></div></body></html>';
const HEAD_WITH_HEADER =
  '<!doctype html><html><head></head><body><div id="app"><div id="side">' +
  '<header><div></div><div id="wa-header-actions"></div></header>' +
  '</div></div></body></html>';
const BRIDGES = ['getDownloadDirNative', 'openDownloadDirNative', 'sendNativeNotification', 'openExternalLink'];

// jsdom reports 0x0 for every getBoundingClientRect, which would make the real
// visibility check treat every element as hidden. Return a plausible rect for
// elements that are not explicitly hidden, so the script's own isElementVisible
// logic is what decides what the user sees.
function patchLayout(window) {
  window.Element.prototype.getBoundingClientRect = function() {
    const cs = window.getComputedStyle(this);
    const hidden = cs.display === 'none' || cs.visibility === 'hidden' || cs.opacity === '0';
    const size = hidden ? 0 : 40;
    return { x: 0, y: 0, width: size, height: size, top: 0, left: 0, right: size, bottom: size };
  };
}

// The rail fallback is mounted from a requestAnimationFrame callback, so the
// DOM must be sampled after at least one frame. Sampling synchronously right
// after eval made an earlier version of this check pass even when two gears
// were on screen.
function nextFrame(window) {
  return new Promise((resolve) => {
    if (window.requestAnimationFrame) window.requestAnimationFrame(() => resolve());
    else setTimeout(resolve, 20);
  });
}

async function run(transform, envMutate, head) {
  const src = transform ? transform(script) : script;
  const dom = new JSDOM(head || HEAD_BARE, {
    runScripts: 'outside-only',
    pretendToBeVisual: true,
    url: 'https://web.whatsapp.com/',
    virtualConsole: new VirtualConsole(),
  });
  const { window } = dom;
  for (const n of BRIDGES) window[n] = () => Promise.resolve('');
  patchLayout(window);
  if (envMutate) envMutate(window);

  let threw = null;
  try { window.eval(src); } catch (e) { threw = e; }
  // Let the deferred entry-point mounts run before inspecting the DOM.
  await nextFrame(window);
  await nextFrame(window);
  await new Promise((r) => setTimeout(r, 30));

  const doc = window.document;
  const SETTINGS_IDS = ['wa-emergency-settings-btn', 'wa-settings-fallback-btn', 'wa-toolbar-settings-btn'];
  const entryPoints = SETTINGS_IDS.filter((id) => doc.getElementById(id));
  // Ids the user can actually see, judged by the same rules the script itself
  // uses: an element hidden with display:none / visibility:hidden / opacity:0
  // does not count. Presence alone is not enough — the duplicate-gear bug had
  // two elements present AND visible at once.
  const isShown = (el) => {
    if (!el) return false;
    const cs = window.getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden' || cs.opacity === '0') return false;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  };
  const visibleEntryPoints = SETTINGS_IDS.filter((id) => isShown(doc.getElementById(id)));

  window.dispatchEvent(new window.KeyboardEvent('keydown', { key: ',', ctrlKey: true, bubbles: true, cancelable: true }));
  const opened = doc.getElementById('wa-settings-overlay') || doc.getElementById('wa-recovery-overlay');

  return {
    threw: threw ? String(threw).split('\n')[0] : 'no',
    entryPoints,
    visibleEntryPoints,
    opened: opened ? opened.id : null,
    recoverable: (typeof window.__waRecoverable === 'function') ? window.__waRecoverable() : [],
  };
}

const cases = [
  ['baseline', null, null, script],
  ['early module failure', (s) => s.replace('\t\t// Emulate window.chrome',
    '\t\tthrow new Error("injected early failure");\n\t\t// Emulate window.chrome'), null, script],
  ['modal failure', (s) => s.replace('window.showSettingsModal = function() {',
    "window.showSettingsModal = function() { throw new Error('injected modal failure');"), null, script],
  ['localStorage denied', null,
    (w) => Object.defineProperty(w, 'localStorage', {
      get() { throw new Error('SecurityError: access denied'); }, configurable: true,
    }), script],
  // The real WhatsApp Web shape: a header exists, so the in-flow toolbar
  // control mounts. Before the fix the rail fallback mounted alongside it and
  // the user saw two identical gears.
  ['whatsapp header present', null, null, script, HEAD_WITH_HEADER],
  // Pre-DOM injection (WebView2 AddScriptToExecuteOnDocumentCreated) where
  // document.head and document.documentElement are both null at eval time.
  ['null head and docEl', null,
    (w) => {
      let active = true;
      const realHead = w.document.head;
      const realDocEl = w.document.documentElement;
      Object.defineProperty(w.document, 'head', {
        get() { return active ? null : realHead; },
        configurable: true,
      });
      Object.defineProperty(w.document, 'documentElement', {
        get() { return active ? null : realDocEl; },
        configurable: true,
      });
      setTimeout(() => { active = false; }, 20);
    }, script],
];

// The spreadsheet preview builds its table with XLSX.utils.sheet_to_html, which
// escapes cell text but writes the raw value into a data-v attribute. A cell
// whose value contains a double quote closes that attribute and injects markup,
// so the sanitizer wrapping it is a security boundary. Audit it here, in the
// jsdom that is already loaded, instead of paying for a second jsdom startup in
// the Go test.
function checkSpreadsheetSanitizer() {
  const failures = [];
  const start = script.indexOf('function sanitizeSheetHtml(html) {');
  if (start === -1) return ['sanitizeSheetHtml helper is missing from the init script'];
  const end = script.indexOf('\n\t\t}\n', start);
  if (end === -1) return ['sanitizeSheetHtml helper is truncated'];
  const fn = script.slice(start, end + '\n\t\t}'.length);

  const dom = new JSDOM('<!doctype html><html><body></body></html>');
  const sanitize = new Function('document', 'return (' + fn + ')')(dom.window.document);

  const BANNED = 'script,style,img,svg,iframe,frame,object,embed,link,meta,base,form,input,button,textarea,select,audio,video,source,track,math,template';
  const audit = (html) => {
    const d = new JSDOM('<!doctype html><html><body>' + html + '</body></html>');
    const doc = d.window.document;
    let bad = doc.querySelectorAll(BANNED).length;
    doc.querySelectorAll('*').forEach((el) => {
      for (const a of Array.from(el.attributes)) {
        if (/^on/i.test(a.name) || /^(src|href|srcdoc|xlink:href)$/i.test(a.name)) bad++;
      }
    });
    return bad;
  };
  const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  const payloads = [
    '" onmouseover="alert(1)',
    '"><img src=x onerror=alert(1)>',
    '"><iframe src=javascript:alert(1)>',
    '<svg onload=alert(1)>',
  ];
  for (const p of payloads) {
    const cell = '<table id="wa-xlsx-table"><tr><td data-t="s" data-v="' + p + '" id="A1">' + esc(p) + '</td></tr></table>';
    const bad = audit(sanitize(cell));
    if (bad) failures.push(`cell ${JSON.stringify(p)} leaves ${bad} dangerous node(s)/attribute(s)`);
    else console.log(`GREEN  ${'spreadsheet cell escaped'.padEnd(22)} payload=${JSON.stringify(p)}`);
  }

  const ok = sanitize('<table id="wa-xlsx-table"><tr><td id="A1">Revenue</td><td id="B1">42</td></tr></table>');
  if (ok.indexOf('Revenue') === -1 || ok.indexOf('42') === -1) failures.push('sanitizer dropped legitimate cell text');
  if (ok.indexOf('id="wa-xlsx-table"') === -1) failures.push('sanitizer dropped the table id');
  return failures;
}

// Drag & drop regression (reported on macOS: "drag n drop file, gambar dll
// masih belum bisa"). A dropped file must reach WhatsApp's composer even when
// an unrelated dialog is mounted. The probe that decides whether WhatsApp's own
// drop handler worked used to accept any [role="dialog"] as proof of success -
// and WhatsApp keeps dialog containers mounted permanently - so the injection
// fallback never ran and the drop silently did nothing. Live diagnostics showed
// the event arriving with the file intact and then no further action at all.
async function checkDragDrop() {
  const failures = [];
  const html =
    '<!doctype html><html><body><div id="app"><div id="side"></div>' +
    '<div id="main"><div role="dialog" id="decoy-dialog">stray dialog</div>' +
    '<div id="attach-wrap"><input type="file" accept="*"></div></div></div></body></html>';
  const dom = new JSDOM(html, {
    runScripts: 'outside-only',
    pretendToBeVisual: true,
    url: 'https://web.whatsapp.com/',
    virtualConsole: new VirtualConsole(),
  });
  const { window } = dom;
  for (const n of BRIDGES) window[n] = () => Promise.resolve('');
  patchLayout(window);

  const diag = [];
  window.waDiagNative = (kind, detail) => { diag.push(kind + ': ' + detail); return true; };

  // jsdom implements neither DataTransfer nor a writable input.files, so both
  // are stubbed down to what the injected code actually uses.
  window.DataTransfer = class {
    constructor() { this._files = []; this.items = { add: (f) => { this._files.push(f); } }; }
    get files() { return this._files; }
  };
  Object.defineProperty(window.HTMLInputElement.prototype, 'files', {
    get() { return this.__waFiles || []; },
    set(v) { this.__waFiles = v; },
    configurable: true,
  });

  window.eval(script);
  await nextFrame(window);
  await nextFrame(window);

  const input = window.document.querySelector('input[type="file"]');
  let staged = 0;
  input.addEventListener('input', () => { staged++; });
  input.addEventListener('change', () => { staged++; });

  const drop = new window.Event('drop', { bubbles: true, cancelable: true });
  Object.defineProperty(drop, 'dataTransfer', {
    value: {
      types: ['Files'],
      files: [{
        name: 'Laporan.docx',
        type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        size: 2048,
      }],
      dropEffect: 'none',
    },
  });
  window.document.getElementById('main').dispatchEvent(drop);

  // Four 350ms probes run before the fallback injection starts, then the retry
  // loop fills the input; 2.5s is comfortably past both.
  await new Promise((r) => setTimeout(r, 2500));

  const detail = diag.join(' | ') || 'no diagnostics at all';
  if (!diag.some((d) => d.indexOf('drop: received') === 0)) {
    failures.push('the drop event never reached the handler (' + detail + ')');
  }
  if (!diag.some((d) => d.indexOf('drop: injected') === 0)) {
    failures.push('the fallback never injected the dropped file (' + detail + ')');
  }
  if (diag.some((d) => d.indexOf('gave up') !== -1)) {
    failures.push('the retry loop gave up instead of injecting (' + detail + ')');
  }
  const filesOnInput = (input.__waFiles && input.__waFiles.length) || 0;
  if (filesOnInput !== 1) {
    failures.push('the document input did not receive the dropped file (files=' + filesOnInput + ')');
  }
  if (staged === 0) {
    failures.push('no input/change event was dispatched, so WhatsApp would never see the file');
  }
  return failures;
}

// Document-preview loop regression (reported on macOS: "preview pdf dan file2
// lain bermasalah"). Dismissing WhatsApp's own viewer makes it re-create the
// attachment blob, which re-entered the createObjectURL interceptor and
// re-opened the in-app preview - a loop. The guard must suppress that re-entry,
// must still honour a fresh click, and must leave a trace in the in-app report.
async function checkDocPreviewLoopGuard() {
  const failures = [];
  const html =
    '<!doctype html><html><body><div id="app"><div id="side"></div><div id="main">' +
    '<div data-testid="msg-container"><div role="row" data-id="m1" title="Laporan.pdf">' +
    '<span id="doc-link">Laporan.pdf</span></div></div>' +
    '<div id="attach-wrap"><input type="file" accept="*"></div>' +
    '</div></div></body></html>';
  const dom = new JSDOM(html, {
    runScripts: 'outside-only',
    pretendToBeVisual: true,
    url: 'https://web.whatsapp.com/',
    virtualConsole: new VirtualConsole(),
  });
  const { window } = dom;
  for (const n of BRIDGES) window[n] = () => Promise.resolve('');
  patchLayout(window);

  const diag = [];
  window.waDiagNative = (kind, detail) => { diag.push(kind + ': ' + detail); return true; };
  // jsdom has no createObjectURL, and the interceptor calls the original
  // *outside* its try block, so the stub must exist before the script runs.
  let blobSeq = 0;
  window.URL.createObjectURL = () => 'blob:jsdom/' + (++blobSeq);
  window.URL.revokeObjectURL = () => {};
  // jsdom does not implement innerText either; the name extractor reads it.
  Object.defineProperty(window.document.getElementById('doc-link'), 'innerText', {
    value: 'Laporan.pdf', configurable: true,
  });

  let reportUrl = '';
  window.openExternalLink = (u) => { reportUrl = u; };

  window.eval(script);
  await nextFrame(window);
  await nextFrame(window);

  const doc = window.document;
  const overlay = () => doc.getElementById('wa-doc-modal-overlay');
  const blob = new window.Blob([new Uint8Array([37, 80, 68, 70])], { type: 'application/pdf' });
  const clickDoc = () => doc.getElementById('doc-link')
    .dispatchEvent(new window.MouseEvent('click', { bubbles: true, cancelable: true }));
  const settle = () => new Promise((r) => setTimeout(r, 150));

  // 1. A user click, then WhatsApp handing over the attachment blob, opens the
  //    in-app preview.
  clickDoc();
  window.URL.createObjectURL(blob);
  await settle();
  if (!overlay()) {
    failures.push('the first preview never opened (diag: ' + (diag.join(' | ') || 'none') + ')');
  }

  // 2. The re-created blob must not reopen it. This is the reported loop.
  if (overlay()) overlay().remove();
  const before = diag.length;
  window.URL.createObjectURL(blob);
  await settle();
  if (overlay()) failures.push('the same blob reopened the preview: the loop guard did not hold');
  if (!diag.slice(before).some((d) => d.indexOf('doc: loop guard') === 0)) {
    failures.push('the guard did not report itself (diag: ' + (diag.slice(before).join(' | ') || 'none') + ')');
  }

  // 2b. A suppression is a decision, not an error, so it has to travel with the
  //     in-app report - otherwise "the preview did nothing" stays unexplained.
  if (typeof window.openIssueReporter === 'function') {
    window.openIssueReporter();
    const body = reportUrl ? decodeURIComponent(reportUrl.split('&body=')[1] || '') : '';
    if (body.indexOf('Feature notes:') === -1 || body.indexOf('suppressed duplicate re-open of Laporan.pdf') === -1) {
      failures.push('the suppression is missing from the in-app report body');
    }
  } else {
    failures.push('openIssueReporter is not installed, so the suppression cannot be reported');
  }

  // 3. A fresh click is new intent and must be honoured again.
  clickDoc();
  window.URL.createObjectURL(blob);
  await settle();
  if (!overlay()) failures.push('a fresh user click was refused by the loop guard');

  return failures;
}

// Capture path (issue #57: "the other person can't hear our voice clearly, it's
// broken like a robot"). The probe exists to gather evidence, so it must be
// invisible: same receiver, same arguments, same return value, and a
// synchronous engine failure must still reach the caller.
async function checkCaptureDiagnostics() {
  const failures = [];
  const dom = new JSDOM(HEAD_BARE, {
    runScripts: 'outside-only',
    pretendToBeVisual: true,
    url: 'https://web.whatsapp.com/',
    virtualConsole: new VirtualConsole(),
  });
  const { window } = dom;
  for (const n of BRIDGES) window[n] = () => Promise.resolve('');
  patchLayout(window);

  const diag = [];
  window.waDiagNative = (kind, detail) => { diag.push(kind + ': ' + detail); return true; };

  const track = {
    label: 'MacBook Pro Microphone',
    getSettings: () => ({
      channelCount: 1, sampleRate: 48000, echoCancellation: true,
      noiseSuppression: true, autoGainControl: true,
    }),
  };
  const stream = { getAudioTracks: () => [track] };
  const sentinel = Promise.resolve(stream);
  const boom = new Error('NotAllowedError');
  const calls = [];
  const engine = {
    getUserMedia: function() {
      calls.push({ constraints: arguments[0], receiver: this });
      if (calls.length > 1) throw boom;
      return sentinel;
    },
  };
  Object.defineProperty(window.navigator, 'mediaDevices', { value: engine, configurable: true });

  window.eval(script);
  await nextFrame(window);

  const constraints = { audio: { channelCount: 1, echoCancellation: false }, video: true };
  const returned = engine.getUserMedia(constraints);
  await new Promise((r) => setTimeout(r, 40));

  if (calls.length !== 1) failures.push('the probe did not pass exactly one call through (got ' + calls.length + ')');
  if (calls.length && calls[0].constraints !== constraints) {
    failures.push('the constraints object was copied or replaced instead of passed through');
  }
  if (calls.length && calls[0].receiver !== engine) failures.push('the engine call lost its receiver');
  if (returned !== sentinel) failures.push('the probe did not return the engine result unchanged');
  if (!diag.some((d) => d.indexOf('mic: getUserMedia constraints:') === 0)) {
    failures.push('the requested constraints were not recorded (diag: ' + (diag.join(' | ') || 'none') + ')');
  }
  if (!diag.some((d) => d.indexOf('mic: granted:') === 0 && d.indexOf('ch=1') !== -1)) {
    failures.push('the granted track settings were not recorded');
  }

  let threw = null;
  try { engine.getUserMedia(constraints); } catch (e) { threw = e; }
  if (threw !== boom) failures.push('a synchronous engine failure was swallowed instead of propagating');

  return failures;
}

async function main() {
  let failures = 0;
  for (const [label, transform, envMutate, , head] of cases) {
    const r = await run(transform, envMutate, head);
    const ok = r.entryPoints.length > 0 && !!r.opened;
    if (!ok) failures++;
    console.log(`${ok ? 'GREEN' : 'RED  '}  ${label.padEnd(22)} entry=${r.entryPoints.join(',') || 'NONE'} opened=${r.opened || 'NOTHING'} uncaught=${r.threw} recoverable=[${(r.recoverable || []).join('; ')}]`);

    const expectedOverlay = (label.includes('failure')) ? 'wa-recovery-overlay' : 'wa-settings-overlay';
    const overlayOk = r.opened === expectedOverlay;
    if (!overlayOk) {
      failures++;
      console.log(`RED    ${(label + ' overlay').padEnd(22)} got=${r.opened} (expected ${expectedOverlay})`);
    }

    // Regression: a settings control hidden with display:none is fine, but two
    // simultaneously visible gears are not — that is the duplicate button users
    // hit when the header control and the rail fallback were both on screen.
    const visible = r.visibleEntryPoints || [];
    if (visible.length > 1) {
      failures++;
      console.log(`RED    ${'duplicate settings gear'.padEnd(22)} visible=${visible.join(',')} (expected at most one)`);
    } else {
      console.log(`GREEN  ${'single settings gear'.padEnd(22)} visible=${visible.join(',') || 'NONE'}`);
    }
  }

  for (const reason of checkSpreadsheetSanitizer()) {
    failures++;
    console.log(`RED    ${'spreadsheet sanitizer'.padEnd(22)} ${reason}`);
  }

  const dndFailures = await checkDragDrop();
  if (dndFailures.length === 0) {
    console.log(`GREEN  ${'drag & drop injection'.padEnd(22)} file reaches the composer despite a stray dialog`);
  }
  for (const reason of dndFailures) {
    failures++;
    console.log(`RED    ${'drag & drop injection'.padEnd(22)} ${reason}`);
  }

  const loopFailures = await checkDocPreviewLoopGuard();
  if (loopFailures.length === 0) {
    console.log(`GREEN  ${'document preview loop'.padEnd(22)} duplicate re-open suppressed, fresh click honoured, reason reported`);
  }
  for (const reason of loopFailures) {
    failures++;
    console.log(`RED    ${'document preview loop'.padEnd(22)} ${reason}`);
  }

  const micFailures = await checkCaptureDiagnostics();
  if (micFailures.length === 0) {
    console.log(`GREEN  ${'capture probe'.padEnd(22)} constraints and track settings recorded, call path untouched`);
  }
  for (const reason of micFailures) {
    failures++;
    console.log(`RED    ${'capture probe'.padEnd(22)} ${reason}`);
  }

  if (failures) {
    console.log(`\nFAIL: ${failures} harness invariant(s) violated: Settings reachability, spreadsheet sanitizing, drag & drop, document preview looping, or the capture probe.`);
    process.exit(1);
  }
  console.log('\nPASS: Settings stays reachable under injected failures, the spreadsheet sanitizer neutralizes cell markup, a dropped file reaches the composer, the document preview cannot loop, and the capture probe is invisible.');
  process.exit(0);
}

main().catch((e) => {
  console.error('harness error:', e && e.stack ? e.stack : e);
  process.exit(1);
});
