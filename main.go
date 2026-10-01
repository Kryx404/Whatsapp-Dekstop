package main

import (
	"runtime"
	"strings"
)

const (
	windowWidth  = 1200
	windowHeight = 800
)

func getInitScript(ua string) string {
	clientPlatform := "macOS"
	clientPlatformVersion := "15.0.0"
	clientArch := "x86"
	if runtime.GOARCH == "arm64" {
		clientArch = "arm"
	}
	if runtime.GOOS == "windows" {
		clientPlatform = "Windows"
		clientPlatformVersion = "10.0.0"
		clientArch = "x86"
	}

	chromeMajor := "150"
	chromeFull := "150.0.0.0"
	if idx := strings.Index(ua, "Chrome/"); idx != -1 {
		rest := ua[idx+len("Chrome/"):]
		if end := strings.IndexByte(rest, ' '); end != -1 {
			chromeFull = rest[:end]
		} else {
			chromeFull = rest
		}
		if dot := strings.IndexByte(chromeFull, '.'); dot != -1 {
			chromeMajor = chromeFull[:dot]
		} else {
			chromeMajor = chromeFull
		}
	}

	script := `
		// --- Safe storage -------------------------------------------------
		// localStorage throws (SecurityError) instead of returning null when
		// the engine denies storage: WebView2 does this in InPrivate mode and
		// when the profile directory is read-only. Unguarded calls used to
		// abort the whole injected script, which took the Settings control and
		// every keyboard shortcut down with it. All reads/writes go through
		// here so a denied store degrades to "preference not persisted".
		var waMemoryStore = {};
		function storageGet(key) {
			try {
				var v = localStorage.getItem(key);
				if (v !== null) return v;
			} catch (e) {
				waNoteRecoverable('storage-get', e);
			}
			return Object.prototype.hasOwnProperty.call(waMemoryStore, key) ? waMemoryStore[key] : null;
		}
		function storageSet(key, value) {
			waMemoryStore[key] = String(value);
			try {
				localStorage.setItem(key, String(value));
			} catch (e) {
				waNoteRecoverable('storage-set', e);
			}
		}
		function storageRemove(key) {
			delete waMemoryStore[key];
			try {
				localStorage.removeItem(key);
			} catch (e) {
				waNoteRecoverable('storage-remove', e);
			}
		}

		// --- Page-side diagnostics ---------------------------------------
		// Reports what the injected features actually did (drag & drop, document
		// preview, account switch) to the native debug log. The native side
		// ignores it unless diagnostics are enabled, so this stays a cheap call
		// in normal use and needs no round trip.
		function waDiag(kind, detail) {
			try {
				if (window.waDiagNative) window.waDiagNative(String(kind), String(detail == null ? '' : detail).slice(0, 300));
			} catch (e) {}
		}

		var origConsoleError = console.error;
		console.error = function() {
			var args = Array.prototype.slice.call(arguments).map(function(a) {
				return (a && a.stack) ? a.stack : String(a);
			}).join(' ');
			waDiag('console-err', args);
			origConsoleError.apply(console, arguments);
		};
		window.addEventListener('error', function(e) {
			waDiag('window-err', (e.message || 'unknown') + ' @ ' + (e.filename || '?') + ':' + (e.lineno || '?'));
		}, true);
		window.addEventListener('unhandledrejection', function(e) {
			var r = e.reason;
			waDiag('unhandled-reject', String((r && (r.stack || r.message)) || r));
		});

		// --- Recoverable-failure log --------------------------------------
		// Non-fatal problems are recorded instead of thrown so one degraded
		// feature never disables the rest of the injected script. Bounded, and
		// surfaced by the in-app diagnostics panel.
		var waRecoverable = [];
		function waNoteRecoverable(where, err) {
			try {
				waRecoverable.push(where + ': ' + String((err && err.message) || err));
				if (waRecoverable.length > 25) waRecoverable.shift();
			} catch (e) {}
		}
		window.__waRecoverable = function() { return waRecoverable.slice(); };

		// Runs a module so that a failure inside it cannot stop later modules.
		// Each IIFE below is independent; without this an early throw (a WebView2
		// API difference, a denied storage read) removes every enhancement
		// defined after it.
		function waRunModule(name, fn) {
			try {
				return fn();
			} catch (e) {
				waNoteRecoverable(name, e);
				return undefined;
			}
		}

		// Go-side platform constant — more reliable than navigator.platform which is
		// deprecated in Chrome 93+ and may return "" in newer WebView2 builds.
		var __WA_GOOS = '` + runtime.GOOS + `';

	try {
		var isChrome = '` + ua + `'.indexOf('Chrome/') !== -1;

		// UserAgent and platform override
		Object.defineProperty(navigator, 'userAgent', {
			get: () => '` + ua + `',
			configurable: true
		});
		Object.defineProperty(navigator, 'appVersion', {
			get: () => '` + ua + `',
			configurable: true
		});
		Object.defineProperty(navigator, 'vendor', {
			get: () => isChrome ? 'Google Inc.' : 'Apple Computer, Inc.',
			configurable: true
		});
		Object.defineProperty(navigator, 'vendorSub', {
			get: () => '',
			configurable: true
		});
		Object.defineProperty(navigator, 'productSub', {
			get: () => '20030107',
			configurable: true
		});

		if (isChrome) {
			// Emulate window.chrome
			if (!window.chrome) {
				window.chrome = {};
			}
			if (!window.chrome.app) {
				window.chrome.app = {
					isInstalled: false,
					InstallState: { DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed' },
					RunningState: { CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running' },
					getDetails: function() { return null; },
					getIsInstalled: function() { return false; },
					installState: function() { return 'not_installed'; },
					runningState: function() { return 'cannot_run'; }
				};
			}
			if (!window.chrome.csi) {
				window.chrome.csi = function() {
					return { startE: Date.now(), onloadT: Date.now(), pageT: 1, tran: 15 };
				};
			}
			if (!window.chrome.loadTimes) {
				window.chrome.loadTimes = function() {
					var nowSec = Date.now() / 1000;
					return {
						requestTime: nowSec,
						startLoadTime: nowSec,
						commitLoadTime: nowSec,
						finishDocumentLoadTime: 0,
						finishLoadTime: 0,
						firstPaintTime: 0,
						firstPaintAfterLoadTime: 0,
						navigationType: 'Other',
						wasFetchedViaSpdy: false,
						wasNpnNegotiated: false,
						npnNegotiatedProtocol: '',
						wasAlternateProtocolAvailable: false,
						connectionInfo: 'unknown'
					};
				};
			}
			try {
				delete window.safari;
			} catch (e) {}
			try {
				Object.defineProperty(window, 'safari', {
					get: () => undefined,
					set: () => {},
					configurable: true
				});
			} catch (e) {}
		}

		// WebKit polyfills for APIs Chrome expects during initial sync
		if (typeof window.requestIdleCallback === 'undefined') {
			window.requestIdleCallback = function(cb, options) {
				var start = Date.now();
				return setTimeout(function() {
					cb({
						didTimeout: false,
						timeRemaining: function() { return Math.max(0, 50 - (Date.now() - start)); }
					});
				}, 1);
			};
			window.cancelIdleCallback = function(id) { clearTimeout(id); };
		}
		if (typeof StorageManager !== 'undefined' && StorageManager.prototype && !StorageManager.prototype.estimate) {
			StorageManager.prototype.estimate = function() {
				return Promise.resolve({
					quota: 50 * 1024 * 1024 * 1024,
					usage: 50 * 1024 * 1024,
					usageDetails: { indexedDB: 50 * 1024 * 1024 }
				});
			};
		}
		(function() {
			var createPermissionStatus = function(state, name) {
				var statusObj = {
					state: state || 'granted',
					name: name || 'notifications',
					onchange: null,
					addEventListener: function(type, listener) {
						if (type === 'change') this.onchange = listener;
					},
					removeEventListener: function(type, listener) {
						if (type === 'change' && this.onchange === listener) this.onchange = null;
					},
					dispatchEvent: function() { return true; }
				};
				return statusObj;
			};

			var origQuery = null;
			if (navigator.permissions && typeof navigator.permissions.query === 'function') {
				try {
					origQuery = navigator.permissions.query.bind(navigator.permissions);
				} catch (e) {}
			}

			var customQuery = function(desc) {
				if (desc && (desc.name === 'notifications' || desc.name === 'push')) {
					var permState = (typeof window.isNotificationsEnabled === 'function' && !window.isNotificationsEnabled()) ? 'denied' : 'granted';
					return Promise.resolve(createPermissionStatus(permState, desc.name));
				}
				if (desc && (desc.name === 'persistent-storage' || desc.name === 'storage-access' || desc.name === 'background-sync' || desc.name === 'periodic-background-sync')) {
					return Promise.resolve(createPermissionStatus('granted', desc.name));
				}
				if (origQuery) {
					try {
						return origQuery(desc).catch(function() {
							return Promise.resolve(createPermissionStatus('granted', desc && desc.name));
						});
					} catch (e) {
						return Promise.resolve(createPermissionStatus('granted', desc && desc.name));
					}
				}
				return Promise.resolve(createPermissionStatus('granted', desc && desc.name));
			};

			if (!navigator.permissions) {
				var permObj = { query: customQuery };
				try {
					Object.defineProperty(navigator, 'permissions', {
						get: () => permObj,
						configurable: true
					});
				} catch (e) {
					try { navigator.permissions = permObj; } catch (e2) {}
				}
			} else {
				try {
					navigator.permissions.query = customQuery;
				} catch (e) {
					try {
						Object.defineProperty(navigator.permissions, 'query', {
							value: customQuery,
							writable: true,
							configurable: true
						});
					} catch (e2) {}
				}
				if (typeof Permissions !== 'undefined' && Permissions.prototype) {
					try {
						Permissions.prototype.query = customQuery;
					} catch (e3) {}
				}
			}
		})();
		if (typeof Document !== 'undefined' && Document.prototype && !Document.prototype.exitFullscreen) {
			Document.prototype.exitFullscreen = function() {
				if (this.webkitExitFullscreen) return this.webkitExitFullscreen();
				return Promise.resolve();
			};
		}
		if (window.performance && !window.performance.memory) {
			window.performance.memory = {
				jsHeapSizeLimit: 2172649472,
				totalJSHeapSize: 50331648,
				usedJSHeapSize: 31457280
			};
		}
		if (typeof navigator.deviceMemory === 'undefined') {
			try {
				Object.defineProperty(navigator, 'deviceMemory', {
					get: () => 8,
					configurable: true
				});
			} catch (e) {
				try { navigator.deviceMemory = 8; } catch (e2) {}
			}
		}

		// NOTE (v1.5.9): a <meta> Content-Security-Policy allowlist was tried in
		// v1.5.8 and REVERTED — WhatsApp Web loads its boot bundles from Meta
		// CDN hosts outside any maintainable allowlist, so the policy blocked
		// boot and left the app stuck on the splash screen. Do not re-add a
		// meta CSP without a report-only phase first.

		// WhatsApp's virtualized lists can emit hundreds of DOM mutations while the
		// user scrolls. Our enhancements are non-critical during that gesture, so
		// defer them briefly instead of competing with WebKit's renderer. This is
		// deliberately a shared gate: observers keep their correctness but never
		// create a second rendering workload during fast scrolling.
		var waBackgroundWorkBusyUntil = 0;
		function markBackgroundWorkBusy() {
			waBackgroundWorkBusyUntil = Date.now() + 350;
		}
		window.addEventListener('scroll', markBackgroundWorkBusy, { passive: true, capture: true });
		window.addEventListener('wheel', markBackgroundWorkBusy, { passive: true, capture: true });
		window.addEventListener('touchmove', markBackgroundWorkBusy, { passive: true, capture: true });
		function shouldPauseBackgroundWork() {
			return document.hidden === true || Date.now() < waBackgroundWorkBusyUntil;
		}

		// Emulate navigator.userAgentData (User-Agent Client Hints) on Chromium/Windows
		if (isChrome && !navigator.userAgentData) {
			var uaBrands = [
				{ brand: 'Not;A=Brand', version: '8' },
				{ brand: 'Chromium', version: '` + chromeMajor + `' },
				{ brand: 'Google Chrome', version: '` + chromeMajor + `' }
			];
			var uaFullBrands = [
				{ brand: 'Not;A=Brand', version: '8.0.0.0' },
				{ brand: 'Chromium', version: '` + chromeFull + `' },
				{ brand: 'Google Chrome', version: '` + chromeFull + `' }
			];
			var uaDataObj = {
				brands: uaBrands,
				mobile: false,
				platform: '` + clientPlatform + `',
				toJSON: function() {
					return {
						brands: uaBrands,
						mobile: false,
						platform: '` + clientPlatform + `'
					};
				},
				getHighEntropyValues: function(hints) {
					return Promise.resolve({
						architecture: '` + clientArch + `',
						bitness: '64',
						brands: uaBrands,
						fullVersionList: uaFullBrands,
						mobile: false,
						model: '',
						platform: '` + clientPlatform + `',
						platformVersion: '` + clientPlatformVersion + `',
						uaFullVersion: '` + chromeFull + `',
						wow64: false
					});
				}
			};
			try {
				Object.defineProperty(navigator, 'userAgentData', {
					get: () => uaDataObj,
					configurable: true
				});
			} catch (e) {
				try { navigator.userAgentData = uaDataObj; } catch (e2) {}
			}
		}

		// Keep WKWebView's real PDF capability untouched. Advertising Chrome's
		// PDF plugin makes WhatsApp open a viewer that WKWebView cannot render.

		// Native Notification Polyfill & ServiceWorker Notification Interceptor
		waRunModule('notifications', function() {
			var notificationsEnabled = true;
			var notificationsStateReady = true;
			window.isNotificationsEnabled = function() {
				return notificationsStateReady && notificationsEnabled;
			};
			window.setNotificationsEnabled = function(enabled) {
				enabled = !!enabled;
				if (!window.setNotificationsEnabledNative) {
					notificationsEnabled = enabled;
					notificationsStateReady = true;
					return Promise.resolve(notificationsEnabled);
				}
				return Promise.resolve(window.setNotificationsEnabledNative(enabled)).then(function(saved) {
					notificationsEnabled = !!saved;
					notificationsStateReady = true;
					return notificationsEnabled;
				});
			};
			window.refreshNotificationsEnabled = function() {
				if (!window.getNotificationsEnabledNative) {
					notificationsStateReady = true;
					return Promise.resolve(notificationsEnabled);
				}
				return Promise.resolve(window.getNotificationsEnabledNative()).then(function(saved) {
					notificationsEnabled = !!saved;
					notificationsStateReady = true;
					return notificationsEnabled;
				}).catch(function() {
					notificationsStateReady = true;
					return notificationsEnabled;
				});
			};
			window.refreshNotificationsEnabled();

			var recentNotifications = new Map();
			var recentNotificationsByTitle = new Map();
			var notifSeq = 0;

			function simulateClick(el) {
				if (!el) return;
				try { el.focus(); } catch (e) {}
				['mousedown', 'mouseup', 'click'].forEach(function(evt) {
					try {
						el.dispatchEvent(new MouseEvent(evt, {
							bubbles: true,
							cancelable: true,
							view: window
						}));
					} catch (e) {}
				});
				try { el.click(); } catch (e2) {}
			}

			function openChatByTitle(chatTitle) {
				if (!chatTitle) return false;
				var raw = String(chatTitle).trim();
				if (!raw || raw === 'WhatsApp Desk' || raw === 'WhatsApp' || raw === 'Update Available') return false;

				var targets = [raw];
				if (raw.indexOf(' @ ') !== -1) {
					var atParts = raw.split(' @ ');
					if (atParts[1] && targets.indexOf(atParts[1].trim()) === -1) targets.push(atParts[1].trim());
					if (atParts[0] && targets.indexOf(atParts[0].trim()) === -1) targets.push(atParts[0].trim());
				}
				if (raw.indexOf(': ') !== -1) {
					var colonParts = raw.split(': ');
					if (colonParts[0] && targets.indexOf(colonParts[0].trim()) === -1) targets.push(colonParts[0].trim());
				}

				var side = document.querySelector('#pane-side') ||
					document.querySelector('#side') ||
					document.querySelector('[data-testid="chat-list"]');
				if (!side) return false;

				for (var t = 0; t < targets.length; t++) {
					var name = targets[t].toLowerCase();
					if (!name) continue;

					// 1. Check title attributes on span/div inside side
					var titleEls = side.querySelectorAll('span[title], div[title]');
					for (var i = 0; i < titleEls.length; i++) {
						var attr = (titleEls[i].getAttribute('title') || '').trim().toLowerCase();
						if (attr && (attr === name || attr.indexOf(name) === 0 || name.indexOf(attr) === 0)) {
							var row = (titleEls[i].closest && titleEls[i].closest('[role="row"], [data-testid="cell-frame-container"], div._ak8l')) || titleEls[i];
							simulateClick(row);
							return true;
						}
					}

					// 2. Check inner text of row elements
					var textSpans = side.querySelectorAll('[role="row"] span[dir="auto"], [data-testid="cell-frame-container"] span[dir="auto"], div._ak8l span[dir="auto"]');
					for (var j = 0; j < textSpans.length; j++) {
						var txt = (textSpans[j].textContent || '').trim().toLowerCase();
						if (txt && (txt === name || (name.length > 3 && txt.indexOf(name) === 0))) {
							var r = (textSpans[j].closest && textSpans[j].closest('[role="row"], [data-testid="cell-frame-container"], div._ak8l')) || textSpans[j];
							simulateClick(r);
							return true;
						}
					}

					// 3. Check aria-label on rows
					var rows = side.querySelectorAll('[role="row"], [data-testid="cell-frame-container"]');
					for (var k = 0; k < rows.length; k++) {
						var aria = (rows[k].getAttribute('aria-label') || '').toLowerCase();
						if (aria && aria.indexOf(name) !== -1) {
							simulateClick(rows[k]);
							return true;
						}
					}
				}

				// 4. Fallback search bar in chat list
				try {
					var searchInput = document.querySelector(
						'#side [contenteditable="true"][data-tab="3"], ' +
						'#side [data-testid="chat-list-search"], ' +
						'#side input[type="text"]'
					);
					if (searchInput) {
						searchInput.focus();
						document.execCommand('selectAll', false, null);
						document.execCommand('insertText', false, targets[0]);
						searchInput.dispatchEvent(new Event('input', { bubbles: true }));
						setTimeout(function() {
							var first = side.querySelector('[role="row"], [data-testid="cell-frame-container"]');
							if (first) simulateClick(first);
						}, 300);
						return true;
					}
				} catch (e) {}

				return false;
			}

			window.__waOnNotificationClicked = function(notifId, title) {
				try { window.focus(); } catch (ef) {}
				// If App Lock screen is visible, queue opening until unlocked
				var lockScreen = document.getElementById('wa-app-lock-screen');
				if (lockScreen) {
					window.__waPendingNotificationClick = { notifId: notifId, title: title };
					return;
				}

				var notif = (notifId && recentNotifications.get(notifId)) ||
					(title && recentNotificationsByTitle.get(String(title).trim().toLowerCase())) ||
					null;

				var dispatched = false;
				if (notif) {
					try {
						var ev = new Event('click');
						if (typeof notif.onclick === 'function') {
							notif.onclick.call(notif, ev);
							dispatched = true;
						}
						if (typeof notif.dispatchEvent === 'function') {
							notif.dispatchEvent(ev);
							dispatched = true;
						}
					} catch (e) {
						console.error('Error invoking notification click:', e);
					}
				}

				// In addition, click the chat directly in DOM
				if (title) {
					setTimeout(function() {
						openChatByTitle(title);
					}, dispatched ? 80 : 0);
				}
			};

			function sendChatMessage(text) {
				if (!text) return false;
				var composer = document.querySelector(
					'#main footer div[contenteditable="true"][role="textbox"], ' +
					'#main footer div[contenteditable="true"], ' +
					'#main [data-testid="conversation-compose-box-input"], ' +
					'#main footer [data-tab="10"], ' +
					'#main footer [data-tab="6"]'
				);
				if (!composer) return false;

				composer.focus();
				var inserted = false;
				try {
					inserted = document.execCommand('insertText', false, text);
				} catch (e) {}

				if (!inserted) {
					try {
						composer.textContent = text;
					} catch (e) {}
				}

				try {
					composer.dispatchEvent(new InputEvent('input', {
						bubbles: true,
						cancelable: true,
						inputType: 'insertText',
						data: text
					}));
				} catch (e) {
					try { composer.dispatchEvent(new Event('input', { bubbles: true })); } catch (e2) {}
				}

				setTimeout(function() {
					var sendBtn = document.querySelector(
						'#main footer button[data-testid="compose-btn-send"], ' +
						'#main footer [data-testid="send"], ' +
						'#main footer [data-icon="send"], ' +
						'#main footer span[data-icon="send"]'
					);
					if (sendBtn) {
						simulateClick(sendBtn.closest('button') || sendBtn);
					} else {
						['keydown', 'keypress', 'keyup'].forEach(function(evtType) {
							try {
								composer.dispatchEvent(new KeyboardEvent(evtType, {
									key: 'Enter',
									code: 'Enter',
									keyCode: 13,
									which: 13,
									bubbles: true,
									cancelable: true
								}));
							} catch (e3) {}
						});
					}
				}, 60);

				return true;
			}

			window.__waOnNotificationReply = function(notifId, title, replyText) {
				if (!replyText || !replyText.trim()) return;
				// If App Lock screen is visible, refuse sending without PIN
				var lockScreen = document.getElementById('wa-app-lock-screen');
				if (lockScreen) {
					return;
				}

				var opened = openChatByTitle(title);
				var attempts = 0;
				var maxAttempts = 15;
				function trySend() {
					attempts++;
					var sent = sendChatMessage(replyText);
					if (!sent && attempts < maxAttempts) {
						setTimeout(trySend, 100);
					}
				}
				setTimeout(trySend, opened ? 120 : 300);
			};

			function dispatchNativeNotification(title, options, notifId) {
				if (typeof options === 'string') {
					options = { body: options };
				}
				options = options || {};
				var body = options.body != null ? String(options.body) : '';
				var notifTitle = title != null && String(title).length > 0 ? String(title) : 'WhatsApp Desk';
				if (notificationsStateReady && notificationsEnabled && window.sendNativeNotification) {
					if (notifId) {
						window.sendNativeNotification(notifTitle, body, notifId);
					} else {
						window.sendNativeNotification(notifTitle, body);
					}
				}
				if (typeof window.__waOnNotificationDispatched === 'function') {
					try { window.__waOnNotificationDispatched(); } catch (e) {}
				}
			}

			function WAClassNotification(title, options) {
				if (typeof options === 'string') {
					options = { body: options };
				}
				options = options || {};
				var notifId = (options.tag && String(options.tag).length > 0) ?
					String(options.tag) :
					('wa_notif_' + Date.now() + '_' + (++notifSeq));
				this.id = notifId;
				this.title = title != null ? String(title) : '';
				this.body = options.body != null ? String(options.body) : '';
				this.tag = options.tag != null ? String(options.tag) : '';
				this.icon = options.icon != null ? String(options.icon) : '';
				this.data = options.data != null ? options.data : null;
				this.onclick = null;
				this.onclose = null;
				this.onerror = null;
				this.onshow = null;
				this._listeners = {};

				recentNotifications.set(notifId, this);
				if (this.title) {
					recentNotificationsByTitle.set(this.title.trim().toLowerCase(), this);
				}
				if (recentNotifications.size > 50) {
					var oldestKey = recentNotifications.keys().next().value;
					recentNotifications.delete(oldestKey);
				}

				dispatchNativeNotification(title, options, notifId);

				var self = this;
				setTimeout(function() {
					if (typeof self.onshow === 'function') {
						try { self.onshow(new Event('show')); } catch (e) {}
					}
				}, 0);
			}
			try {
				Object.defineProperty(WAClassNotification, 'permission', {
					get: function() { return 'granted'; },
					configurable: true,
					enumerable: true
				});
			} catch (e) {
				WAClassNotification.permission = 'granted';
			}
			try {
				Object.defineProperty(WAClassNotification.prototype, 'permission', {
					get: function() { return 'granted'; },
					configurable: true,
					enumerable: true
				});
			} catch (e) {}
			WAClassNotification.maxActions = 2;
			WAClassNotification.requestPermission = function(callback) {
				var p = Promise.resolve('granted');
				if (typeof callback === 'function') {
					try { callback('granted'); } catch (e) {}
				}
				return p;
			};
			WAClassNotification.prototype.close = function() {
				if (typeof this.onclose === 'function') {
					try { this.onclose(new Event('close')); } catch (e) {}
				}
			};
			WAClassNotification.prototype.addEventListener = function(type, listener) {
				if (!this._listeners) this._listeners = {};
				if (!this._listeners[type]) this._listeners[type] = [];
				this._listeners[type].push(listener);
				if (type === 'click') this.onclick = listener;
				if (type === 'close') this.onclose = listener;
				if (type === 'error') this.onerror = listener;
				if (type === 'show') this.onshow = listener;
			};
			WAClassNotification.prototype.removeEventListener = function(type, listener) {
				if (this._listeners && this._listeners[type]) {
					var idx = this._listeners[type].indexOf(listener);
					if (idx !== -1) this._listeners[type].splice(idx, 1);
				}
				if (type === 'click' && this.onclick === listener) this.onclick = null;
				if (type === 'close' && this.onclose === listener) this.onclose = null;
				if (type === 'error' && this.onerror === listener) this.onerror = null;
				if (type === 'show' && this.onshow === listener) this.onshow = null;
			};
			WAClassNotification.prototype.dispatchEvent = function(event) {
				var type = event && event.type;
				if (!type) return true;
				if (typeof this['on' + type] === 'function') {
					try { this['on' + type].call(this, event); } catch (e) {}
				}
				if (this._listeners && this._listeners[type]) {
					var list = this._listeners[type].slice();
					for (var i = 0; i < list.length; i++) {
						try { list[i].call(this, event); } catch (e2) {}
					}
				}
				return true;
			};

			try {
				Object.defineProperty(window, 'Notification', {
					value: WAClassNotification,
					writable: true,
					configurable: true,
					enumerable: true
				});
			} catch (e) {
				window.Notification = WAClassNotification;
			}
			try {
				if (typeof Window !== 'undefined' && Window.prototype) {
					Object.defineProperty(Window.prototype, 'Notification', {
						value: WAClassNotification,
						writable: true,
						configurable: true,
						enumerable: true
					});
				}
			} catch (e) {}

			try {
				if (typeof ServiceWorkerRegistration !== 'undefined' && ServiceWorkerRegistration.prototype) {
					ServiceWorkerRegistration.prototype.showNotification = function(title, options) {
						var notifId = (options && options.tag) ? String(options.tag) : ('wa_sw_' + Date.now() + '_' + (++notifSeq));
						if (title) {
							recentNotificationsByTitle.set(String(title).trim().toLowerCase(), {
								title: String(title),
								tag: options && options.tag ? String(options.tag) : '',
								onclick: null,
								dispatchEvent: function() { return true; }
							});
						}
						dispatchNativeNotification(title, options, notifId);
						return Promise.resolve();
					};
					if (!ServiceWorkerRegistration.prototype.getNotifications) {
						ServiceWorkerRegistration.prototype.getNotifications = function() {
							return Promise.resolve([]);
						};
					}
				}
			} catch (e) {}
		});

		// Robust HTML5 Media Autoplay & Inline Playback Support for Status/Stories and Videos
		waRunModule('media-playback', function() {
			if (!window.HTMLMediaElement) return;

			function prepareMedia(el) {
				if (!el || el.__wa_media_ready) return;
				el.__wa_media_ready = true;
				if (el.tagName === 'VIDEO') {
					el.setAttribute('playsinline', '');
					el.setAttribute('webkit-playsinline', '');
					el.setAttribute('x5-playsinline', '');
				}
				if (!el.getAttribute('preload')) {
					el.setAttribute('preload', 'metadata');
				}
			}

			var origPlay = HTMLMediaElement.prototype.play;
			HTMLMediaElement.prototype.play = function() {
				var self = this;
				prepareMedia(self);
				var res = origPlay.apply(this, arguments);
				if (res && typeof res.catch === 'function') {
					return res.catch(function(err) {
						// When WebKit blocks unmuted autoplay, mute the media and retry playback
						if (err && (err.name === 'NotAllowedError' || err.name === 'AbortError')) {
							self.muted = true;
							return origPlay.apply(self);
						}
						return Promise.reject(err);
					});
				}
				return res;
			};

			// Automatically prepare video/audio elements injected into DOM. WhatsApp's
			// virtualized chat list mutates frequently, so queue only newly-added
			// subtrees and process them once per animation frame. Rescanning the entire
			// document on every busy frame makes scrolling unnecessarily expensive.
			if (window.MutationObserver) {
				var mediaScanScheduled = false;
				var pendingMediaRoots = [];
				function queueMediaRoot(node) {
					if (!node || node.nodeType !== 1 || pendingMediaRoots.length >= 24) return;
					try {
						if ((node.matches && node.matches('video, audio')) ||
							(node.querySelector && node.querySelector('video, audio'))) {
							pendingMediaRoots.push(node);
						}
					} catch (e) {}
				}
				function scanForUnpreparedMedia() {
					mediaScanScheduled = false;
					var roots = pendingMediaRoots.splice(0, pendingMediaRoots.length);
					if (shouldPauseBackgroundWork()) return;
					for (var r = 0; r < roots.length; r++) {
						var root = roots[r];
						if (root.matches && root.matches('video, audio')) prepareMedia(root);
						if (!root.querySelectorAll) continue;
						var list = root.querySelectorAll('video, audio');
						for (var l = 0; l < list.length; l++) prepareMedia(list[l]);
					}
				}
				function scheduleMediaScan() {
					if (mediaScanScheduled || pendingMediaRoots.length === 0) return;
					mediaScanScheduled = true;
					requestAnimationFrame(scanForUnpreparedMedia);
				}
				var mediaObserver = new MutationObserver(function(mutations) {
					if (shouldPauseBackgroundWork()) return;
					for (var m = 0; m < mutations.length; m++) {
						var added = mutations[m].addedNodes;
						for (var n = 0; n < added.length; n++) queueMediaRoot(added[n]);
					}
					scheduleMediaScan();
				});
				var targetNode = document.documentElement || document.body || document;
				if (targetNode && targetNode.nodeType) {
					try {
						mediaObserver.observe(targetNode, { childList: true, subtree: true });
						queueMediaRoot(targetNode);
						scheduleMediaScan();
					} catch (e) {}
				} else {
					document.addEventListener('DOMContentLoaded', function() {
						var root = document.body || document.documentElement || document;
						if (root && root.nodeType) {
							try {
								mediaObserver.observe(root, { childList: true, subtree: true });
								queueMediaRoot(root);
								scheduleMediaScan();
							} catch (e) {}
						}
					});
				}
			}
		});

		function isDocumentFileName(name) {
			if (!name) return false;
			var ext = name.toLowerCase();
			return ext.endsWith('.pdf') || ext.endsWith('.doc') || ext.endsWith('.docx') ||
				   ext.endsWith('.xls') || ext.endsWith('.xlsx') || ext.endsWith('.ppt') ||
				   ext.endsWith('.pptx') || ext.endsWith('.txt') || ext.endsWith('.csv') ||
				   ext.endsWith('.rtf');
		}

		function cleanDownloadFilename(name) {
			if (!name) return '';
			try {
				var clean = String(name).split(/[\\/]/).pop().trim();
				clean = clean.replace(/[\u0000-\u001f]/g, '');
				return clean;
			} catch (e) {
				return '';
			}
		}

		// Escape a string for interpolation into innerHTML or an HTML
		// attribute. Filenames and paths here originate from chat content
		// (extractDocumentName) or release metadata, so they must never be
		// concatenated raw: a name like '"><img src=x onerror=...>x.pdf'
		// would otherwise execute in the privileged page context that can
		// reach every native bridge. Valid names render identically.
		function escapeHtml(s) {
			return String(s == null ? '' : s).replace(/[&<>"']/g, function(c) {
				return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
			});
		}

		// XLSX.utils.sheet_to_html escapes cell text but writes the raw value
		// into a data-v attribute, so a cell whose value is '"><img src=x
		// onerror=...>' closes the attribute early and injects live markup --
		// reachable from any spreadsheet sent in a chat. Parse the generated
		// markup inside an inert <template> (its content is a separate document
		// fragment, so images do not load and handlers never fire) and drop
		// every attribute we do not control, leaving cell content as text only.
		function sanitizeSheetHtml(html) {
			var tpl = document.createElement('template');
			tpl.innerHTML = String(html == null ? '' : html);
			// Elements a spreadsheet cell must never be able to introduce. The
			// attribute pass below already strips on* handlers and src/href, but
			// leaving an inert <img>/<iframe> behind would still be a rendering
			// artifact, so remove them outright.
			var banned = tpl.content.querySelectorAll('script,style,img,svg,iframe,frame,object,embed,link,meta,base,form,input,button,textarea,select,audio,video,source,track,math,template');
			for (var b = banned.length - 1; b >= 0; b--) {
				if (banned[b].parentNode) banned[b].parentNode.removeChild(banned[b]);
			}
			var nodes = tpl.content.querySelectorAll('*');
			for (var i = 0; i < nodes.length; i++) {
				var attrs = nodes[i].attributes;
				for (var a = attrs.length - 1; a >= 0; a--) {
					var name = attrs[a].name.toLowerCase();
					if (name === 'id' || name === 'colspan' || name === 'rowspan') continue;
					nodes[i].removeAttribute(attrs[a].name);
				}
			}
			return tpl.innerHTML;
		}

		function isPlaceholderDownloadFilename(name) {
			var clean = cleanDownloadFilename(name).toLowerCase();
			if (!clean) return true;
			var stem = clean.replace(/\.[^.]+$/, '').replace(/\s*\(\d+\)$/, '').trim();
			return ['document', 'download', 'file', 'attachment', 'whatsapp_file', 'whatsapp_media'].indexOf(stem) !== -1;
		}

		function filenameFromContentDisposition(header) {
			if (!header) return '';
			try {
				var encoded = header.match(/filename\*\s*=\s*[^']*''([^;]+)/i);
				if (encoded && encoded[1]) return cleanDownloadFilename(decodeURIComponent(encoded[1].replace(/^\"|\"$/g, '')));
				var plain = header.match(/filename\s*=\s*(?:\"([^\"]+)\"|([^;]+))/i);
				return cleanDownloadFilename(plain ? (plain[1] || plain[2]) : '');
			} catch (e) {
				return '';
			}
		}

		function resolveDownloadFilename(filename, contentDisposition) {
			var cdName = filenameFromContentDisposition(contentDisposition);
			var docName = (typeof isRecentPDFIntent === 'function' && isRecentPDFIntent()) ? lastClickedDocName : '';
			var candidates = [cdName, filename, docName];
			for (var i = 0; i < candidates.length; i++) {
				var candidate = cleanDownloadFilename(candidates[i]);
				if (candidate && !isPlaceholderDownloadFilename(candidate)) return candidate;
			}
			for (var j = 0; j < candidates.length; j++) {
				var fallback = cleanDownloadFilename(candidates[j]);
				if (fallback) return fallback;
			}
			return 'whatsapp_file';
		}

		// Dismiss WhatsApp Web's internal stuck viewer overlay
		function dismissStuckViewer() {
			var attempts = 0;
			// Bounded and gentle on purpose. This used to hammer WhatsApp's
			// viewer for 2.4s (30 rounds of 80ms), clicking close buttons and
			// dispatching synthetic Escape events - which fought the page and
			// helped turn a re-opened document into a preview loop. Eight
			// rounds is plenty for the overlay to mount, and nothing is sent
			// when there is no viewer to dismiss.
			var maxAttempts = 8;
			var dismissTimer = setInterval(function() {
				attempts++;
				if (attempts > maxAttempts) {
					clearInterval(dismissTimer);
					return;
				}
				var viewer = document.querySelector('[data-testid="media-viewer"], [data-animate-media-viewer="true"]');
				if (!viewer) {
					if (attempts > 3) clearInterval(dismissTimer);
					return;
				}
				var closeSelectors = [
					'button[data-testid="x-viewer"]',
					'[data-testid="x-viewer"]',
					'[data-icon="x-viewer"]',
					'[data-icon="x"]',
					'[data-icon="back"]',
					'button[aria-label*="Close" i]',
					'button[aria-label*="Tutup" i]',
					'[role="button"][aria-label*="Close" i]',
					'[role="button"][aria-label*="Tutup" i]',
					'button[title*="Close" i]',
					'button[title*="Tutup" i]',
					'[data-testid="btn-close"]',
					'[data-testid="media-viewer-close"]'
				];
				var closed = false;
				for (var i = 0; i < closeSelectors.length; i++) {
					try {
						var el = viewer.querySelector(closeSelectors[i]) || document.querySelector(closeSelectors[i]);
						if (el) {
							var btn = (el.closest && el.closest('button, [role="button"]')) || el;
							btn.click();
							closed = true;
							break;
						}
					} catch (e) {}
				}
				// Dispatch synthetic Escape tagged so our preview modal ignores it
				var escEvt = new KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', keyCode: 27, which: 27, bubbles: true, cancelable: true });
				escEvt._waViewerDismiss = true;
				try {
					viewer.dispatchEvent(escEvt);
					var app = document.getElementById('app');
					if (app) app.dispatchEvent(escEvt);
				} catch (e) {}

				if (closed || attempts > 6) {
					viewer.style.display = 'none';
					clearInterval(dismissTimer);
				}
			}, 80);
		}
		window.dismissStuckViewer = dismissStuckViewer;

		// Track clicked document filenames with robust regex matching
		var lastClickedDocName = '';
		var lastDocumentIntentAt = 0;
		// Loop guard state for the automatic document preview: the same
		// document is only auto-previewed once per cooldown window, so a
		// re-created blob cannot reopen the viewer in a loop.
		var lastDocPreviewName = '';
		var lastDocPreviewAt = 0;
		var docPreviewCooldownMs = 8000;
		function extractDocumentName(el) {
			if (!el || typeof el.closest !== 'function') return '';
			// NEVER extract document names from inside the media viewer, modal dialogs, or top toolbars
			if (el.closest('[data-testid="media-viewer"]') ||
			    el.closest('#wa-doc-modal-overlay') ||
			    el.closest('[role="toolbar"]') ||
			    el.closest('header')) {
				return '';
			}

			// Only search within a chat message container / row / bubble
			var msgContainer = el.closest('[data-testid*="msg-container"], [role="row"], div[data-id], .message-in, .message-out');
			if (!msgContainer) return '';

			var node = el;
			while (node && node !== msgContainer.parentElement && node !== document.body) {
				var title = node.getAttribute && (node.getAttribute('title') || node.getAttribute('aria-label') || '');
				var titleMatch = title && title.match(/([^\n\r<>]{1,180}\.(pdf|docx?|xlsx?|pptx?|txt|csv|rtf))\b/i);
				if (titleMatch && titleMatch[1]) return titleMatch[1].trim();

				// Check text only on leaf-ish nodes to prevent matching unrelated long container text
				if (!node.children || node.children.length < 5) {
					var text = (node.innerText || node.textContent || '').trim();
					if (text.length > 0 && text.length < 250) {
						var textMatch = text.match(/([^\n\r<>]{1,180}\.(pdf|docx?|xlsx?|pptx?|txt|csv|rtf))\b/i);
						if (textMatch && textMatch[1]) return titleMatch ? titleMatch[1].trim() : textMatch[1].trim();
					}
				}
				if (node === msgContainer) break;
				node = node.parentElement;
			}

			var leafNodes = msgContainer.querySelectorAll('span[title], span, div[title], div');
			for (var li = 0; li < leafNodes.length && li < 25; li++) {
				var leaf = leafNodes[li];
				var lTitle = leaf.getAttribute && (leaf.getAttribute('title') || leaf.getAttribute('aria-label') || '');
				var ltMatch = lTitle && lTitle.match(/([^\n\r<>]{1,180}\.(pdf|docx?|xlsx?|pptx?|txt|csv|rtf))\b/i);
				if (ltMatch && ltMatch[1]) return ltMatch[1].trim();
				if (!leaf.children || leaf.children.length === 0) {
					var lText = (leaf.innerText || leaf.textContent || '').trim();
					if (lText.length > 0 && lText.length < 250) {
						var lm = lText.match(/([^\n\r<>]{1,180}\.(pdf|docx?|xlsx?|pptx?|txt|csv|rtf))\b/i);
						if (lm && lm[1]) return lm[1].trim();
					}
				}
			}
			return '';
		}
		function isRecentPDFIntent() {
			return !!lastClickedDocName && isDocumentFileName(lastClickedDocName) &&
				(Date.now() - lastDocumentIntentAt) < 30000;
		}
		document.addEventListener('click', function(e) {
			var name = extractDocumentName(e.target);
			if (name) {
				lastClickedDocName = name;
				lastDocumentIntentAt = Date.now();
				// An explicit click is fresh intent: clear the loop guard so the
				// same document can be previewed again on purpose.
				lastDocPreviewName = '';
				lastDocPreviewAt = 0;
			} else {
				// User clicked an image, media viewer, video, or non-document element:
				// clear stale document name so image downloads never inherit a PDF name.
				if (e.target && e.target.closest && (e.target.closest('img, video, canvas, [data-testid="media-viewer"], [data-testid*="image"]') || !e.target.closest('[data-testid*="msg-container"]'))) {
					lastClickedDocName = '';
					lastDocumentIntentAt = 0;
				}
			}
		}, true);

		window.closeDocumentViewerAfterNativePreview = function() {
			// The native PDF window is already closed at this point. Only dismiss
			// WhatsApp's own media viewer if it is still present; never send a
			// global Escape because WhatsApp may interpret it as closing the chat.
			var viewer = document.querySelector('[data-testid="media-viewer"]');
			var selectors = [
				'button[data-testid="x-viewer"]', '[data-testid="x-viewer"]',
				'[data-icon="x-viewer"]', '[data-icon="x"]', '[data-icon="back"]',
				'button[aria-label*="Close" i]', 'button[aria-label*="Tutup" i]',
				'[role="button"][aria-label*="Close" i]', '[role="button"][aria-label*="Tutup" i]',
				'button[title*="Close" i]', 'button[title*="Tutup" i]'
			].join(',');
			var candidates = viewer ? viewer.querySelectorAll(selectors) : [];
			var best = null;
			var bestScore = -1;
			for (var i = 0; i < candidates.length; i++) {
				var raw = candidates[i];
				if (raw.closest && raw.closest('#wa-doc-modal-overlay')) continue;
				var control = (raw.closest && raw.closest('button, [role="button"]')) || raw;
				var rect = control.getBoundingClientRect();
				if (rect.width < 8 || rect.height < 8 || rect.bottom <= 0 || rect.right <= 0 ||
					rect.top >= window.innerHeight || rect.left >= window.innerWidth) continue;
				var style = window.getComputedStyle(control);
				if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) continue;
				var score = 0;
				if (rect.top < window.innerHeight * 0.3) score += 4;
				if (rect.left > window.innerWidth * 0.7) score += 4;
				if (control.closest && control.closest('[role="dialog"], [data-testid*="viewer"], header, [role="toolbar"]')) score += 5;
				if (score > bestScore) { best = control; bestScore = score; }
			}

			if (best && bestScore >= 8) {
				best.click();
			}
			lastDocumentIntentAt = 0;
			lastClickedDocName = '';
		};

		// Handle explicit user clicks on WhatsApp Web's Media Viewer ✕ close button
		// Guarantees immediate exit to chat view even if internal viewer state is desynced
		document.addEventListener('click', function(e) {
			var target = e.target;
			if (!target || typeof target.closest !== 'function') return;
			var viewer = target.closest('[data-testid="media-viewer"]');
			if (!viewer) return;

			var isCloseBtn = target.closest([
				'button[data-testid="x-viewer"]',
				'[data-testid="x-viewer"]',
				'[data-icon="x-viewer"]',
				'[data-icon="x"]',
				'[data-icon="back"]',
				'button[aria-label*="Close" i]',
				'button[aria-label*="Tutup" i]',
				'button[title*="Close" i]',
				'button[title*="Tutup" i]',
				'[data-testid="btn-close"]'
			].join(','));

			if (isCloseBtn) {
				lastDocumentIntentAt = 0;
				lastClickedDocName = '';
				setTimeout(function() {
					var activeViewer = document.querySelector('[data-testid="media-viewer"]');
					if (activeViewer) {
						var escEvt = new KeyboardEvent('keydown', {
							key: 'Escape',
							code: 'Escape',
							keyCode: 27,
							which: 27,
							bubbles: true,
							cancelable: true
						});
						document.dispatchEvent(escEvt);
						window.dispatchEvent(escEvt);
					}
				}, 60);
			}
		}, false);

		// Intercept external link clicks to open in default browser
		document.addEventListener('click', function(e) {
			var target = e.target;
			while (target && target !== document.body && target.tagName !== 'A') {
				target = target.parentElement;
			}
			if (target && target.tagName === 'A' && target.href) {
				try {
					var url = new URL(target.href);
					if (!url.hostname.endsWith('whatsapp.com') && !url.hostname.endsWith('whatsapp.net') && (url.protocol === 'http:' || url.protocol === 'https:')) {
						e.preventDefault();
						e.stopPropagation();
						if (window.openExternalLink) {
							window.openExternalLink(target.href);
						}
					}
				} catch(err) {}
			}
		}, true);

		// Drag & Drop file upload to chat (stabilized for macOS and Windows)
		var lastUploadAt = 0;
		var lastExplicitDownloadAt = 0;
		function isRecentUpload() {
			return (Date.now() - lastUploadAt) < 6000;
		}
		function isRecentExplicitDownload() {
			return (Date.now() - lastExplicitDownloadAt) < 6000;
		}

		waRunModule('drag-drop-paste', function() {
			var dragCounter = 0;
			var dropInProgress = false;

			function getDropZone() {
				return document.querySelector('#main') || document.querySelector('[data-testid="conversation-panel"]') || document.querySelector('[data-testid="chat-list"]') || document.body;
			}

			function isFileDrag(e) {
				var dt = e.dataTransfer;
				if (!dt) return false;
				if (dt.types) {
					for (var i = 0; i < dt.types.length; i++) {
						if (dt.types[i] === 'Files') return true;
					}
				}
				return false;
			}

			function isChatDrop(e) {
				var target = e.target;
				if (target && target.closest && target.closest('#wa-settings-modal, #wa-doc-modal-overlay, #wa-onboarding-overlay, [role="dialog"]')) {
					if (e.stopImmediatePropagation) e.stopImmediatePropagation();
					return false;
				}
				return true;
			}

			function handleDragEnter(e) {
				if (!isFileDrag(e) || !isChatDrop(e)) return;
				dragCounter++;
				if (dragCounter === 1) {
					var kinds = '';
					try { kinds = (e.dataTransfer && e.dataTransfer.types) ? Array.prototype.join.call(e.dataTransfer.types, ',') : ''; } catch (err) {}
					waDiag('drag', 'file drag entered (types=' + kinds + ')');
				}
				e.preventDefault();
				// Do NOT stopPropagation — let WhatsApp's own dragenter handlers also fire
				// so its native drop zone activates (needed for document drops)
				var dz = getDropZone();
				if (dz) dz.classList.add('wa-drag-over');
			}

			function handleDragLeave(e) {
				dragCounter--;
				if (dragCounter <= 0) {
					dragCounter = 0;
					var dz = getDropZone();
					if (dz) dz.classList.remove('wa-drag-over');
				}
			}

			function handleDragOver(e) {
				if (!isFileDrag(e) || !isChatDrop(e)) return;
				e.preventDefault();
				// Do NOT stopPropagation — WhatsApp needs dragover to reach #main
				// for its native drop handler to accept the drop event
				e.dataTransfer.dropEffect = 'copy';
			}

			function isMediaFile(file) {
				if (!file) return false;
				var t = (file.type || '').toLowerCase();
				var n = (file.name || '').toLowerCase();
				if (t.startsWith('image/') || t.startsWith('video/')) return true;
				return /\.(jpe?g|png|gif|webp|bmp|svg|ico|heic|heif|mp4|mov|m4v|3gp|mkv|avi|webm)$/i.test(n);
			}

			function areAllMediaFiles(files) {
				if (!files || !files.length) return false;
				for (var i = 0; i < files.length; i++) {
					if (!isMediaFile(files[i])) return false;
				}
				return true;
			}

			function findAttachButton() {
				return document.querySelector(
					'[data-testid="attach-menu-plus"], ' +
					'[data-testid="conversation-clip"], ' +
					'[data-testid="clip"], [data-icon="clip"], ' +
					'[data-testid="plus"], [data-icon="plus"], ' +
					'#main footer [role="button"][aria-label*="Attach" i], ' +
					'#main footer [role="button"][aria-label*="Lampirkan" i], ' +
					'#main footer button[aria-label*="Attach" i], ' +
					'#main footer button[aria-label*="Lampirkan" i], ' +
					'button[aria-label*="Attach" i], button[aria-label*="Lampirkan" i], ' +
					'[role="button"][aria-label*="Attach" i], [role="button"][aria-label*="Lampirkan" i], ' +
					'button[title*="Attach" i], button[title*="Lampirkan" i]'
				);
			}

			function findInputInOrNear(el) {
				if (!el) return null;
				var inp = el.querySelector('input[type="file"]');
				if (inp) return inp;
				var container = el.closest('li, [role="menuitem"], [role="button"], [data-testid*="attach"]');
				if (container) {
					inp = container.querySelector('input[type="file"]');
					if (inp) return inp;
				}
				if (el.parentElement) {
					inp = el.parentElement.querySelector('input[type="file"]');
					if (inp) return inp;
				}
				return null;
			}

			function findMediaInput() {
				var selectors = [
					'li[data-testid*="attach-media"]',
					'li[data-testid*="attach-image"]',
					'li[data-testid*="image"]',
					'[data-testid*="attach-media"]',
					'[data-testid*="attach-image"]',
					'[data-testid="mi-attach-media"]',
					'[data-testid="attach-image"]',
					'[data-icon="attach-image"]',
					'[data-icon="image"]',
					'[aria-label*="Photos & videos" i]',
					'[aria-label*="Foto & video" i]',
					'[aria-label*="Fotos y videos" i]',
					'[aria-label*="Fotos e vídeos" i]',
					'[title*="Photos & videos" i]',
					'[title*="Foto & video" i]'
				];
				for (var s = 0; s < selectors.length; s++) {
					var el = document.querySelector(selectors[s]);
					if (el) {
						var inp = findInputInOrNear(el);
						if (inp) return inp;
					}
				}

				var allInputs = document.querySelectorAll('input[type="file"]');
				for (var i = 0; i < allInputs.length; i++) {
					var input = allInputs[i];
					if (input.closest && input.closest('[data-testid*="sticker"], [aria-label*="sticker" i], [aria-label*="stiker" i]')) {
						continue;
					}
					var accept = (input.getAttribute('accept') || '').toLowerCase();
					if (accept.indexOf('image/png,image/jpeg,image/webp') !== -1 && accept.indexOf('image/*') === -1) {
						continue;
					}
					if (accept.indexOf('image/*') !== -1 || accept.indexOf('video') !== -1) {
						return input;
					}
				}
				return null;
			}

			// Find a hidden document file input that is pre-rendered in the DOM by WhatsApp Web.
			// WhatsApp pre-renders hidden file inputs even before the attach menu is opened.
			// The document input typically has accept="*" or no accept attribute.
			function findDocumentInput() {
				var allInputs = document.querySelectorAll('input[type="file"]');
				for (var i = 0; i < allInputs.length; i++) {
					var input = allInputs[i];

					// Skip sticker inputs
					if (input.closest && input.closest('[data-testid*="sticker"], [aria-label*="sticker" i], [aria-label*="stiker" i]')) {
						continue;
					}

					var accept = (input.getAttribute('accept') || '').toLowerCase().trim();

					// Skip clearly media-only inputs (image/* or video/* without broad acceptance)
					if (accept === 'image/*' || accept === 'video/*') continue;
					if (accept.indexOf('image/*') !== -1 && accept.indexOf('video') !== -1 &&
					    accept.indexOf('pdf') === -1 && accept.indexOf('application') === -1 && accept !== '*') {
						continue;
					}
					if (accept.indexOf('image/png,image/jpeg,image/webp') !== -1 && accept.indexOf('*') === -1) {
						continue;
					}

					// Document inputs:
					//  - accept="*" or accept="*/*" (accept all)
					//  - accept="" or no accept attribute (no restriction)
					//  - accept contains document/application types
					if (accept === '*' || accept === '*/*' || accept === '' ||
					    accept.indexOf('document') !== -1 || accept.indexOf('application') !== -1 ||
					    accept.indexOf('pdf') !== -1) {
						return input;
					}
				}
				return null;
			}

			function setFilesOnInput(fileInput, files) {
				if (!fileInput || !files || files.length === 0) return false;
				try {
					var dt = new DataTransfer();
					for (var i = 0; i < files.length; i++) dt.items.add(files[i]);
					var setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'files');
					if (setter && setter.set) {
						setter.set.call(fileInput, dt.files);
					} else {
						fileInput.files = dt.files;
					}
					if (!fileInput.files || fileInput.files.length !== files.length) {
						fileInput.files = dt.files;
					}
					fileInput.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
					fileInput.dispatchEvent(new Event('change', { bubbles: true, composed: true }));
					return true;
				} catch (err) {
					return false;
				}
			}

			// WhatsApp mounts its file inputs lazily: until the attach menu has
			// been opened there is often no input[type=file] in the DOM at all,
			// so the injection below found nothing and a dropped file did
			// nothing visible. Clicking the attach button forces WhatsApp to
			// render the menu (and with it the inputs) so the retry loop can
			// fill them. The menu item itself is only clicked for media, where
			// it merely mounts the in-app editor; the document entry opens the
			// OS file picker, which must never appear behind the user's back.
			function openAttachMenuForInjection(isMedia) {
				var btn = findAttachButton();
				if (!btn) return false;
				var opened = false;
				try {
					btn.click();
					opened = true;
				} catch (e) {}
				if (!opened) return false;
				if (isMedia) {
					var itemSelectors = [
						'li[data-testid*="attach-media"]',
						'[data-testid*="attach-media"]',
						'[data-icon="attach-image"]',
						'[aria-label*="Photos & videos" i]',
						'[aria-label*="Foto & video" i]'
					];
					for (var i = 0; i < itemSelectors.length; i++) {
						var el = document.querySelector(itemSelectors[i]);
						if (el) {
							var clickable = (el.closest && el.closest('li, [role="button"], button')) || el;
							try { clickable.click(); } catch (e) {}
							break;
						}
					}
				}
				return true;
			}

			// Only used for media injection (documents are handled natively by WhatsApp).
			function injectFiles(files, attempt, isMedia) {
				if (isMedia === undefined) isMedia = areAllMediaFiles(files);

				var targetInput = isMedia ? findMediaInput() : findDocumentInput();
				if (targetInput && setFilesOnInput(targetInput, files)) {
					waDiag('drop', 'injected ' + files.length + ' file(s) as ' + (isMedia ? 'media' : 'document') + ' (attempt ' + attempt + ')');
					dropInProgress = false;
					return true;
				}

				// Give WhatsApp a nudge to mount its inputs, then keep probing.
				if (attempt === 6 || attempt === 18) {
					var opened = openAttachMenuForInjection(isMedia);
					waDiag('drop', 'no ' + (isMedia ? 'media' : 'document') + ' input at attempt ' + attempt + '; attach menu ' + (opened ? 'opened' : 'not found'));
				}

				if (attempt < 40) {
					setTimeout(function() { injectFiles(files, attempt + 1, isMedia); }, 40);
				} else {
					waDiag('drop', 'gave up: no file input appeared after 40 attempts');
					dropInProgress = false;
				}
				return false;
			}

			function clearDragVisualState() {
				dragCounter = 0;
				var dz = getDropZone();
				if (dz) dz.classList.remove('wa-drag-over');
			}

			function handleDrop(e) {
				// Reset the drag visual state on EVERY drop, before any early
				// return. The wa-drag-over class sets pointer-events:none on
				// every element, so a drop that lands on an excluded target
				// (a dialog, the settings modal) or arrives with an empty
				// file list (cloud placeholder files, e.g. OneDrive on
				// Windows) used to leave the class stuck until reload — every
				// click in the app went dead, including selecting a contact
				// from the @mention popup, while typing and Enter kept
				// working.
				clearDragVisualState();
				if (!isFileDrag(e)) { waDiag('drop', 'ignored: no Files in dataTransfer'); return; }
				if (!isChatDrop(e)) { waDiag('drop', 'ignored: excluded target'); return; }
				if (dropInProgress) { waDiag('drop', 'ignored: another drop is in progress'); return; }

				var files = Array.prototype.slice.call((e.dataTransfer && e.dataTransfer.files) || []);
				if (!files || files.length === 0) { waDiag('drop', 'ignored: empty file list'); return; }

				var isMedia = areAllMediaFiles(files);
				var described = files.slice(0, 3).map(function(f) {
					return (f.name || '?') + ' [' + (f.type || '?') + ', ' + (f.size || 0) + 'B]';
				}).join('; ');
				waDiag('drop', 'received ' + files.length + ' file(s) media=' + isMedia + ': ' + described);

				// Prevent browser navigation (navigating to file:// URL)
				e.preventDefault();
				lastUploadAt = Date.now(); // prevent download interceptor from triggering
				// A second drop arriving while this one is still being staged
				// would inject over the first batch.
				dropInProgress = true;

				// Do NOT stopImmediatePropagation so WhatsApp's native drop handler
				// on #main / conversation-panel receives the drop event for BOTH
				// media (photos/videos) and documents (PDF, Office, etc.).
				//
				// The probe that decides whether that native handler worked used
				// to accept any [role="dialog"] as proof. WhatsApp keeps dialog
				// containers mounted permanently, so the probe always concluded
				// "WhatsApp handled it", the injection fallback never ran, and a
				// dropped file silently did nothing. Only a real staging surface
				// counts now, and for documents it must also mention the dropped
				// file name, so an unrelated open dialog cannot masquerade as one.
				var droppedNames = files.map(function(f) { return String(f.name || '').toLowerCase(); }).filter(Boolean);
				function waDropStaged() {
					var editor = document.querySelector(
						'[data-testid="media-editor"], [data-testid="image-editor"], ' +
						'[data-testid="drawer-middle"], [data-testid="document-preview"], ' +
						'[data-animate-modal-popup="true"]'
					);
					if (!editor) return false;
					// The media editor exists only once media has been staged.
					if (isMedia) return true;
					var text = String(editor.textContent || '').toLowerCase();
					for (var i = 0; i < droppedNames.length; i++) {
						if (droppedNames[i] && text.indexOf(droppedNames[i]) !== -1) return true;
					}
					return false;
				}

				var waNativeEditorChecks = 0;
				var waNativeEditorPoll = setInterval(function() {
					waNativeEditorChecks++;
					var staged = waDropStaged();
					if (staged || waNativeEditorChecks >= 4) {
						clearInterval(waNativeEditorPoll);
						waDiag('drop', 'native staging after ' + waNativeEditorChecks + ' probe(s): ' + staged);
						if (staged) {
							dropInProgress = false;
						} else {
							// Cleared by injectFiles when the retry loop settles.
							injectFiles(files, 0, isMedia);
							setTimeout(function() { dropInProgress = false; }, 4000);
						}
					}
				}, 350);
			}

			// File-picker uploads do not pass through the drag/drop handler above.
			// Mark them as uploads as soon as WhatsApp receives the selected files so
			// the document preview hooks below do not mistake the composer blob for a
			// downloaded attachment.
			function handleFileInputChange(e) {
				var input = e && e.target;
				if (!input || input.tagName !== 'INPUT' || input.type !== 'file') return;
				if (input.files && input.files.length > 0) lastUploadAt = Date.now();
			}

			document.addEventListener('dragenter', handleDragEnter, true);
			document.addEventListener('dragleave', handleDragLeave, true);
			document.addEventListener('dragover', handleDragOver, true);
			document.addEventListener('drop', handleDrop, true);
			// Safety nets: a drag that never produces a matching dragleave
			// (cancelled via Esc, source outside the page, or the window
			// losing focus mid-drag) must not leave the class behind.
			document.addEventListener('dragend', clearDragVisualState, true);
			window.addEventListener('blur', clearDragVisualState);
			document.addEventListener('change', handleFileInputChange, true);
		});

		// Helper: Decode base64 dataURI to Uint8Array
		function base64ToUint8Array(dataUri) {
			try {
				var base64 = dataUri.indexOf(';base64,') !== -1 ? dataUri.split(';base64,')[1] : dataUri;
				var binary = atob(base64);
				var bytes = new Uint8Array(binary.length);
				for (var i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
				return bytes;
			} catch (e) {
				return null;
			}
		}

		async function decompressDeflateRaw(compressedData) {
			if (typeof DecompressionStream === 'undefined') return null;
			try {
				var ds = new DecompressionStream('deflate-raw');
				var stream = new Response(compressedData).body.pipeThrough(ds);
				return await new Response(stream).text();
			} catch (e) {
				try {
					var ds2 = new DecompressionStream('deflate');
					var stream2 = new Response(compressedData).body.pipeThrough(ds2);
					return await new Response(stream2).text();
				} catch (e2) {
					return null;
				}
			}
		}

		// Helper: Read a specific file from ZIP payload (e.g. word/document.xml, xl/worksheets/sheet1.xml)
		async function readZipEntryText(uint8Array, targetPath) {
			if (!uint8Array || uint8Array.length < 30) return null;
			try {
				var view = new DataView(uint8Array.buffer, uint8Array.byteOffset, uint8Array.byteLength);
				var offset = 0;
				while (offset < uint8Array.length - 30) {
					if (view.getUint32(offset, true) === 0x04034b50) {
						var compMethod = view.getUint16(offset + 8, true);
						var compSize = view.getUint32(offset + 18, true);
						var nameLen = view.getUint16(offset + 26, true);
						var extraLen = view.getUint16(offset + 28, true);
						var nameBytes = uint8Array.subarray(offset + 30, offset + 30 + nameLen);
						var name = new TextDecoder().decode(nameBytes);
						var dataStart = offset + 30 + nameLen + extraLen;
						var dataEnd = dataStart + compSize;

						if (name.toLowerCase() === targetPath.toLowerCase()) {
							var compressedData = uint8Array.subarray(dataStart, dataEnd);
							if (compMethod === 0) {
								return new TextDecoder().decode(compressedData);
							} else if (compMethod === 8) {
								return await decompressDeflateRaw(compressedData);
							}
						}
						offset = dataEnd > offset ? dataEnd : (offset + 1);
					} else {
						offset++;
					}
				}
			} catch (e) {
				console.warn('Zip read error:', e);
			}
			return null;
		}

		function parsePptxToHtml(slideXmls) {
			if (!slideXmls || !slideXmls.length) return '';
			var html = ['<div style="width:100%;height:100%;overflow-y:auto;padding:24px 16px;box-sizing:border-box;display:flex;flex-direction:column;align-items:center;background:#0c1317;">'];
			for (var i = 0; i < slideXmls.length; i++) {
				var xml = slideXmls[i];
				if (!xml) continue;
				var tMatches = xml.match(/<a:t\b[^>]*>([\s\S]*?)<\/a:t>/g) || [];
				var lines = [];
				for (var t = 0; t < tMatches.length; t++) {
					var rawT = tMatches[t].replace(/<a:t\b[^>]*>|<\/a:t>/g, '');
					rawT = rawT.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').trim();
					if (rawT) lines.push(rawT);
				}
				if (lines.length) {
					html.push('<div style="width:100%;max-width:760px;background:#ffffff;border-radius:8px;box-shadow:0 4px 16px rgba(0,0,0,0.4);padding:28px 32px;box-sizing:border-box;margin-bottom:18px;">');
					html.push('<div style="font-size:11px;font-weight:700;color:#00a884;text-transform:uppercase;margin-bottom:10px;letter-spacing:0.5px;">Slide ' + (i + 1) + '</div>');
					html.push('<h3 style="font-size:17px;font-weight:700;margin:0 0 10px;color:#111b21;">' + lines[0] + '</h3>');
					for (var l = 1; l < lines.length; l++) {
						html.push('<p style="font-size:13px;color:#3b4a54;margin:5px 0;line-height:1.5;">• ' + lines[l] + '</p>');
					}
					html.push('</div>');
				}
			}
			html.push('</div>');
			return html.length > 2 ? html.join('') : '';
		}

		function parseDocxToHtml(xmlStr) {
			if (!xmlStr) return '';
			var pMatches = xmlStr.match(/<w:p\b[\s\S]*?<\/w:p>/g) || [];
			var html = [];
			for (var i = 0; i < pMatches.length; i++) {
				var pStr = pMatches[i];
				var isH1 = /<w:pStyle\b[^>]*w:val="Heading1"/i.test(pStr);
				var isH2 = /<w:pStyle\b[^>]*w:val="Heading2"/i.test(pStr);
				var isH3 = /<w:pStyle\b[^>]*w:val="Heading[3-6]"/i.test(pStr);
				var rMatches = pStr.match(/<w:r\b[\s\S]*?<\/w:r>/g) || [];
				var pText = '';
				for (var j = 0; j < rMatches.length; j++) {
					var rStr = rMatches[j];
					var isBold = /<w:b\b/.test(rStr);
					var isItalic = /<w:i\b/.test(rStr);
					var tMatches = rStr.match(/<w:t\b[^>]*>([\s\S]*?)<\/w:t>/g) || [];
					for (var k = 0; k < tMatches.length; k++) {
						var tVal = tMatches[k].replace(/<w:t\b[^>]*>|<\/w:t>/g, '');
						tVal = tVal.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
						if (isBold) tVal = '<strong>' + tVal + '</strong>';
						if (isItalic) tVal = '<em>' + tVal + '</em>';
						pText += tVal;
					}
				}
				if (pText.trim()) {
					if (isH1) html.push('<h2 style="color:#111b21;margin:18px 0 8px;font-size:18px;font-weight:700;">' + pText + '</h2>');
					else if (isH2) html.push('<h3 style="color:#111b21;margin:14px 0 6px;font-size:16px;font-weight:600;">' + pText + '</h3>');
					else if (isH3) html.push('<h4 style="color:#111b21;margin:12px 0 4px;font-size:14px;font-weight:600;">' + pText + '</h4>');
					else html.push('<p style="color:#222e35;margin:8px 0;line-height:1.65;font-size:13.5px;">' + pText + '</p>');
				}
			}
			return html.join('');
		}

		// Spreadsheet preview (.xlsx, .xls, .csv) is rendered via the bundled SheetJS
		// library (see renderSpreadsheetPreview / showInAppDocModal below), which can
		// read both modern OOXML and legacy binary Excel formats directly from bytes,
		// so a hand-rolled XML/CSV parser is no longer needed here.

		// In-App Document Preview Modal Overlay (PDF, Excel, Word, Text)
		function showInAppDocModal(filename, blobUrl, savedPath, dataUri, ownedBlobUrl) {
			var existing = document.getElementById('wa-doc-modal-overlay');
			if (existing && existing.parentNode) existing.parentNode.removeChild(existing);

			// Immediately dismiss WhatsApp Web's stuck background viewer
			if (window.dismissStuckViewer) window.dismissStuckViewer();

			var ext = (filename && filename.indexOf('.') !== -1 ? filename.split('.').pop() : '').toLowerCase();
			var isPdf = ext === 'pdf';
			var isExcel = ext === 'xlsx' || ext === 'xls' || ext === 'csv';
			var isWord = ext === 'docx' || ext === 'doc' || ext === 'rtf' || ext === 'txt';
			var isPpt = ext === 'pptx' || ext === 'ppt';

			// WKWebView has no reliable built-in renderer for PDF blob URLs.
			// On macOS, render the already-saved file with PDFKit instead.
			if (isPdf && savedPath && window.showPDFPreviewNative) {
				if (window.dismissStuckViewer) window.dismissStuckViewer();
				var opened = false;
				try {
					opened = !!window.showPDFPreviewNative(savedPath);
				} catch (e) {
					waDiag('doc', 'native PDF preview threw: ' + (e && e.message ? e.message : e));
				}
				waDiag('doc', 'native PDF preview for ' + filename + ' -> ' + opened);
				if (opened) {
					if (ownedBlobUrl) {
						try { URL.revokeObjectURL(ownedBlobUrl); } catch (e) {}
					}
					return;
				}
				// PDFKit could not take the file (missing, unreadable, or a
				// rejected path): fall through to the card below so the user
				// still has an explicit way to open or save it. The blob URL is
				// intentionally not revoked here - the card still needs it.
				showFloatingToast('⚠️ Could not render this PDF in-app — open it instead');
			}

			var docIcon = '📄';
			var openBtnText = '📂 Open in System App';
			var docTypeLabel = 'Document';
			if (isPdf) {
				docIcon = '📄';
				openBtnText = '📂 Open in System App';
				docTypeLabel = 'PDF Document';
			} else if (isExcel) {
				docIcon = '📊';
				openBtnText = '📊 Open in Excel / Numbers';
				docTypeLabel = 'Excel Spreadsheet';
			} else if (isWord) {
				docIcon = '📝';
				openBtnText = '📝 Open in Word / Pages';
				docTypeLabel = 'Word Document';
			} else if (isPpt) {
				docIcon = '📽️';
				openBtnText = '📽️ Open in PowerPoint / Keynote';
				docTypeLabel = 'PowerPoint Presentation';
			}

			// The dim below is deliberately flat, with no backdrop blur. Blurring
			// a full-viewport fixed element forces an offscreen compositing pass
			// on every frame, so scrolling the preview inside the modal
			// re-blurred the whole page each frame -- that was the preview
			// feeling heavier than the built-in one. The account-switch veil in
			// this same file avoids that effect for exactly this reason, and the
			// onboarding panel is kept free of it too. A denser dim carries the
			// same visual weight at a fraction of the cost. This comment names
			// the effect in prose on purpose: the literal CSS token must not
			// appear in this function, so the regression guard stays meaningful.
			var overlay = document.createElement('div');
			overlay.id = 'wa-doc-modal-overlay';
			overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.92);z-index:99999999;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:16px;box-sizing:border-box;animation:waFadeIn 0.2s ease;';

			var modal = document.createElement('div');
			modal.style.cssText = 'width:94%;max-width:1020px;height:92%;background:#111b21;border:1px solid rgba(255,255,255,0.14);border-radius:12px;display:flex;flex-direction:column;overflow:hidden;box-shadow:0 8px 24px rgba(0,0,0,0.55);transform:translateZ(0);';

			// Header
			var header = document.createElement('div');
			header.style.cssText = 'display:flex;align-items:center;justify-content:space-between;padding:10px 16px;border-bottom:1px solid rgba(255,255,255,0.08);background:#202c33;flex-shrink:0;';
			header.innerHTML = '' +
				'<div style="display:flex;align-items:center;gap:10px;min-width:0;">' +
				'  <span style="font-size:22px;">' + docIcon + '</span>' +
				'  <div style="min-width:0;">' +
				'    <strong style="font-size:13.5px;color:#e9edef;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;display:block;max-width:420px;" title="' + escapeHtml(filename) + '">' + escapeHtml(filename) + '</strong>' +
				'    <span style="font-size:11px;color:#8696a0;">' + docTypeLabel + ' · Direct Preview</span>' +
				'  </div>' +
				'</div>' +
				'<div style="display:flex;align-items:center;gap:8px;">' +
				'  <button id="wa-btn-open-preview" style="background:#00a884;color:#111b21;border:none;padding:6px 14px;border-radius:6px;font-size:12px;font-weight:600;cursor:pointer;display:flex;align-items:center;gap:4px;box-shadow:0 2px 6px rgba(0,168,132,0.3);">' +
				'    ' + openBtnText +
				'  </button>' +
				'  <button id="wa-btn-folder-doc" style="background:#2a3942;color:#e9edef;border:1px solid rgba(255,255,255,0.1);padding:6px 12px;border-radius:6px;font-size:12px;font-weight:500;cursor:pointer;">' +
				'    📂 Show in Folder' +
				'  </button>' +
				'  <button id="wa-btn-save-doc" style="background:#2a3942;color:#e9edef;border:1px solid rgba(255,255,255,0.1);padding:6px 12px;border-radius:6px;font-size:12px;font-weight:500;cursor:pointer;">' +
				'    💾 Download' +
				'  </button>' +
				'  <button id="wa-btn-close-doc" style="background:transparent;border:none;color:#8696a0;cursor:pointer;font-size:20px;padding:4px 8px;border-radius:6px;line-height:1;">✕</button>' +
				'</div>';
			modal.appendChild(header);

			// Body Container
			var body = document.createElement('div');
			body.style.cssText = 'flex:1;width:100%;height:100%;position:relative;background:#0c1317;overflow:hidden;display:flex;flex-direction:column;align-items:center;justify-content:center;';
			modal.appendChild(body);

			function triggerOpenSystem() {
				if (savedPath && window.showPDFPreviewNative && isPdf) {
					window.showPDFPreviewNative(savedPath);
				} else if (savedPath && window.openFileNative) {
					window.openFileNative(savedPath);
				} else if (window.previewDocumentNative) {
					window.previewDocumentNative(filename, dataUri || blobUrl);
				}
			}

			function renderCardFallback(hint) {
				var displayPath = savedPath || 'Downloads folder';
				body.innerHTML = '' +
					'<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;padding:40px;text-align:center;">' +
					'  <div style="font-size:64px;margin-bottom:16px;">' + docIcon + '</div>' +
					'  <h2 style="color:#e9edef;font-size:18px;font-weight:600;margin:0 0 8px;max-width:540px;word-break:break-all;">' + escapeHtml(filename) + '</h2>' +
					'  <div style="color:#00a884;font-size:12px;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;margin-bottom:12px;">' + docTypeLabel + ' · Saved</div>' +
					'  <p style="color:#8696a0;font-size:13px;max-width:460px;line-height:1.5;margin:0 0 16px;">' +
					(hint || ('The ' + docTypeLabel + ' is saved on your computer. Click below to open it in your default application.')) +
					'  </p>' +
					'  <div style="font-family:monospace;font-size:11px;color:#8696a0;background:rgba(255,255,255,0.06);padding:6px 14px;border-radius:6px;max-width:520px;overflow:hidden;text-overflow:ellipsis;margin-bottom:24px;border:1px solid rgba(255,255,255,0.08);">' + escapeHtml(displayPath) + '</div>' +
					'  <div style="display:flex;gap:12px;align-items:center;">' +
					'    <button id="wa-btn-card-launch" style="background:#00a884;color:#111b21;border:none;padding:10px 24px;border-radius:8px;font-size:13.5px;font-weight:600;cursor:pointer;display:flex;align-items:center;gap:6px;box-shadow:0 4px 12px rgba(0,168,132,0.3);">' +
					openBtnText +
					'    </button>' +
					'    <button id="wa-btn-card-folder" style="background:#2a3942;color:#e9edef;border:1px solid rgba(255,255,255,0.1);padding:10px 20px;border-radius:8px;font-size:13px;font-weight:500;cursor:pointer;">' +
					'📂 Show in Folder' +
					'    </button>' +
					'  </div>' +
					'</div>';
				var cardBtn = document.getElementById('wa-btn-card-launch');
				if (cardBtn) cardBtn.onclick = triggerOpenSystem;
				var folderBtn = document.getElementById('wa-btn-card-folder');
				if (folderBtn) folderBtn.onclick = function() {
					if (window.openDownloadDirNative) window.openDownloadDirNative();
				};
			}

			// Lazy-load SheetJS (xlsx.core.min.js) only when spreadsheet preview is first needed.
			var xlsxLoadPromise = null;
			function ensureXLSXLoaded() {
				// Only a library that exposes XLSX.utils is usable; an empty stub would
				// make every later XLSX.utils call throw, so treat that as "not loaded".
				if (window.XLSX && window.XLSX.utils) return Promise.resolve();
				if (xlsxLoadPromise) return xlsxLoadPromise;
				xlsxLoadPromise = new Promise(function(resolve, reject) {
					// Fetch the bundled SheetJS from the native side
					if (window.loadXLSXLibraryNative) {
						window.loadXLSXLibraryNative().then(function(jsCode) {
							try {
								  eval(jsCode);
								  // If the host page happens to expose CommonJS exports/module,
								  // the SheetJS core build initialises that object instead of a global
								  // and leaves window.XLSX as an empty stub. The direct eval above also
								  // created an eval-scoped XLSX binding, so prefer it in that case.
								  if (typeof XLSX !== 'undefined' && (!window.XLSX || !window.XLSX.utils)) {
								      window.XLSX = XLSX;
								  }
								  if (!window.XLSX || !window.XLSX.utils) {
								      throw new Error('spreadsheet library failed to initialise');
								  }
								  resolve();
							} catch (e) {
								  reject(e);
							}
						}).catch(reject);
					} else {
						reject(new Error('loadXLSXLibraryNative not available'));
					}
				});
				return xlsxLoadPromise;
			}

			// Render a parsed spreadsheet workbook (from the bundled SheetJS library) as an
			// HTML table, with a sheet-switcher tab bar when the workbook has multiple sheets.
			function renderSpreadsheetPreview(workbook, activeSheetName) {
				var sheetNames = (workbook && workbook.SheetNames) || [];
				if (!sheetNames.length) {
					renderCardFallback('This spreadsheet has no readable sheets.');
					return;
				}
				var activeName = (activeSheetName && sheetNames.indexOf(activeSheetName) !== -1) ? activeSheetName : sheetNames[0];
				var worksheet = workbook.Sheets[activeName];
				var tableHtml = sanitizeSheetHtml(XLSX.utils.sheet_to_html(worksheet, { id: 'wa-xlsx-table' }));

				var tabsHtml = '';
				if (sheetNames.length > 1) {
					tabsHtml = '<div id="wa-xlsx-tabs" style="display:flex;gap:4px;padding:8px 12px;background:#202c33;border-bottom:1px solid #2a3942;overflow-x:auto;flex-shrink:0;">';
					for (var si = 0; si < sheetNames.length; si++) {
						var name = sheetNames[si];
						var active = name === activeName;
						var safeName = name.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
						tabsHtml += '<button data-sheet="' + safeName + '" style="padding:5px 12px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;white-space:nowrap;border:1px solid ' + (active ? '#00a884' : '#2a3942') + ';background:' + (active ? '#00a884' : 'transparent') + ';color:' + (active ? '#111b21' : '#8696a0') + ';">' + safeName + '</button>';
					}
					tabsHtml += '</div>';
				}

				var tableStyle = '<style>#wa-xlsx-table{border-collapse:collapse;width:100%;font-family:system-ui,-apple-system,sans-serif;font-size:12px;color:#e9edef;}#wa-xlsx-table td,#wa-xlsx-table th{border:1px solid #2a3942;padding:6px 10px;white-space:nowrap;}#wa-xlsx-table tr:nth-child(even){background:#182229;}#wa-xlsx-table tr:nth-child(odd){background:#111b21;}</style>';

				body.innerHTML = '<div style="width:100%;height:100%;display:flex;flex-direction:column;">' + tabsHtml +
					'<div style="flex:1;overflow:auto;background:#111b21;contain:content;overscroll-behavior:contain;">' + tableStyle + tableHtml + '</div></div>';

				var tabsEl = document.getElementById('wa-xlsx-tabs');
				if (tabsEl) {
					var tabBtns = tabsEl.querySelectorAll('button');
					for (var bi2 = 0; bi2 < tabBtns.length; bi2++) {
						tabBtns[bi2].onclick = function() {
							renderSpreadsheetPreview(workbook, this.getAttribute('data-sheet'));
						};
					}
				}
			}

			// Render content according to file type
			if (isPdf) {
				var pdfSrc = ownedBlobUrl || blobUrl || '';
				if ((!pdfSrc || pdfSrc.indexOf('blob:') !== 0) && dataUri && dataUri.indexOf(';base64,') !== -1) {
					pdfSrc = 'data:application/pdf;base64,' + dataUri.split(';base64,')[1];
				}
				if (pdfSrc) {
					body.innerHTML = '<iframe src="' + pdfSrc + '" style="width:100%;height:100%;border:none;background:#525659;contain:content;" title="' + escapeHtml(filename) + '"></iframe>';
				} else {
					renderCardFallback();
				}
			} else if (ext === 'csv' || ext === 'xlsx' || ext === 'xls') {
				body.innerHTML = '<div style="color:#8696a0;font-size:13px;display:flex;align-items:center;gap:8px;">⏳ Loading spreadsheet preview...</div>';
				var rawXlsxB64 = (dataUri || '').indexOf(';base64,') !== -1 ? dataUri.split(';base64,')[1] : (dataUri || '');
				if (rawXlsxB64) {
					ensureXLSXLoaded().then(function() {
						try {
							// SheetJS auto-detects the real format from the bytes (OOXML zip for
							// .xlsx, binary OLE2/BIFF for legacy .xls, or plain text for .csv), so
							// one code path correctly previews all three, including .xls which the
							// previous hand-rolled parser never actually supported.
							var workbook = XLSX.read(rawXlsxB64, { type: 'base64', cellDates: true });
							renderSpreadsheetPreview(workbook);
						} catch (e) {
							renderCardFallback('Unable to render an in-app preview for this spreadsheet. Click below to open it in your default application.');
						}
					}).catch(function() {
						renderCardFallback('Unable to load spreadsheet library.');
					});
				} else {
					renderCardFallback();
				}
			} else if (ext === 'docx') {
				body.innerHTML = '<div style="color:#8696a0;font-size:13px;display:flex;align-items:center;gap:8px;">⏳ Loading Word preview...</div>';
				var uint8Doc = base64ToUint8Array(dataUri || '');
				if (uint8Doc) {
					var parsePromiseDoc = readZipEntryText(uint8Doc, 'word/document.xml');
					var timeoutPromiseDoc = new Promise(function(resolve) { setTimeout(function() { resolve(null); }, 1500); });
					Promise.race([parsePromiseDoc, timeoutPromiseDoc]).then(function(docXml) {
						if (docXml) {
							var docHtml = parseDocxToHtml(docXml);
							body.innerHTML = '' +
								'<div style="width:100%;height:100%;overflow-y:auto;padding:24px 16px;box-sizing:border-box;display:flex;justify-content:center;contain:content;overscroll-behavior:contain;background:#0c1317;">' +
								'  <div style="width:100%;max-width:760px;background:#ffffff;border-radius:6px;box-shadow:0 4px 20px rgba(0,0,0,0.5);padding:40px 48px;box-sizing:border-box;min-height:90%;">' +
								docHtml +
								'  </div>' +
								'</div>';
						} else {
							renderCardFallback();
						}
					}).catch(function() {
						renderCardFallback();
					});
				} else {
					renderCardFallback();
				}
			} else if (ext === 'doc') {
				renderCardFallback('Word 97-2003 Document (.doc). Click below to open in Microsoft Word or default application.');
			} else if (ext === 'pptx') {
				body.innerHTML = '<div style="color:#8696a0;font-size:13px;display:flex;align-items:center;gap:8px;">⏳ Loading PowerPoint preview...</div>';
				var uint8Ppt = base64ToUint8Array(dataUri || '');
				if (uint8Ppt) {
					var parsePromisePpt = Promise.all([
						readZipEntryText(uint8Ppt, 'ppt/slides/slide1.xml'),
						readZipEntryText(uint8Ppt, 'ppt/slides/slide2.xml'),
						readZipEntryText(uint8Ppt, 'ppt/slides/slide3.xml'),
						readZipEntryText(uint8Ppt, 'ppt/slides/slide4.xml'),
						readZipEntryText(uint8Ppt, 'ppt/slides/slide5.xml')
					]);
					var timeoutPromisePpt = new Promise(function(resolve) { setTimeout(function() { resolve(null); }, 1500); });
					Promise.race([parsePromisePpt, timeoutPromisePpt]).then(function(slides) {
						var validSlides = slides ? slides.filter(Boolean) : [];
						if (validSlides.length) {
							body.innerHTML = parsePptxToHtml(validSlides);
						} else {
							renderCardFallback();
						}
					}).catch(function() {
						renderCardFallback();
					});
				} else {
					renderCardFallback();
				}
			} else if (ext === 'ppt') {
				renderCardFallback('PowerPoint 97-2003 Presentation (.ppt). Click below to open in PowerPoint or default application.');
			} else if (ext === 'txt' || ext === 'rtf' || ext === 'log') {
				try {
					var rawTxtB64 = (dataUri || '').indexOf(';base64,') !== -1 ? (dataUri || '').split(';base64,')[1] : (dataUri || '');
					var binTxt = atob(rawTxtB64);
					var bytesTxt = new Uint8Array(binTxt.length);
					for (var ti = 0; ti < binTxt.length; ti++) bytesTxt[ti] = binTxt.charCodeAt(ti);
					var textContent = new TextDecoder('utf-8').decode(bytesTxt);
					body.innerHTML = '<div style="width:100%;height:100%;overflow:auto;padding:24px;box-sizing:border-box;background:#111b21;contain:content;overscroll-behavior:contain;color:#e9edef;font-family:monospace;font-size:13px;line-height:1.6;white-space:pre-wrap;">' +
						textContent.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') +
						'</div>';
				} catch (e) {
					renderCardFallback();
				}
			} else {
				renderCardFallback();
			}

			overlay.appendChild(modal);
			document.body.appendChild(overlay);

			function closeDocModal() {
				window.removeEventListener('keydown', onEsc);
				if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
				if (ownedBlobUrl) {
					try { URL.revokeObjectURL(ownedBlobUrl); } catch (e) {}
					ownedBlobUrl = '';
				}
				dataUri = '';
				if (window.dismissStuckViewer) window.dismissStuckViewer();
			}

			document.getElementById('wa-btn-close-doc').onclick = closeDocModal;
			overlay.onclick = function(e) {
				if (e.target === overlay) closeDocModal();
			};

			document.getElementById('wa-btn-open-preview').onclick = triggerOpenSystem;

			var btnFolder = document.getElementById('wa-btn-folder-doc');
			if (btnFolder) {
				btnFolder.onclick = function() {
					if (window.openDownloadDirNative) window.openDownloadDirNative();
				};
			}

			document.getElementById('wa-btn-save-doc').onclick = function() {
				if (dataUri && window.saveDownloadedFileNative) {
					window.saveDownloadedFileNative(filename, dataUri).then(function(p) {
						if (p) showFloatingToast('💾 Saved: ' + filename);
					});
				} else if (savedPath) {
					showFloatingToast('💾 File is saved at: ' + savedPath);
				}
			};

			var onEsc = function(e) {
				if (e.key === 'Escape' && e.isTrusted && !e._waViewerDismiss) {
					closeDocModal();
				}
			};
			window.addEventListener('keydown', onEsc);
		}
		window.showInAppDocModal = showInAppDocModal;

		// Intercept URL.createObjectURL to catch decrypted PDF/document blobs directly
		var origCreateObjectURL = URL.createObjectURL;
		URL.createObjectURL = function(blob) {
			var url = origCreateObjectURL.apply(this, arguments);
			try {
				var bType = (blob && blob.type) ? blob.type.toLowerCase() : '';
				if (bType.indexOf('image/') === 0 || bType.indexOf('video/') === 0 || bType.indexOf('audio/') === 0) {
					return url;
				}
				var isDocBlob = bType.indexOf('pdf') >= 0 || bType.indexOf('officedocument') >= 0 ||
					bType.indexOf('msword') >= 0 || bType.indexOf('ms-excel') >= 0 ||
					bType.indexOf('spreadsheet') >= 0 || bType.indexOf('wordprocessing') >= 0 ||
					bType === 'text/csv' || bType === 'text/plain' ||
					(blob && (blob.type === 'application/octet-stream' || bType === '') && isRecentPDFIntent());

				if (blob && isDocBlob && !isRecentUpload() && (!isRecentExplicitDownload() || isRecentPDFIntent())) {
					var name = resolveDownloadFilename(lastClickedDocName, '') || 'document';
					if (!name.includes('.')) {
						if (bType.indexOf('pdf') >= 0) name += '.pdf';
						else if (bType.indexOf('sheet') >= 0 || bType.indexOf('excel') >= 0) name += '.xlsx';
						else if (bType.indexOf('word') >= 0) name += '.docx';
						else name += '.pdf';
					}
					// Loop guard. Dismissing WhatsApp's own viewer can make it
					// re-create the attachment blob, which re-entered this
					// interceptor and re-opened the preview window in a loop.
					// The same document is only auto-previewed once per window;
					// a fresh user click resets the intent and is honoured again.
					if (name === lastDocPreviewName && (Date.now() - lastDocPreviewAt) < docPreviewCooldownMs) {
						waDiag('doc', 'loop guard: skipped re-preview of ' + name + ' after ' + (Date.now() - lastDocPreviewAt) + 'ms');
						// A background re-open, never a user action: a real click
						// clears the guard above. Recorded (not shown) so a
						// "preview did nothing" report carries the reason.
						if (window.__waNote) window.__waNote('doc preview', 'suppressed duplicate re-open of ' + name);
						return url;
					}
					lastDocPreviewName = name;
					lastDocPreviewAt = Date.now();
					waDiag('doc', 'previewing ' + name + ' (type=' + bType + ', ' + blob.size + 'B)');
					var isPdf = name.toLowerCase().endsWith('.pdf');
					var previewBlob = isPdf ? blob.slice(0, blob.size, 'application/pdf') : blob;
					var ownedBlobUrl = isPdf ? origCreateObjectURL(previewBlob) : '';
					var reader = new FileReader();
					reader.onloadend = function() {
						var base64data = reader.result;
						if (window.saveDownloadedFileNative) {
							window.saveDownloadedFileNative(name, base64data).then(function(savedPath) {
								showInAppDocModal(name, ownedBlobUrl, savedPath, base64data, ownedBlobUrl);
								dismissStuckViewer();
								showFloatingToast('📄 Document preview: ' + name);
							});
						} else {
							showInAppDocModal(name, ownedBlobUrl, '', base64data, ownedBlobUrl);
							dismissStuckViewer();
						}
					};
					reader.readAsDataURL(blob);
				}
			} catch (e) {}
			return url;
		};

		function handleBlobDocumentPreview(blobUrl) {
			var name = resolveDownloadFilename(lastClickedDocName, '') || 'document.pdf';
			fetch(blobUrl)
				.then(function(res) { return res.blob(); })
				.then(function(blob) {
					var isPdf = name.toLowerCase().endsWith('.pdf');
					var previewBlob = isPdf ? blob.slice(0, blob.size, 'application/pdf') : blob;
					var ownedBlobUrl = isPdf ? origCreateObjectURL(previewBlob) : '';
					var reader = new FileReader();
					reader.onloadend = function() {
						var base64data = reader.result;
						if (window.saveDownloadedFileNative) {
							window.saveDownloadedFileNative(name, base64data).then(function(savedPath) {
								showInAppDocModal(name, ownedBlobUrl, savedPath, base64data, ownedBlobUrl);
								dismissStuckViewer();
								showFloatingToast('📄 Document preview: ' + name);
							});
						} else {
							showInAppDocModal(name, ownedBlobUrl, '', base64data, ownedBlobUrl);
							dismissStuckViewer();
						}
					};
					reader.readAsDataURL(blob);
				})
				.catch(function(err) {
					console.error('Error handling blob preview:', err);
				});
		}

		// Intercept window.open for Blob URLs (PDF/Document previews) and external URLs
		var origWindowOpen = window.open;
		window.open = function(url, target, features) {
			if (url && typeof url === 'string') {
				if (url.indexOf('blob:') === 0) {
					handleBlobDocumentPreview(url);
					return null;
				}
				try {
					var parsed = new URL(url, window.location.href);
					if (!parsed.hostname.endsWith('whatsapp.com') && !parsed.hostname.endsWith('whatsapp.net') && (parsed.protocol === 'http:' || parsed.protocol === 'https:')) {
						if (window.openExternalLink) {
							window.openExternalLink(parsed.href);
							return null;
						}
					}
				} catch(err) {}
			}
			return origWindowOpen.apply(this, arguments);
		};

		// Zoom Keyboard Shortcuts (Cmd + / Cmd - / Cmd 0)
		waRunModule('zoom-shortcuts', function() {
			var currentZoom = 1.0;
			window.addEventListener('keydown', function(e) {
				if (e.metaKey || e.ctrlKey) {
					if (e.key === '=' || e.key === '+') {
						e.preventDefault();
						currentZoom = Math.min(currentZoom + 0.1, 2.0);
						document.body.style.zoom = currentZoom;
					} else if (e.key === '-') {
						e.preventDefault();
						currentZoom = Math.max(currentZoom - 0.1, 0.6);
						document.body.style.zoom = currentZoom;
					} else if (e.key === '0') {
						e.preventDefault();
						currentZoom = 1.0;
						document.body.style.zoom = currentZoom;
					}
				}
			});
		});

		// Dock Badge Unread Count Synchronizer (shows new notifications received while unfocused)
		waRunModule('dock-badge', function() {
			var isFocused = document.hasFocus ? document.hasFocus() : !document.hidden;
			var unfocusedCount = 0;
			var lastBadge = null;

			function parseTitleUnread() {
				var title = document.title || '';
				var match = title.match(/\(([^)]+)\)/);
				if (!match) return 0;
				var num = parseInt(match[1], 10);
				return isNaN(num) ? 0 : num;
			}
			var baseTitleUnread = parseTitleUnread();

			function updateBadgeDisplay(val) {
				var badgeStr = val > 0 ? String(val) : '';
				if (badgeStr !== lastBadge) {
					lastBadge = badgeStr;
					if (window.updateDockBadge) {
						window.updateDockBadge(badgeStr);
					}
				}
			}

			function onWindowFocused() {
				isFocused = true;
				unfocusedCount = 0;
				baseTitleUnread = parseTitleUnread();
				updateBadgeDisplay(0);
			}

			function onWindowBlur() {
				isFocused = false;
				unfocusedCount = 0;
				baseTitleUnread = parseTitleUnread();
				updateBadgeDisplay(0);
			}

			window.addEventListener('focus', onWindowFocused);
			window.addEventListener('blur', onWindowBlur);
			document.addEventListener('visibilitychange', function() {
				if (document.hidden) {
					onWindowBlur();
				} else {
					onWindowFocused();
				}
			});

			function onNewNotification() {
				if (isFocused && document.hasFocus && document.hasFocus()) return;
				unfocusedCount++;
				updateBadgeDisplay(unfocusedCount);
			}

			window.__waOnNotificationDispatched = onNewNotification;

			function syncBadge() {
				if (isFocused && document.hasFocus && document.hasFocus()) {
					updateBadgeDisplay(0);
					return;
				}
				var currentUnread = parseTitleUnread();
				if (currentUnread > baseTitleUnread) {
					var delta = currentUnread - baseTitleUnread;
					unfocusedCount = Math.max(unfocusedCount, delta);
					updateBadgeDisplay(unfocusedCount);
				} else if (currentUnread < baseTitleUnread) {
					baseTitleUnread = currentUnread;
				}
			}

			var titleEl = document.querySelector('title');
			if (titleEl && titleEl.nodeType) {
				try {
					new MutationObserver(syncBadge).observe(titleEl, { childList: true, characterData: true, subtree: true });
				} catch (e) {
					setInterval(syncBadge, 2000);
				}
			} else {
				setInterval(syncBadge, 2000);
			}
		});

		// Memory Optimization: Idle Garbage Collection
		waRunModule('memory-opt', function() {
			var releaseTimer = null;
			document.addEventListener('visibilitychange', function() {
				clearTimeout(releaseTimer);
				if (!document.hidden) return;
				lastClickedDocName = '';
				lastDocumentIntentAt = 0;
				// Wait a bit longer than a quick alt-tab before trimming memory, so briefly
				// switching windows doesn't repeatedly trigger native working-set trims.
				releaseTimer = setTimeout(function() {
					if (typeof window.gc === 'function') window.gc();
					if (window.releaseMemoryNative) window.releaseMemoryNative();
				}, 5000);
			});
		});

		// Debounced window resize persistence
		waRunModule('window-resize', function() {
			var resizeTimer = null;
			window.addEventListener('resize', function() {
				clearTimeout(resizeTimer);
				resizeTimer = setTimeout(function() {
					if (window.saveWindowStateNative) {
						var w = window.outerWidth || window.innerWidth;
						var h = window.outerHeight || window.innerHeight;
						if (w && h) {
							window.saveWindowStateNative(Math.round(w), Math.round(h));
						}
					}
				}, 500);
			});
		});

		// Floating HUD Toast for User Feedback. Optional action renders a
		// clickable button inside the toast (e.g. "Open folder" after a
		// download); the toast then stays interactive for a few seconds longer.
		function showFloatingToast(msg, action) {
			var toast = document.getElementById('wa-hud-toast');
			if (!toast) {
				toast = document.createElement('div');
				toast.id = 'wa-hud-toast';
				toast.style.cssText = 'position:fixed;top:16px;left:50%;transform:translateX(-50%);background:rgba(32,44,51,0.94);backdrop-filter:blur(10px);color:#00a884;border:1px solid rgba(0,168,132,0.4);border-radius:20px;padding:8px 20px;font-size:12.5px;font-weight:600;z-index:2147483647;box-shadow:0 8px 24px rgba(0,0,0,0.6);transition:all 0.22s cubic-bezier(0.16,1,0.3,1);opacity:0;display:flex;align-items:center;gap:12px;max-width:90vw;';
				var parent = document.body || document.documentElement;
				if (parent) parent.appendChild(toast);
			}
			if (!toast) return;
			var currentParent = document.body || document.documentElement;
			if (currentParent && toast.parentNode !== currentParent) {
				currentParent.appendChild(toast);
			} else if (currentParent && currentParent.lastElementChild !== toast) {
				currentParent.appendChild(toast);
			}
			toast.textContent = '';
			toast.style.pointerEvents = 'none';
			var label = document.createElement('span');
			label.textContent = msg;
			label.style.cssText = 'white-space:nowrap;overflow:hidden;text-overflow:ellipsis;';
			toast.appendChild(label);
			if (action && action.label && typeof action.onClick === 'function') {
				toast.style.pointerEvents = 'auto';
				var btn = document.createElement('button');
				btn.textContent = action.label;
				btn.style.cssText = 'background:#00a884;color:#111b21;border:none;padding:3px 10px;border-radius:12px;font-size:11px;font-weight:700;cursor:pointer;flex-shrink:0;';
				btn.onclick = function(e) {
					e.stopPropagation();
					action.onClick();
					toast.style.opacity = '0';
				};
				toast.appendChild(btn);
			}
			toast.style.opacity = '1';
			toast.style.transform = 'translateX(-50%) translateY(4px)';
			clearTimeout(toast._timer);
			toast._timer = setTimeout(function() {
				toast.style.opacity = '0';
				toast.style.transform = 'translateX(-50%) translateY(0)';
				toast.style.pointerEvents = 'none';
			}, action ? 6000 : 2500);
		}
		window.showFloatingToast = showFloatingToast;

		// Issue reporter: page errors are buffered locally (never uploaded),
		// and the Control Center offers a one-click pre-filled GitHub issue.
		// Nothing leaves the machine until the user presses Report — the
		// browser then shows the composed issue for review before submitting.
		waRunModule('diagnostics-buffer', function() {
			window.__waMeta = { ver: '__WA_APP_VERSION__', platform: '` + runtime.GOOS + `' };

			var waErrBuf = [];
			function waPushErr(kind, msg) {
				msg = String(msg || 'unknown error').slice(0, 200);
				var last = waErrBuf[waErrBuf.length - 1];
				if (last && last.m === msg) { last.n++; return; }
				waErrBuf.push({ k: kind, m: msg, n: 1 });
				if (waErrBuf.length > 25) waErrBuf.shift();
			}
			// Deliberate decisions that changed what the user saw, as opposed to
			// errors. A report saying "the document preview did nothing" is only
			// actionable if it also says the loop guard suppressed the re-open,
			// so these travel with the report instead of living only in the
			// debug log the reporter has to know how to switch on.
			var waNotes = [];
			window.__waNote = function(kind, msg) {
				try {
					msg = String(kind) + ': ' + String(msg == null ? '' : msg).slice(0, 200);
					var last = waNotes[waNotes.length - 1];
					if (last && last.m === msg) { last.n++; return; }
					waNotes.push({ m: msg, n: 1 });
					if (waNotes.length > 25) waNotes.shift();
				} catch (e) {}
			};
			window.addEventListener('error', function(e) {
				var src = '';
				try { src = String(e.filename || '').split('/').pop(); } catch (x) {}
				waPushErr('error', (e.message || 'unknown') + ' @ ' + (src || '?') + ':' + (e.lineno || '?'));
			}, true);
			window.addEventListener('unhandledrejection', function(e) {
				var r = e.reason;
				waPushErr('unhandled', String((r && (r.stack || r.message)) || r).slice(0, 200));
			});

			function resolveMaybe(v) {
				if (v && typeof v.then === 'function') return v;
				return Promise.resolve(v);
			}
			window.openIssueReporter = function(crashTail) {
				var meta = window.__waMeta || { ver: '?', platform: '?' };
				var lines = ['WhatsApp Desk v' + meta.ver + ' (' + meta.platform + ')', ''];
				if (waErrBuf.length) {
					lines.push('Recent page errors:');
					waErrBuf.slice(-8).forEach(function(e) {
						lines.push('- [' + e.k + '] ' + e.m + (e.n > 1 ? ' (x' + e.n + ')' : ''));
					});
					lines.push('');
				} else {
					lines.push('No page errors captured.');
					lines.push('');
				}
				if (waNotes.length) {
					lines.push('Feature notes:');
					waNotes.slice(-8).forEach(function(e) {
						lines.push('- ' + e.m + (e.n > 1 ? ' (x' + e.n + ')' : ''));
					});
					lines.push('');
				}
				if (crashTail) {
					var fence = String.fromCharCode(96, 96, 96);
					lines.push('Crash log tail:');
					lines.push(fence);
					lines.push(String(crashTail).slice(0, 1200));
					lines.push(fence);
				}
				lines.push('_Submitted from the in-app reporter — please add steps to reproduce._');
				var url = 'https://github.com/vianziro/Whatsapp-Dekstop/issues/new' +
					'?title=' + encodeURIComponent('Report v' + meta.ver + ' (' + meta.platform + '): ') +
					'&body=' + encodeURIComponent(lines.join('\n').slice(0, 2500)) +
					'&labels=' + encodeURIComponent('bug');
				if (window.openExternalLink) window.openExternalLink(url);
				if (window.markCrashNotifiedNative) {
					try { resolveMaybe(window.markCrashNotifiedNative()); } catch (e) {}
				}
			};
			window.reportIssueNow = function() {
				if (window.getPendingCrashNative) {
					try {
						resolveMaybe(window.getPendingCrashNative()).then(function(t) {
							window.openIssueReporter(t || '');
						});
						return;
					} catch (e) {}
				}
				window.openIssueReporter('');
			};

			// Startup nudge, once per crash: offer reporting instead of nagging.
			setTimeout(function() {
				if (!window.getPendingCrashNative || typeof showFloatingToast !== 'function') return;
				try {
					resolveMaybe(window.getPendingCrashNative()).then(function(tail) {
						if (!tail) return;
						showFloatingToast('⚠️ Previous session crashed — tap to report', {
							label: 'Report',
							onClick: function() { window.reportIssueNow(); }
						});
					});
				} catch (e) {}
			}, 10000);
		});

		// Privacy Mode Toggle (Cmd + Shift + P)
		waRunModule('privacy-mode', function() {
			var isPrivacyActive = false;
			var styleEl = document.createElement('style');
			styleEl.id = 'whatsapp-privacy-style';
			// PRIVACY STRATEGY: text and previews use authentic visual blur
			// (filter: blur(6px)), not opaque gray redaction blocks.
			// Chat-list timestamps are private too and reveal with their row.
			// Full set of chat list container selectors ensures instant auto-unblur
			// on hover across all modern WhatsApp Web DOM structures.
			styleEl.textContent = [
				// Layer 1: names + previews in the chat list, hover row/item to peek.
				// Chat-list timestamps are included in this blur layer.
				// Covers #side generally (including Archived chats drawer & filtered views)
				// as well as #pane-side and modern aria/data-testid containers.
				'.privacy-mode [data-wa-privacy-chat-row="1"] span,',
				'.privacy-mode [data-wa-privacy-chat-row="1"] ._ak8q,',
				'.privacy-mode [data-wa-privacy-chat-row="1"] ._ak8k,',
				'.privacy-mode [data-wa-privacy-archived-row="1"] span,',
				'.privacy-mode [data-wa-privacy-archived-row="1"] ._ak8q,',
				'.privacy-mode [data-wa-privacy-archived-row="1"] ._ak8k,',
				'.blur-chatlist-only [data-wa-privacy-chat-row="1"] span,',
				'.blur-chatlist-only [data-wa-privacy-chat-row="1"] ._ak8q,',
				'.blur-chatlist-only [data-wa-privacy-chat-row="1"] ._ak8k,',
				'.blur-chatlist-only [data-wa-privacy-archived-row="1"] span,',
				'.blur-chatlist-only [data-wa-privacy-archived-row="1"] ._ak8q,',
				'.blur-chatlist-only [data-wa-privacy-archived-row="1"] ._ak8k',
				'{ filter: blur(6px) !important; transition: filter 0.15s ease-out !important; }',
				// The Archived view explanation is UI guidance, not private chat data.
				'.privacy-mode [data-wa-privacy-archived-info="1"],',
				'.privacy-mode [data-wa-privacy-archived-info="1"] *',
				'{ filter: none !important; }',
				'.privacy-mode [data-wa-privacy-archive-control="1"],',
				'.privacy-mode [data-wa-privacy-archive-control="1"] *',
				'{ filter: none !important; }',
				'.privacy-mode.blur-avatars [data-wa-privacy-avatar="1"],',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-avatar="1"]',
				'{ filter: blur(12px) !important; transition: filter 0.15s ease-out !important; }',
				'.privacy-mode.blur-avatars [data-wa-privacy-archived-row="1"] img,',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-archived-row="1"] img,',
				'.privacy-mode.blur-avatars [data-wa-privacy-archived-row="1"] image,',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-archived-row="1"] image,',
				'.privacy-mode.blur-avatars [data-wa-privacy-archived-row="1"] ._ak8h,',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-archived-row="1"] ._ak8h,',
				'.privacy-mode.blur-avatars [data-wa-privacy-archived-row="1"] [data-testid*="avatar" i],',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-archived-row="1"] [data-testid*="avatar" i],',
				'.privacy-mode.blur-avatars [data-wa-privacy-archived-row="1"] [data-testid="default-user"],',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-archived-row="1"] [data-testid="default-user"],',
				'.privacy-mode.blur-avatars [data-wa-privacy-archived-row="1"] [data-icon="default-user"],',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-archived-row="1"] [data-icon="default-user"],',
				'.privacy-mode.blur-avatars [data-wa-privacy-archived-row="1"] [data-icon="default-group"],',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-archived-row="1"] [data-icon="default-group"],',
				'.privacy-mode.blur-avatars [data-wa-privacy-archived-row="1"] svg[viewBox="0 0 49 49"],',
				'.blur-chatlist-only.blur-avatars [data-wa-privacy-archived-row="1"] svg[viewBox="0 0 49 49"]',
				'{ filter: blur(12px) !important; transition: filter 0.15s ease-out !important; }',
				// Hovering any row or container restores its contents instantly.
				'.privacy-mode [data-wa-privacy-hover="1"] span,',
				'.privacy-mode [data-wa-privacy-hover="1"] ._ak8q,',
				'.privacy-mode [data-wa-privacy-hover="1"] ._ak8k,',
				'.privacy-mode [data-wa-privacy-reveal="1"],',
				'.blur-chatlist-only [data-wa-privacy-hover="1"] span,',
				'.blur-chatlist-only [data-wa-privacy-hover="1"] ._ak8q,',
				'.blur-chatlist-only [data-wa-privacy-hover="1"] ._ak8k,',
				'.blur-chatlist-only [data-wa-privacy-reveal="1"]',
				'{ filter: none !important; }',
				// Layer 2: everything textual inside a message bubble.
				// Hovering the bubble restores the whole subtree.
				'.privacy-mode #main [data-testid="msg-container"] span:not([data-wa-time]),',
				'.privacy-mode #main .message-in span:not([data-wa-time]),',
				'.privacy-mode #main .message-out span:not([data-wa-time])',
				'{ filter: blur(6px) !important; transition: filter 0.15s ease-out !important; }',
				'.privacy-mode #main [data-testid="msg-container"]:hover span,',
				'.privacy-mode #main .message-in:hover span,',
				'.privacy-mode #main .message-out:hover span,',
				'.privacy-mode #main [data-testid="msg-container"] span:hover,',
				'.privacy-mode #main .message-in span:hover,',
				'.privacy-mode #main .message-out span:hover',
				'{ filter: none !important; }',
				// In-chat media previews hide with blur; cover the stable message row
				// too because document thumbnails and quoted media may sit outside the
				// older message-in/message-out wrappers.
				'.privacy-mode #main [data-testid="msg-container"] img:not([data-emoji]),',
				'.privacy-mode #main [data-testid="msg-container"] video,',
				'.privacy-mode #main [data-testid="msg-container"] canvas,',
				'.privacy-mode #main .message-in img:not([data-emoji]),',
				'.privacy-mode #main .message-in video,',
				'.privacy-mode #main .message-in canvas,',
				'.privacy-mode #main .message-out img:not([data-emoji]),',
				'.privacy-mode #main .message-out video,',
				'.privacy-mode #main .message-out canvas,',
				'.privacy-mode #main [role="row"] img:not([data-emoji]),',
				'.privacy-mode #main [role="row"] image,',
				'.privacy-mode #main [role="row"] video,',
				'.privacy-mode #main [role="row"] canvas,',
				'.privacy-mode #main [role="row"] iframe,',
				'.privacy-mode #main [role="row"] [style*="background-image"],',
				'.privacy-mode #main [role="row"] [data-testid="quoted-message"]',
				'{ filter: blur(12px) !important; transition: filter 0.15s ease-out !important; }',
				'.privacy-mode #main [data-testid="msg-container"]:hover img,',
				'.privacy-mode #main [data-testid="msg-container"]:hover video,',
				'.privacy-mode #main [data-testid="msg-container"]:hover canvas,',
				'.privacy-mode #main .message-in:hover img,',
				'.privacy-mode #main .message-in:hover video,',
				'.privacy-mode #main .message-in:hover canvas,',
				'.privacy-mode #main .message-out:hover img,',
				'.privacy-mode #main .message-out:hover video,',
				'.privacy-mode #main .message-out:hover canvas,',
				'.privacy-mode #main [role="row"]:hover img,',
				'.privacy-mode #main [role="row"]:hover image,',
				'.privacy-mode #main [role="row"]:hover video,',
				'.privacy-mode #main [role="row"]:hover canvas,',
				'.privacy-mode #main [role="row"]:hover iframe,',
				'.privacy-mode #main [role="row"]:hover [style*="background-image"],',
				'.privacy-mode #main [role="row"]:hover [data-testid="quoted-message"],',
				'.privacy-mode #main [data-testid="msg-container"] img:hover,',
				'.privacy-mode #main [data-testid="msg-container"] video:hover,',
				'.privacy-mode #main [data-testid="msg-container"] canvas:hover',
				'{ filter: none !important; }',
				// Stickers use dedicated containers (and canvases for animated stickers),
				// so they need their own blur layer instead of relying on photo selectors.
				'.privacy-mode #main [data-testid="sticker-container"],',
				'.privacy-mode #main [data-testid="animated-sticker"],',
				'.privacy-mode #main img[src*=".webp"][data-testid*="sticker" i]',
				'{ filter: blur(12px) !important; transition: filter 0.15s ease-out !important; }',
				'.privacy-mode #main [data-testid="sticker-container"]:hover,',
				'.privacy-mode #main [data-testid="sticker-container"]:hover *,',
				'.privacy-mode #main [data-testid="msg-container"]:hover [data-testid="sticker-container"],',
				'.privacy-mode #main [data-testid="msg-container"]:hover [data-testid="animated-sticker"],',
				'.privacy-mode #main [data-testid="msg-container"]:hover img[src*=".webp"][data-testid*="sticker" i],',
				'.privacy-mode #main .message-in:hover [data-testid="sticker-container"],',
				'.privacy-mode #main .message-in:hover [data-testid="animated-sticker"],',
				'.privacy-mode #main .message-in:hover img[src*=".webp"][data-testid*="sticker" i],',
				'.privacy-mode #main .message-out:hover [data-testid="sticker-container"],',
				'.privacy-mode #main .message-out:hover [data-testid="animated-sticker"],',
				'.privacy-mode #main .message-out:hover img[src*=".webp"][data-testid*="sticker" i],',
				'.privacy-mode #main [role="row"]:hover [data-testid="sticker-container"],',
				'.privacy-mode #main [role="row"]:hover [data-testid="animated-sticker"],',
				'.privacy-mode #main [role="row"]:hover img[src*=".webp"][data-testid*="sticker" i],',
				'.privacy-mode #main [data-testid="animated-sticker"]:hover,',
				'.privacy-mode #main img[src*=".webp"][data-testid*="sticker" i]:hover',
				'{ filter: none !important; }',
				// Layer 3: conversation header name/status, hover to reveal.
				'.privacy-mode #main header span:not([data-wa-time])',
				'{ filter: blur(6px) !important; transition: filter 0.15s ease-out !important; }',
				'.privacy-mode #main header:hover span,',
				'.privacy-mode #main header span:hover',
				'{ filter: none !important; }',
				// Layer 4: optional avatar blur (.blur-avatars on <html>).
				// Supports standard contacts, pinned chats, disappearing messages,
				// contacts posting a status (with status rings), archived chats,
				// and contacts with or without custom profile pictures (SVG/default user).
				'.privacy-mode.blur-avatars #side img,',
				'.privacy-mode.blur-avatars #side image,',
				'.privacy-mode.blur-avatars #side ._ak8h,',
				'.privacy-mode.blur-avatars #side [data-testid="default-user"],',
				'.privacy-mode.blur-avatars #side [data-icon="default-user"],',
				'.privacy-mode.blur-avatars #side [data-icon="default-group"],',
				'.privacy-mode.blur-avatars #side [data-icon="community-outline"],',
				'.privacy-mode.blur-avatars #side svg[viewBox="0 0 49 49"],',
				'.privacy-mode.blur-avatars #pane-side img,',
				'.privacy-mode.blur-avatars #pane-side image,',
				'.privacy-mode.blur-avatars #pane-side ._ak8h,',
				'.privacy-mode.blur-avatars #pane-side [data-testid="default-user"],',
				'.privacy-mode.blur-avatars [data-testid="chat-list"] img,',
				'.privacy-mode.blur-avatars [data-testid="chat-list"] image,',
				'.privacy-mode.blur-avatars [data-testid="chat-list"] ._ak8h,',
				'.privacy-mode.blur-avatars [data-testid="chat-list"] [data-testid="default-user"],',
				'.privacy-mode.blur-avatars div[aria-label="Chat list"] img,',
				'.privacy-mode.blur-avatars div[aria-label="Chat list"] image,',
				'.privacy-mode.blur-avatars div[aria-label="Chat list"] ._ak8h,',
				'.privacy-mode.blur-avatars div[aria-label*="Archived" i] img,',
				'.privacy-mode.blur-avatars div[aria-label*="Archived" i] image,',
				'.privacy-mode.blur-avatars div[aria-label*="Archived" i] ._ak8h,',
				'.privacy-mode.blur-avatars #main header img,',
				'.privacy-mode.blur-avatars #main header image,',
				'.privacy-mode.blur-avatars #main header ._ak8h,',
				'.privacy-mode.blur-avatars #main header [data-testid="default-user"],',
				'.privacy-mode.blur-avatars #main header [data-icon="default-user"],',
				'.privacy-mode.blur-avatars #main header svg[viewBox="0 0 49 49"],',
				'.privacy-mode.blur-avatars #main header div[role="button"]:first-child div.x1n2onr6.x16ye13r.x5lhr3w,',
				'.privacy-mode.blur-avatars #main .message-in img:not([data-emoji]),',
				'.privacy-mode.blur-avatars #main .message-in image,',
				'.privacy-mode.blur-avatars #main .message-in ._ak8h,',
				'.privacy-mode.blur-avatars #main .message-out img:not([data-emoji]),',
				'.privacy-mode.blur-avatars #main .message-out image,',
				'.privacy-mode.blur-avatars #main .message-out ._ak8h,',
				'.privacy-mode.blur-avatars #main [data-testid="msg-container"] ._ak8h,',
				'.privacy-mode.blur-avatars div[role="dialog"] ._ak8h,',
				'.privacy-mode.blur-avatars div[role="dialog"] img',
				'{ filter: blur(12px) !important; transition: filter 0.15s ease-out !important; }',
				// Sidebar avatar reveal is marker-based so a broad parent hover cannot
				// reveal avatars from neighboring chat rows.
				'.privacy-mode.blur-avatars #main header:hover img,',
				'.privacy-mode.blur-avatars #main header:hover image,',
				'.privacy-mode.blur-avatars #main header:hover ._ak8h,',
				'.privacy-mode.blur-avatars #main header:hover [data-testid="default-user"],',
				'.privacy-mode.blur-avatars #main header:hover svg[viewBox="0 0 49 49"],',
				'.privacy-mode.blur-avatars #main header img:hover,',
				'.privacy-mode.blur-avatars #main header image:hover,',
				'.privacy-mode.blur-avatars #main header ._ak8h:hover,',
				'.privacy-mode.blur-avatars #main header ._ak8h:hover *,',
				'.privacy-mode.blur-avatars #main header [data-testid="default-user"]:hover,',
				'.privacy-mode.blur-avatars #main header svg[viewBox="0 0 49 49"]:hover,',
				'.privacy-mode.blur-avatars #main .message-in:hover img,',
				'.privacy-mode.blur-avatars #main .message-in:hover image,',
				'.privacy-mode.blur-avatars #main .message-in:hover ._ak8h,',
				'.privacy-mode.blur-avatars #main .message-out:hover img,',
				'.privacy-mode.blur-avatars #main .message-out:hover image,',
				'.privacy-mode.blur-avatars #main .message-out:hover ._ak8h,',
				'.privacy-mode.blur-avatars div[role="dialog"] ._ak8h:hover,',
				'.privacy-mode.blur-avatars div[role="dialog"] img:hover',
				'{ filter: none !important; }',
				// Layer 5: fullscreen media viewer
				'.privacy-mode [data-testid="media-viewer"] img,',
				'.privacy-mode [data-testid="media-viewer"] video',
				'{ filter: blur(16px) !important; transition: filter 0.15s ease-out !important; }',
				'.privacy-mode [data-testid="media-viewer"]:hover img,',
				'.privacy-mode [data-testid="media-viewer"]:hover video',
				'{ filter: none !important; }',
				// Layer 6: when "Blur chat list only" is enabled, keep #main (opened chat) completely unblurred
				'.blur-chatlist-only #main,',
				'.blur-chatlist-only #main *,',
				'.privacy-mode.blur-chatlist-only #main,',
				'.privacy-mode.blur-chatlist-only #main *,',
				'.privacy-mode.blur-chatlist-only #main [data-testid="msg-container"] span,',
				'.privacy-mode.blur-chatlist-only #main .message-in span,',
				'.privacy-mode.blur-chatlist-only #main .message-out span,',
				'.privacy-mode.blur-chatlist-only #main [data-testid="msg-container"] img,',
				'.privacy-mode.blur-chatlist-only #main [data-testid="msg-container"] video,',
				'.privacy-mode.blur-chatlist-only #main [data-testid="msg-container"] canvas,',
				'.privacy-mode.blur-chatlist-only #main .message-in img,',
				'.privacy-mode.blur-chatlist-only #main .message-in video,',
				'.privacy-mode.blur-chatlist-only #main .message-in canvas,',
				'.privacy-mode.blur-chatlist-only #main .message-out img,',
				'.privacy-mode.blur-chatlist-only #main .message-out video,',
				'.privacy-mode.blur-chatlist-only #main .message-out canvas,',
				'.privacy-mode.blur-chatlist-only #main [role="row"] img,',
				'.privacy-mode.blur-chatlist-only #main [role="row"] image,',
				'.privacy-mode.blur-chatlist-only #main [role="row"] video,',
				'.privacy-mode.blur-chatlist-only #main [role="row"] canvas,',
				'.privacy-mode.blur-chatlist-only #main [role="row"] iframe,',
				'.privacy-mode.blur-chatlist-only #main [role="row"] [style*="background-image"],',
				'.privacy-mode.blur-chatlist-only #main [role="row"] [data-testid="quoted-message"],',
				'.privacy-mode.blur-chatlist-only #main [data-testid="sticker-container"],',
				'.privacy-mode.blur-chatlist-only #main [data-testid="sticker-container"] *,',
				'.privacy-mode.blur-chatlist-only #main [data-testid="animated-sticker"],',
				'.privacy-mode.blur-chatlist-only #main img[src*=".webp"][data-testid*="sticker" i],',
				'.privacy-mode.blur-chatlist-only #main header span,',
				'.privacy-mode.blur-chatlist-only #main header img,',
				'.privacy-mode.blur-chatlist-only #main header image,',
				'.privacy-mode.blur-chatlist-only #main header ._ak8h,',
				'.privacy-mode.blur-chatlist-only #main header [data-testid="default-user"],',
				'.privacy-mode.blur-chatlist-only #main header [data-icon="default-user"],',
				'.privacy-mode.blur-chatlist-only #main header svg[viewBox="0 0 49 49"],',
				'.privacy-mode.blur-chatlist-only #main header div[role="button"]:first-child div.x1n2onr6.x16ye13r.x5lhr3w,',
				'.privacy-mode.blur-chatlist-only #main [data-testid="msg-container"],',
				'.privacy-mode.blur-chatlist-only #main [data-testid="msg-container"] *',
				'{ filter: none !important; }',
				// Drag & drop visual feedback
				'.wa-drag-over { outline: 3px solid #00a884; outline-offset: -3px; }',
				'.wa-drag-over * { pointer-events: none; }'
			].join('\n');

			var activePrivacyHoverRow = null;
			// Elements the current hovered row had to un-blur inline. Tracked so
			// clearing touches only what was changed instead of scanning the whole
			// document for the marker on every row the cursor crosses.
			var privacyHoverOverrides = [];
			// Text in a hovered row is un-blurred by the [data-wa-privacy-hover]
			// CSS rules, so only media and avatar slots need an inline override -
			// their blur rules are anchored to #side / #pane-side ids, and an
			// inline !important is the only thing that reliably cancels those.
			var PRIVACY_HOVER_OVERRIDE_SELECTOR = 'img, image, ._ak8h, [data-wa-privacy-avatar="1"], [data-testid*="avatar" i], [data-testid="default-user"], [data-icon="default-user"], [data-icon="default-group"], [data-icon="community-outline"], svg[viewBox="0 0 49 49"]';
			var PRIVACY_ARCHIVED_LABEL_RE = /^(Archived|Diarsipkan|Archiviert|Archivio|Archiviati|Archivados?|Archivadas?|Архив|已归档|封存)\b/i;
			var PRIVACY_ARCHIVED_INFO_RE = /These chats stay archived when new messages are received|To change this experience, go to settings > chats on your phone|Obrolan ini tetap diarsipkan saat pesan baru diterima|Untuk mengubah pengalaman ini.*Pengaturan.*(Chat|Obrolan)/i;
			// Every chat-list row shape WhatsApp Web has used. Shared so the
			// per-pointer-event predicates can reject a container in one
			// selector match instead of scanning its subtree.
			var PRIVACY_ROW_SELECTOR = '[role="row"], [role="listitem"], [data-testid="cell-frame-container"], div[tabindex="-1"], div._ak8l';
			function privacyIsArchivedNavigationText(text) {
				return PRIVACY_ARCHIVED_LABEL_RE.test((text || '').replace(/\s+/g, ' ').trim());
			}
			function privacyIsArchivedInfoText(text) {
				return PRIVACY_ARCHIVED_INFO_RE.test((text || '').replace(/\s+/g, ' ').trim());
			}
			// Avatar slots are marked in two steps on purpose. Marking a row writes
			// attributes that change styles, and the geometry fallback below reads
			// layout back - so running both inside the row loop forced a full
			// style+layout flush per row. Probes are queued and run once all the
			// writes have landed.
			var privacyGeometryQueue = [];
			function markPrivacyAvatarTargets(row, avatarSelector) {
				var avatars = row.querySelectorAll(avatarSelector);
				for (var a = 0; a < avatars.length; a++) avatars[a].setAttribute('data-wa-privacy-avatar', '1');
				if (avatars.length) return;
				if (privacyGeometryQueue.indexOf(row) === -1) privacyGeometryQueue.push(row);
			}
			function flushPrivacyAvatarGeometry() {
				if (!privacyGeometryQueue.length) return;
				var queue = privacyGeometryQueue;
				privacyGeometryQueue = [];
				for (var q = 0; q < queue.length; q++) probePrivacyAvatarGeometry(queue[q]);
			}
			function probePrivacyAvatarGeometry(row) {
				if (!row || !row.getBoundingClientRect) return;
				// An earlier pass already located this row's slot, so there is
				// nothing left to measure.
				if (row.querySelector('[data-wa-privacy-avatar="1"]')) return;
				var rowRect = row.getBoundingClientRect();
				if (!rowRect || rowRect.width <= 0 || rowRect.height <= 0) return;
				// getComputedStyle is the most expensive read here, so the probe is
				// bounded per row. A row whose leading edge holds no background-image
				// slot is simply left unmarked, which is the pre-existing outcome for
				// exotic layouts too.
				var probes = 0;
				var visualCandidates = row.querySelectorAll('div, span, [role="img"]');
				for (var v = 0; v < visualCandidates.length && probes < 40; v++) {
					var visual = visualCandidates[v];
					var visualRect = visual.getBoundingClientRect ? visual.getBoundingClientRect() : null;
					if (!visualRect || visualRect.width < 28 || visualRect.height < 28 || visualRect.width > 96 || visualRect.height > 96) continue;
					if (Math.abs(visualRect.width - visualRect.height) > 18 || visualRect.left > rowRect.left + 96 || visualRect.top > rowRect.top + 24) continue;
					var backgroundImage = '';
					probes++;
					try { backgroundImage = window.getComputedStyle(visual).backgroundImage || ''; } catch (e) {}
					if (backgroundImage === '' || backgroundImage === 'none') continue;
					visual.setAttribute('data-wa-privacy-avatar', '1');
					return;
				}
				// WhatsApp renders initials as text inside a circular slot instead of an img.
				// The same geometry guard prevents the fallback from marking the row.
				var initials = row.querySelectorAll('span, div');
				for (var i = 0; i < initials.length && i < 40; i++) {
					var text = (initials[i].textContent || '').trim();
					if (!/^[A-Za-z0-9]{1,3}$/.test(text)) continue;
					var candidate = initials[i];
					for (var depth = 0; candidate && candidate !== row && depth < 5; depth++, candidate = candidate.parentElement) {
						var rect = candidate.getBoundingClientRect ? candidate.getBoundingClientRect() : null;
						if (!rect || rect.width < 28 || rect.height < 28 || rect.width > 96 || rect.height > 96) continue;
						if (Math.abs(rect.width - rect.height) > 18 || rect.left > rowRect.left + 96 || rect.top > rowRect.top + 24) continue;
						candidate.setAttribute('data-wa-privacy-avatar', '1');
						return;
					}
				}
			}
			function markPrivacyChatRows() {
				var roots = document.querySelectorAll('#side, #pane-side, [data-testid="chat-list"], div[aria-label="Chat list"]');
				var rowSelector = '[role="row"], [role="listitem"], [data-testid="cell-frame-container"], div._ak8l';
				var avatarSelector = 'img, image, ._ak8h, [data-testid="default-user"], [data-testid*="avatar" i], [data-icon="default-user"], [data-icon="default-group"], svg[viewBox="0 0 49 49"]';
				for (var r = 0; r < roots.length; r++) {
					var rows = roots[r].querySelectorAll(rowSelector);
					for (var i = 0; i < rows.length; i++) {
						var row = rows[i];
						var rowText = (row.textContent || '').trim();
						if (privacyIsArchivedNavigationText(rowText) || row.querySelector('[data-icon*="archive" i], [data-testid*="archive" i], [aria-label*="archiv" i], [aria-label*="diarsip" i]')) continue;
						if (row.querySelectorAll('span').length < 2 &&
							!row.matches('[data-testid="cell-frame-container"], div._ak8l')) continue;
						row.setAttribute('data-wa-privacy-chat-row', '1');
						markPrivacyAvatarTargets(row, avatarSelector);
					}
				}
				var archivedLabels = document.querySelectorAll('#side span, #pane-side span, #side [role="button"], #pane-side [role="button"]');
				for (var l = 0; l < archivedLabels.length; l++) {
					var label = archivedLabels[l];
					if (!privacyIsArchivedNavigationText(label.textContent)) continue;
					var labelParent = label;
					for (var depth = 0; labelParent && depth < 8; depth++, labelParent = labelParent.parentElement) {
						if (!privacyIsArchivedNavigationText(labelParent.textContent)) continue;
						if (labelParent.clientHeight >= 40 || labelParent.querySelector('[data-icon*="archive" i], [data-testid*="archive" i]')) {
							labelParent.removeAttribute('data-wa-privacy-chat-row');
							labelParent.setAttribute('data-wa-privacy-archive-control', '1');
							break;
						}
					}
				}
				var archiveNodes = document.querySelectorAll('[data-icon*="archive" i], [data-testid*="archive" i], [aria-label*="archiv" i], [aria-label*="diarsip" i]');
				for (var n = 0; n < archiveNodes.length; n++) {
					var control = archiveNodes[n];
					while (control && control !== document.body) {
						var controlText = (control.textContent || '').trim();
						if (privacyIsArchivedNavigationText(controlText)) {
							control.removeAttribute('data-wa-privacy-chat-row');
							control.setAttribute('data-wa-privacy-archive-control', '1');
							break;
						}
						control = control.parentElement;
					}
				}
				flushPrivacyAvatarGeometry();
			}
			function forceArchivedControlVisible() {
				// Resolve every target first, then write. The resolved step reads
				// layout (getBoundingClientRect) and the write step invalidates it,
				// so interleaving the two flushed style+layout once per candidate.
				var labels = document.querySelectorAll('[data-wa-privacy-archive-control="1"]');
				var targets = [];
				for (var i = 0; i < labels.length; i++) {
					if (!privacyIsArchivedNavigationText(labels[i].textContent)) continue;
					var control = labels[i];
					for (var depth = 0; control && depth < 10; depth++, control = control.parentElement) {
						var rect = control.getBoundingClientRect ? control.getBoundingClientRect() : null;
						var isRow = control.matches && control.matches(PRIVACY_ROW_SELECTOR);
						if (!isRow && (!rect || rect.height < 40 || rect.width < 200)) continue;
						targets.push({ node: control, deep: true });
						break;
					}
				}
				var archiveIcons = document.querySelectorAll('[data-icon*="archive" i], [data-testid*="archive" i], [aria-label*="archiv" i], [aria-label*="diarsip" i]');
				for (var a = 0; a < archiveIcons.length; a++) {
					var icon = archiveIcons[a];
					var iconRow = icon.closest && icon.closest(PRIVACY_ROW_SELECTOR);
					if (iconRow && privacyIsArchivedNavigationText(iconRow.textContent)) {
						targets.push({ node: iconRow, deep: false });
					}
					if (icon.matches && (icon.matches('[data-icon*="archive" i]') || icon.matches('[data-testid*="archive" i]') || icon.matches('[aria-label*="archiv" i]') || icon.matches('[aria-label*="diarsip" i]'))) {
						targets.push({ node: icon, deep: true });
					}
				}
				for (var t = 0; t < targets.length; t++) {
					var node = targets[t].node;
					node.removeAttribute('data-wa-privacy-chat-row');
					node.setAttribute('data-wa-privacy-archive-control', '1');
					node.style.setProperty('filter', 'none', 'important');
					if (!targets[t].deep) continue;
					var children = node.querySelectorAll('*');
					for (var c = 0; c < children.length; c++) children[c].style.setProperty('filter', 'none', 'important');
				}
			}
			function isPrivacySidebarControl(node) {
				if (!node || !node.matches) return false;
				// Already classified by a previous pass: cheapest possible answer.
				if (node.getAttribute && node.getAttribute('data-wa-privacy-archive-control') === '1') return true;
				// Only a row-scale node can be the Archived navigation entry, and
				// this predicate is asked of every ancestor on every pointer event.
				// Asking it of a container such as #pane-side meant reading the whole
				// chat list's text and running a subtree query per ancestor per
				// event - which is what made the list stutter under the cursor.
				if (!node.matches(PRIVACY_ROW_SELECTOR)) return false;
				var text = (node.textContent || '').trim();
				return privacyIsArchivedNavigationText(text) || !!node.querySelector('[data-icon*="archive" i], [data-testid*="archive" i], [aria-label*="archiv" i], [aria-label*="diarsip" i]');
			}
			function privacyChatListRootFromTarget(target) {
				var node = target && target.nodeType === 1 ? target : null;
				while (node && node !== document.body) {
					if (node.id === 'pane-side' || node.id === 'side' ||
						node.getAttribute('data-testid') === 'chat-list' ||
						node.getAttribute('aria-label') === 'Chat list' ||
						node.getAttribute('data-wa-privacy-archived-view') === '1' ||
						/(archiv|diarsip)/i.test(node.getAttribute('aria-label') || '')) return node;
					node = node.parentElement;
				}
				return null;
			}
			function privacyChatRowFromTarget(target) {
				if (isPrivacyArchivedInfo(target)) return null;
				var listRoot = privacyChatListRootFromTarget(target);
				var node = target && target.nodeType === 1 ? target : null;
				while (node && node !== listRoot && node !== document.body) {
					if (node.getAttribute && node.getAttribute('data-wa-privacy-archived-row') === '1') return node;
					if (isPrivacySidebarControl(node)) return null;
					if (node.matches && (node.matches('[role="row"]') ||
						node.matches('[role="listitem"]') ||
						node.matches('[data-testid="cell-frame-container"]') ||
						node.matches('div[tabindex="-1"]') ||
						node.matches('div._ak8l'))) return node;
					node = node.parentElement;
				}
				return null;
			}
			function isPrivacyArchivedInfo(target) {
				var node = target && target.nodeType === 1 ? target : null;
				while (node && node !== document.body) {
					if (node.getAttribute && node.getAttribute('data-wa-privacy-archived-info') === '1') return true;
					node = node.parentElement;
				}
				return false;
			}
			function clearPrivacyHoverRow() {
				if (activePrivacyHoverRow) activePrivacyHoverRow.removeAttribute('data-wa-privacy-hover');
				// Only the elements this row overrode are restored. A detached row
				// (WhatsApp recycled it without a matching mouseout) is harmless:
				// dropping its inline filter cannot affect what is on screen.
				for (var i = 0; i < privacyHoverOverrides.length; i++) {
					var el = privacyHoverOverrides[i];
					el.removeAttribute('data-wa-privacy-reveal');
					if (!isPrivacyArchivedInfo(el)) {
						el.style.removeProperty('filter');
						el.removeAttribute('data-wa-privacy-filter-overridden');
					}
				}
				privacyHoverOverrides = [];
				activePrivacyHoverRow = null;
			}
			function markPrivacyHoverRow(row) {
				if (activePrivacyHoverRow === row) return;
				clearPrivacyHoverRow();
				activePrivacyHoverRow = row;
				row.setAttribute('data-wa-privacy-hover', '1');
				var revealTargets = row.querySelectorAll(PRIVACY_HOVER_OVERRIDE_SELECTOR);
				for (var i = 0; i < revealTargets.length; i++) {
					if (isPrivacyArchivedInfo(revealTargets[i])) continue;
					revealTargets[i].setAttribute('data-wa-privacy-reveal', '1');
					revealTargets[i].style.setProperty('filter', 'none', 'important');
					revealTargets[i].setAttribute('data-wa-privacy-filter-overridden', '1');
					privacyHoverOverrides.push(revealTargets[i]);
				}
			}
			function updatePrivacyHoverFromTarget(target) {
				if (!privacyChatListRootFromTarget(target)) {
					clearPrivacyHoverRow();
					return;
				}
				var row = privacyChatRowFromTarget(target);
				if (row) markPrivacyHoverRow(row);
				else clearPrivacyHoverRow();
			}
			function markArchivedPrivacyViews() {
				var rowSelector = '[role="row"], [role="listitem"], [data-testid="cell-frame-container"], div[tabindex="-1"], div._ak8l';
				var avatarSelector = 'img, image, ._ak8h, [data-testid*="avatar" i], [data-testid="default-user"], [data-icon="default-user"], [data-icon="default-group"], svg[viewBox="0 0 49 49"]';
				function isArchivedChatRow(row) {
					if (!row || !row.querySelectorAll) return false;
					var text = (row.textContent || '').trim();
					if (!text || privacyIsArchivedNavigationText(text)) return false;
					var hasChatText = row.querySelectorAll('span').length >= 2;
					return hasChatText && (row.matches(rowSelector) || row.querySelector(avatarSelector));
				}
				function markRow(row) {
					if (!isArchivedChatRow(row)) return;
					row.setAttribute('data-wa-privacy-archived-row', '1');
					markPrivacyAvatarTargets(row, avatarSelector);
				}
				function markRowsInView(view) {
					var rows = view.querySelectorAll(rowSelector);
					for (var r = 0; r < rows.length; r++) markRow(rows[r]);
					var avatars = view.querySelectorAll(avatarSelector);
					for (var a = 0; a < avatars.length; a++) {
						var row = avatars[a].parentElement;
						while (row && row !== view) {
							if (isArchivedChatRow(row)) {
								markRow(row);
								break;
							}
							row = row.parentElement;
						}
					}
				}
				function markArchivedInfo(view) {
					var candidates = view.querySelectorAll('span, p, div');
					for (var i = 0; i < candidates.length; i++) {
						var candidate = candidates[i];
						var text = (candidate.textContent || '').replace(/\s+/g, ' ').trim();
						if (text.length < 40 || text.length > 240) continue;
						if (privacyIsArchivedInfoText(text)) {
							candidate.setAttribute('data-wa-privacy-archived-info', '1');
							candidate.style.setProperty('filter', 'none', 'important');
							var children = candidate.querySelectorAll('*');
							for (var c = 0; c < children.length; c++) children[c].style.setProperty('filter', 'none', 'important');
					}
				}
			}
				function privacyLooksLikeArchivedView(anchor) {
					if (!anchor || !anchor.matches) return false;
					if (anchor.getAttribute('data-wa-privacy-archived-view') === '1' ||
						anchor.matches('[aria-label*="archiv" i], [aria-label*="diarsip" i], [data-testid*="archiv" i], [role="heading"], h1, h2, h3')) return true;
					var parent = anchor;
					for (var depth = 0; parent && depth < 8; depth++, parent = parent.parentElement) {
						if (parent.querySelector && parent.querySelector('[data-icon*="back" i], [data-testid*="back" i], [aria-label*="back" i], [aria-label*="kembali" i]')) return true;
					}
					return false;
				}
				var anchors = document.querySelectorAll('[data-wa-privacy-archived-view="1"], [aria-label*="archiv" i], [aria-label*="diarsip" i], [data-testid*="archiv" i], [role="heading"], h1, h2, h3, #side span, #pane-side span');
				for (var i = 0; i < anchors.length; i++) {
					var anchor = anchors[i];
					var label = (anchor.getAttribute('aria-label') || '').trim();
					var text = (anchor.textContent || '').trim();
					if (!/(archiv|diarsip)/i.test(label) && !privacyIsArchivedNavigationText(text) && anchor.getAttribute('data-wa-privacy-archived-view') !== '1') continue;
					if ((anchor.matches('#side span, #pane-side span')) && !privacyLooksLikeArchivedView(anchor)) continue;
					var view = anchor;
					for (var depth = 0; view && depth < 8; depth++, view = view.parentElement) {
						var hasChatRows = view.querySelector && view.querySelector(rowSelector);
						var hasChatVisuals = view.querySelectorAll && view.querySelectorAll('img, image').length >= 2 && view.querySelectorAll('span').length >= 2;
						if (hasChatRows || hasChatVisuals) {
							view.setAttribute('data-wa-privacy-archived-view', '1');
							if (view.getAttribute('data-wa-privacy-archived-info-checked') !== '1') {
								markArchivedInfo(view);
								view.setAttribute('data-wa-privacy-archived-info-checked', '1');
							}
							markRowsInView(view);
							break;
						}
					}
				}
				flushPrivacyAvatarGeometry();
			}
			function scheduleArchivedPrivacyMark() {
				if (!isPrivacyActive && !(window.isBlurChatListOnly && window.isBlurChatListOnly())) return;
				[0, 100, 300].forEach(function(delay) {
					setTimeout(function() {
						if (isPrivacyActive || (window.isBlurChatListOnly && window.isBlurChatListOnly())) markArchivedPrivacyViews();
					}, delay);
				});
			}
			document.addEventListener('click', function(e) {
				var target = e.target && e.target.closest ? e.target.closest('[data-icon*="archive" i], [data-testid*="archive" i], [aria-label*="archiv" i], [aria-label*="diarsip" i]') : null;
				if (target || isPrivacySidebarControl(e.target)) scheduleArchivedPrivacyMark();
			}, true);
			document.addEventListener('mouseover', function(e) {
				if (isPrivacyActive || (window.isBlurChatListOnly && window.isBlurChatListOnly())) {
					updatePrivacyHoverFromTarget(e.target);
				}
			}, true);
			// Deliberately no mousemove handler. It fires on every pixel of travel,
			// and each call walked the ancestor chain asking whether each level was
			// the Archived entry. A passive scroll listener covers the one case it
			// was really needed for - the list moving under a stationary cursor -
			// at one attribute removal per frame instead of per pixel.
			var privacyScrollFrame = 0;
			document.addEventListener('scroll', function() {
				if ((!isPrivacyActive && !(window.isBlurChatListOnly && window.isBlurChatListOnly())) || !activePrivacyHoverRow || privacyScrollFrame) return;
				if (!window.requestAnimationFrame) { clearPrivacyHoverRow(); return; }
				privacyScrollFrame = window.requestAnimationFrame(function() {
					privacyScrollFrame = 0;
					clearPrivacyHoverRow();
				});
			}, { passive: true, capture: true });
			document.addEventListener('mouseout', function(e) {
				if (!isPrivacyActive && !(window.isBlurChatListOnly && window.isBlurChatListOnly())) return;
				var row = privacyChatRowFromTarget(e.target);
				if (row && (!e.relatedTarget || !row.contains(e.relatedTarget))) clearPrivacyHoverRow();
			}, true);
			var privacySidebarRefreshTimer = null;
			function schedulePrivacySidebarRefresh() {
				clearTimeout(privacySidebarRefreshTimer);
				privacySidebarRefreshTimer = setTimeout(function() {
					privacySidebarRefreshTimer = null;
					if (!isPrivacyActive && !(window.isBlurChatListOnly && window.isBlurChatListOnly())) return;
					// WhatsApp's virtualized chat list mutates continuously while the
					// user scrolls, so this ran a full re-scan several times a second
					// during the gesture. Reschedule instead of dropping: the markers
					// must still land the moment the gesture ends, or a chat row could
					// stay readable.
					if (shouldPauseBackgroundWork()) {
						if (!document.hidden) schedulePrivacySidebarRefresh();
						return;
					}
					markPrivacyChatRows();
					markArchivedPrivacyViews();
					forceArchivedControlVisible();
				}, 100);
			}
			var privacySidebarObserver = new MutationObserver(function() {
				if (isPrivacyActive || (window.isBlurChatListOnly && window.isBlurChatListOnly())) schedulePrivacySidebarRefresh();
			});
			function observePrivacySidebar() {
				var side = document.getElementById('side') || document.getElementById('pane-side');
				if (side) privacySidebarObserver.observe(side, { childList: true, subtree: true });
			}
			observePrivacySidebar();

			function ensurePrivacyStyle() {
				if (!document.getElementById('whatsapp-privacy-style')) {
					var h = document.head || document.documentElement;
					if (h) h.appendChild(styleEl);
				}
			}
			ensurePrivacyStyle();

			function applyPrivacyMode(active, silent) {
				isPrivacyActive = !!active;
				// State lives on <html>, never on WhatsApp's mutable <body>.
				// All privacy selectors are descendant selectors, so they
				// match identically from the <html> ancestor.
				var rootEl = document.documentElement;
				if (!rootEl) return isPrivacyActive;
				if (isPrivacyActive) {
					ensurePrivacyStyle();
					rootEl.classList.add('privacy-mode');
					markPrivacyChatRows();
					markArchivedPrivacyViews();
					forceArchivedControlVisible();
					observePrivacySidebar();
					if (!silent) {
						showFloatingToast((window.isBlurChatListOnly && window.isBlurChatListOnly()) ? '🔒 Chat list blur: on' : '🔒 Privacy Mode: Enabled');
					}
				} else {
					rootEl.classList.remove('privacy-mode');
					if (!silent) {
						showFloatingToast((window.isBlurChatListOnly && window.isBlurChatListOnly()) ? '🔓 Chat list blur: off' : '🔓 Privacy Mode: Disabled');
					}
				}
				return isPrivacyActive;
			}

			window.togglePrivacyMode = function() {
				autoLocked = false;
				return applyPrivacyMode(!isPrivacyActive, false);
			};
			window.isPrivacyModeActive = function() {
				return isPrivacyActive;
			};

			// "Blur profile photos" setting: gates the .blur-avatars layer.
			// Applied on <html> next to .privacy-mode; persisted natively.
			window.isBlurAvatars = function() {
				return !!(document.documentElement && document.documentElement.classList && document.documentElement.classList.contains('blur-avatars'));
			};

			// Timestamp sparing: tag short clock/day strings so the CSS above
			// can exclude them via :not([data-wa-time]). textContent never
			// forces layout; each span is visited once (__waTimeSeen); the
			// :not() selector keeps repeat runs cheap. Ticks at most every 3s,
			// only while privacy is on and the page is visible, so the steady
			// state cost is ~zero. Attribute writes don't trip the childList
			// observers, so this can't feed an observer loop.
			var WA_TIME_RE = /^(\d{1,2}:\d{2}(\s?(AM|PM))?|Today|Yesterday|Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday|Hari ini|Kemarin|Senin|Selasa|Rabu|Kamis|Jumat|Sabtu|Minggu)$/i;
			function tagTimesIn(root) {
				if (!root || !root.querySelectorAll) return;
				var spans = root.querySelectorAll('span:not([data-wa-time])');
				var n = 0;
				for (var i = 0; i < spans.length && n < 250; i++) {
					var s = spans[i];
					if (s.__waTimeSeen) continue;
					s.__waTimeSeen = true;
					n++;
					try {
						var t = (s.textContent || '').trim();
						if (WA_TIME_RE.test(t)) s.setAttribute('data-wa-time', '1');
					} catch (e) {}
				}
			}
			setInterval(function() {
				if ((!isPrivacyActive && !(window.isBlurChatListOnly && window.isBlurChatListOnly())) || shouldPauseBackgroundWork()) return;
				markPrivacyChatRows();
				markArchivedPrivacyViews();
				forceArchivedControlVisible();
				tagTimesIn(document.getElementById('main'));
				tagTimesIn(document.getElementById('side') || document.getElementById('pane-side'));
			}, 3000);
			window.setBlurAvatars = function(on) {
				on = !!on;
				if (document.documentElement && document.documentElement.classList) {
					if (on) document.documentElement.classList.add('blur-avatars');
					else document.documentElement.classList.remove('blur-avatars');
				}
				if (window.setBlurAvatarsNative) {
					Promise.resolve(window.setBlurAvatarsNative(on)).catch(function() {});
				}
				return on;
			};
			if (window.getBlurAvatarsNative) {
				window.getBlurAvatarsNative().then(function(on) {
					if (on && document.documentElement && document.documentElement.classList) {
						document.documentElement.classList.add('blur-avatars');
					}
				}).catch(function() {});
			}

			// "Blur chat list only" setting: blurs sidebar chat list while keeping
			// opened chat conversation clear and readable.
			window.isBlurChatListOnly = function() {
				return !!(document.documentElement && document.documentElement.classList && document.documentElement.classList.contains('blur-chatlist-only'));
			};
			window.setBlurChatListOnly = function(on) {
				on = !!on;
				if (document.documentElement && document.documentElement.classList) {
					if (on) {
						document.documentElement.classList.add('blur-chatlist-only');
						ensurePrivacyStyle();
						markPrivacyChatRows();
						observePrivacySidebar();
					} else {
						document.documentElement.classList.remove('blur-chatlist-only');
					}
				}
				if (window.setBlurChatListOnlyNative) {
					Promise.resolve(window.setBlurChatListOnlyNative(on)).catch(function() {});
				}
				return on;
			};
			if (window.getBlurChatListOnlyNative) {
				window.getBlurChatListOnlyNative().then(function(on) {
					if (on && document.documentElement && document.documentElement.classList) {
						document.documentElement.classList.add('blur-chatlist-only');
						ensurePrivacyStyle();
						markPrivacyChatRows();
						observePrivacySidebar();
					}
				}).catch(function() {});
			}

			// Auto-lock when window loses focus: blurs when window loses focus or
			// is hidden, and unblurs upon regaining focus or interaction.
			// Persisted in localStorage so it survives reloads.
			var AUTO_LOCK_KEY = 'wa_desk_privacy_autolock';
			var autoLockEnabled = storageGet(AUTO_LOCK_KEY) === '1';
			var autoLocked = false;

			function isAutoLockEnabled() { return autoLockEnabled; }
			function setAutoLockEnabled(on) {
				autoLockEnabled = !!on;
				storageSet(AUTO_LOCK_KEY, autoLockEnabled ? '1' : '0');
				if (!autoLockEnabled && autoLocked) unlockOnFocus();
				return autoLockEnabled;
			}
			window.isAutoLockEnabled = isAutoLockEnabled;
			window.setAutoLockEnabled = setAutoLockEnabled;
			window.isPrivacyAutoLock = isAutoLockEnabled;
			window.setPrivacyAutoLock = setAutoLockEnabled;

			function lockOnBlur() {
				if (!autoLockEnabled || autoLocked || isPrivacyActive) return;
				autoLocked = true;
				applyPrivacyMode(true, true);
			}
			function unlockOnFocus() {
				if (!autoLocked) return;
				autoLocked = false;
				applyPrivacyMode(false, true);
			}

			window.addEventListener('blur', function() {
				if (autoLockEnabled) lockOnBlur();
			});
			window.addEventListener('focus', function() {
				if (autoLocked) unlockOnFocus();
			});
			window.addEventListener('mousedown', function() {
				if (autoLocked) unlockOnFocus();
			}, true);
			document.addEventListener('visibilitychange', function() {
				if (document.hidden) {
					if (autoLockEnabled) lockOnBlur();
				} else {
					if (autoLocked) unlockOnFocus();
					if (isPrivacyActive || (window.isBlurChatListOnly && window.isBlurChatListOnly())) schedulePrivacySidebarRefresh();
				}
			});

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'p' || e.key === 'P')) {
					e.preventDefault();
					e.stopPropagation();
					window.togglePrivacyMode();
				}
			}, true);
		});

		// Always on Top Toggle (Cmd/Ctrl + Shift + T)
		waRunModule('always-on-top', function() {
			var isPinnedState = false;
			window.toggleAlwaysOnTop = function() {
				if (window.toggleAlwaysOnTopNative) {
					return window.toggleAlwaysOnTopNative().then(function(isPinned) {
						isPinnedState = isPinned;
						showFloatingToast(isPinned ? '📌 Always on Top: Enabled' : '📌 Always on Top: Disabled');
						return isPinned;
					});
				}
				return Promise.resolve(false);
			};
			window.isAlwaysOnTopActive = function() {
				return isPinnedState;
			};

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 't' || e.key === 'T')) {
					e.preventDefault();
					e.stopPropagation();
					window.toggleAlwaysOnTop();
				}
			}, true);
		});

		// Reload and Refresh Functions (Cmd/Ctrl + R, Cmd/Ctrl + Shift + R, F5)
		window.reloadWhatsApp = function() {
			showFloatingToast('🔄 Reloading conversation...');
			setTimeout(function() { window.location.reload(); }, 200);
		};
		window.hardRefreshWhatsApp = function() {
			showFloatingToast('⚡ Hard refresh (clearing cache)...');
			try {
				if (window.caches && caches.keys) {
					caches.keys().then(function(names) {
						names.forEach(function(name) { caches.delete(name); });
					});
				}
			} catch (e) {}
			setTimeout(function() {
				window.location.href = window.location.origin + window.location.pathname + '?_t=' + Date.now();
			}, 200);
		};

		window.addEventListener('keydown', function(e) {
			if (e.key === 'F5' || ((e.metaKey || e.ctrlKey) && (e.key === 'r' || e.key === 'R') && !e.shiftKey && !e.altKey)) {
				e.preventDefault();
				e.stopPropagation();
				window.reloadWhatsApp();
			} else if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'r' || e.key === 'R')) {
				e.preventDefault();
				e.stopPropagation();
				window.hardRefreshWhatsApp();
			}
		}, true);

		// Audio Mute Toggle (Cmd/Ctrl + Shift + M)
		waRunModule('audio-mute', function() {
			var isMuted = false;
			window.toggleMuteAudio = function() {
				isMuted = !isMuted;
				document.querySelectorAll('audio, video').forEach(function(el) {
					el.muted = isMuted;
				});
				showFloatingToast(isMuted ? '🔇 Notification Audio: Muted' : '🔊 Notification Audio: Unmuted');
				return isMuted;
			};
			window.isAudioMuted = function() {
				return isMuted;
			};

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'm' || e.key === 'M')) {
					e.preventDefault();
					e.stopPropagation();
					window.toggleMuteAudio();
				}
			}, true);
			document.addEventListener('play', function(e) {
				if (isMuted && e.target && (e.target.tagName === 'AUDIO' || e.target.tagName === 'VIDEO')) {
					e.target.muted = true;
				}
			}, true);
		});

		// App Lock (PIN / Passcode Protection)
		waRunModule('app-lock', function() {
			var isLocked = false;
			var hasPasscode = false;
			var isLockEnabled = false;
			var isFocusLossLock = false;

			function refreshLockConfig() {
				if (!window.getAppLockEnabledNative) return Promise.resolve();
				return Promise.all([
					Promise.resolve(window.getAppLockEnabledNative()),
					Promise.resolve(window.hasAppLockPasscodeNative()),
					Promise.resolve(window.getAppLockOnFocusLossNative())
				]).then(function(res) {
					isLockEnabled = !!res[0];
					hasPasscode = !!res[1];
					isFocusLossLock = !!res[2];
					return { enabled: isLockEnabled, hasPasscode: hasPasscode, focusLoss: isFocusLossLock };
				}).catch(function() {});
			}

			window.isAppLocked = function() {
				return isLocked;
			};
			window.hasAppLockPasscode = function() {
				return hasPasscode;
			};
			window.isAppLockEnabled = function() {
				return isLockEnabled;
			};
			window.isAppLockOnFocusLoss = function() {
				return isFocusLossLock;
			};
			window.refreshAppLockState = refreshLockConfig;

			function mountLockScreen() {
				if (document.getElementById('wa-app-lock-screen')) {
					var existingInput = document.getElementById('wa-lock-pin-input');
					if (existingInput) existingInput.focus();
					return;
				}
				isLocked = true;

				var screen = document.createElement('div');
				screen.id = 'wa-app-lock-screen';
				screen.style.cssText = 'position:fixed;inset:0;background:#111b21;z-index:2147483646;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:20px;box-sizing:border-box;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;color:#e9edef;user-select:none;';

				var card = document.createElement('div');
				card.style.cssText = 'width:340px;max-width:92vw;background:#202c33;border-radius:12px;padding:32px 28px 28px;box-sizing:border-box;box-shadow:0 12px 36px rgba(0,0,0,0.5);display:flex;flex-direction:column;align-items:center;text-align:center;border:1px solid rgba(255,255,255,0.08);';

				var iconWrap = document.createElement('div');
				iconWrap.style.cssText = 'width:56px;height:56px;border-radius:50%;background:rgba(0,168,132,0.15);color:#00a884;display:flex;align-items:center;justify-content:center;font-size:26px;margin-bottom:16px;';
				iconWrap.innerHTML = '🔒';

				var title = document.createElement('h2');
				title.style.cssText = 'margin:0 0 6px;font-size:18px;font-weight:600;color:#e9edef;';
				title.textContent = 'WhatsApp Desk';

				var subtitle = document.createElement('p');
				subtitle.style.cssText = 'margin:0 0 20px;font-size:12.5px;color:#8696a0;';
				subtitle.textContent = 'Aplikasi terkunci. Masukkan PIN Anda.';

				var input = document.createElement('input');
				input.id = 'wa-lock-pin-input';
				input.type = 'password';
				input.maxLength = 32;
				input.placeholder = '••••';
				input.autocomplete = 'off';
				input.style.cssText = 'width:100%;height:44px;background:#111b21;border:1px solid #33434c;border-radius:8px;padding:0 12px;font-size:18px;letter-spacing:4px;text-align:center;color:#e9edef;box-sizing:border-box;outline:none;transition:border-color 0.15s ease;';

				input.onfocus = function() { input.style.borderColor = '#00a884'; };
				input.onblur = function() { input.style.borderColor = '#33434c'; };

				var errorMsg = document.createElement('div');
				errorMsg.id = 'wa-lock-error-msg';
				errorMsg.style.cssText = 'color:#f15c6d;font-size:11.5px;min-height:18px;margin-top:8px;font-weight:500;';

				var submitBtn = document.createElement('button');
				submitBtn.id = 'wa-lock-unlock-btn';
				submitBtn.textContent = 'Buka Kunci';
				submitBtn.style.cssText = 'width:100%;height:40px;background:#00a884;color:#111b21;border:none;border-radius:8px;font-size:13px;font-weight:600;cursor:pointer;margin-top:14px;transition:opacity 0.15s ease;';

				function doUnlock() {
					var pin = input.value;
					if (!pin) {
						errorMsg.textContent = 'Masukkan PIN terlebih dahulu';
						input.focus();
						return;
					}
					if (!window.verifyAppLockPasscodeNative) {
						if (screen.parentNode) screen.parentNode.removeChild(screen);
						isLocked = false;
						return;
					}
					Promise.resolve(window.verifyAppLockPasscodeNative(pin)).then(function(valid) {
						if (valid) {
							isLocked = false;
							if (screen.parentNode) screen.parentNode.removeChild(screen);
							if (typeof window.showFloatingToast === 'function') {
								window.showFloatingToast('🔓 WhatsApp Desk terbuka');
							}
							if (window.__waPendingNotificationClick) {
								var pending = window.__waPendingNotificationClick;
								window.__waPendingNotificationClick = null;
								setTimeout(function() {
									if (typeof window.__waOnNotificationClicked === 'function') {
										window.__waOnNotificationClicked(pending.notifId, pending.title);
									}
								}, 100);
							}
						} else {
							errorMsg.textContent = 'PIN salah. Coba lagi.';
							input.value = '';
							input.style.borderColor = '#f15c6d';
							input.focus();
						}
					}).catch(function() {
						errorMsg.textContent = 'Gagal memverifikasi PIN.';
					});
				}

				submitBtn.onclick = doUnlock;
				input.onkeydown = function(e) {
					if (e.key === 'Enter') {
						e.preventDefault();
						doUnlock();
					}
				};

				card.appendChild(iconWrap);
				card.appendChild(title);
				card.appendChild(subtitle);
				card.appendChild(input);
				card.appendChild(errorMsg);
				card.appendChild(submitBtn);
				screen.appendChild(card);

				var mountTarget = document.body || document.documentElement;
				if (mountTarget) mountTarget.appendChild(screen);

				setTimeout(function() { input.focus(); }, 50);
			}

			window.lockApp = function() {
				refreshLockConfig().then(function() {
					if (hasPasscode) {
						mountLockScreen();
					}
				});
			};

			// Prevent hotkeys from triggering actions behind lock screen
			window.addEventListener('keydown', function(e) {
				if (isLocked) {
					if (e.target && e.target.id === 'wa-lock-pin-input') {
						return;
					}
					e.preventDefault();
					e.stopPropagation();
					var inp = document.getElementById('wa-lock-pin-input');
					if (inp) inp.focus();
				}
			}, true);

			// Focus loss lock
			window.addEventListener('blur', function() {
				if (isLockEnabled && isFocusLossLock && !isLocked && hasPasscode) {
					mountLockScreen();
				}
			});
			document.addEventListener('visibilitychange', function() {
				if (document.hidden && isLockEnabled && isFocusLossLock && !isLocked && hasPasscode) {
					mountLockScreen();
				}
			});

			// Shortcut: Cmd+L (Mac) / Ctrl+L (Windows)
			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && (e.key === 'l' || e.key === 'L')) {
					e.preventDefault();
					e.stopPropagation();
					if (isLocked) return;
					refreshLockConfig().then(function() {
						if (hasPasscode) {
							mountLockScreen();
						} else {
							if (typeof window.showSettingsModal === 'function') {
								window.showSettingsModal();
							}
							if (typeof window.promptConfigureAppLock === 'function') {
								window.promptConfigureAppLock();
							}
						}
					});
				}
			}, true);

			// Configure PIN Dialog (Set PIN / Change PIN / Disable PIN)
			window.promptConfigureAppLock = function(onDone) {
				if (document.getElementById('wa-pin-config-modal')) return;
				refreshLockConfig().then(function() {
					var modalOverlay = document.createElement('div');
					modalOverlay.id = 'wa-pin-config-modal';
					modalOverlay.style.cssText = 'position:fixed;inset:0;background:rgba(8,15,19,.75);z-index:2147483647;display:flex;align-items:center;justify-content:center;padding:16px;box-sizing:border-box;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;color:#e9edef;';

					var box = document.createElement('div');
					box.style.cssText = 'width:360px;max-width:94vw;background:#202c33;border-radius:10px;padding:24px;box-sizing:border-box;border:1px solid rgba(255,255,255,0.1);box-shadow:0 12px 32px rgba(0,0,0,.45);';

					var heading = document.createElement('h3');
					heading.style.cssText = 'margin:0 0 6px;font-size:15px;font-weight:600;color:#e9edef;';
					heading.textContent = hasPasscode ? 'Ubah / Hapus PIN App Lock' : 'Atur PIN App Lock';

					var desc = document.createElement('p');
					desc.style.cssText = 'margin:0 0 16px;font-size:11.5px;color:#8696a0;';
					desc.textContent = hasPasscode ? 'Masukkan PIN lama dan PIN baru (kosongkan PIN baru untuk menonaktifkan PIN).' : 'Masukkan PIN baru untuk mengunci aplikasi.';

					var formWrap = document.createElement('div');
					formWrap.style.cssText = 'display:flex;flex-direction:column;gap:10px;';

					var oldInput = null;
					if (hasPasscode) {
						var oldLbl = document.createElement('label');
						oldLbl.style.cssText = 'font-size:11px;color:#8696a0;display:block;margin-bottom:2px;';
						oldLbl.textContent = 'PIN Lama:';
						oldInput = document.createElement('input');
						oldInput.id = 'wa-pin-old-input';
						oldInput.type = 'password';
						oldInput.maxLength = 32;
						oldInput.placeholder = 'PIN lama saat ini...';
						oldInput.style.cssText = 'width:100%;height:36px;background:#111b21;border:1px solid #33434c;border-radius:6px;padding:0 10px;font-size:13px;color:#e9edef;box-sizing:border-box;outline:none;';
						formWrap.appendChild(oldLbl);
						formWrap.appendChild(oldInput);
					}

					var newLbl = document.createElement('label');
					newLbl.style.cssText = 'font-size:11px;color:#8696a0;display:block;margin-bottom:2px;';
					newLbl.textContent = hasPasscode ? 'PIN Baru (kosongkan untuk hapus):' : 'PIN Baru:';
					var newInput = document.createElement('input');
					newInput.id = 'wa-pin-new-input';
					newInput.type = 'password';
					newInput.maxLength = 32;
					newInput.placeholder = hasPasscode ? 'Kosongkan jika ingin menghapus...' : 'Ketik PIN baru...';
					newInput.style.cssText = 'width:100%;height:36px;background:#111b21;border:1px solid #33434c;border-radius:6px;padding:0 10px;font-size:13px;color:#e9edef;box-sizing:border-box;outline:none;';
					formWrap.appendChild(newLbl);
					formWrap.appendChild(newInput);

					var confirmLbl = document.createElement('label');
					confirmLbl.style.cssText = 'font-size:11px;color:#8696a0;display:block;margin-bottom:2px;';
					confirmLbl.textContent = 'Konfirmasi PIN Baru:';
					var confirmInput = document.createElement('input');
					confirmInput.id = 'wa-pin-confirm-input';
					confirmInput.type = 'password';
					confirmInput.maxLength = 32;
					confirmInput.placeholder = 'Ulangi PIN baru...';
					confirmInput.style.cssText = 'width:100%;height:36px;background:#111b21;border:1px solid #33434c;border-radius:6px;padding:0 10px;font-size:13px;color:#e9edef;box-sizing:border-box;outline:none;';
					formWrap.appendChild(confirmLbl);
					formWrap.appendChild(confirmInput);

					var errDiv = document.createElement('div');
					errDiv.style.cssText = 'color:#f15c6d;font-size:11.5px;min-height:16px;margin-top:6px;';

					var actions = document.createElement('div');
					actions.style.cssText = 'display:flex;justify-content:flex-end;gap:8px;margin-top:16px;';

					var cancelBtn = document.createElement('button');
					cancelBtn.textContent = 'Batal';
					cancelBtn.style.cssText = 'background:transparent;border:1px solid rgba(255,255,255,0.15);color:#8696a0;padding:6px 14px;border-radius:6px;font-size:12px;cursor:pointer;';
					cancelBtn.onclick = function() {
						modalOverlay.parentNode.removeChild(modalOverlay);
					};

					var saveBtn = document.createElement('button');
					saveBtn.textContent = 'Simpan';
					saveBtn.style.cssText = 'background:#00a884;border:none;color:#111b21;padding:6px 16px;border-radius:6px;font-size:12px;font-weight:600;cursor:pointer;';
					saveBtn.onclick = function() {
						var oldVal = oldInput ? oldInput.value : '';
						var newVal = newInput.value;
						var confVal = confirmInput.value;

						if (newVal !== confVal) {
							errDiv.textContent = 'Konfirmasi PIN tidak cocok!';
							return;
						}
						if (!hasPasscode && !newVal) {
							errDiv.textContent = 'PIN baru tidak boleh kosong!';
							return;
						}
						if (!window.setAppLockPasscodeNative) {
							errDiv.textContent = 'Bridge native tidak tersedia';
							return;
						}
						Promise.resolve(window.setAppLockPasscodeNative(oldVal, newVal)).then(function(ok) {
							if (ok) {
								modalOverlay.parentNode.removeChild(modalOverlay);
								refreshLockConfig().then(function() {
									if (typeof onDone === 'function') onDone();
									if (typeof window.showFloatingToast === 'function') {
										window.showFloatingToast(newVal ? '🔒 PIN App Lock berhasil disimpan!' : '🔓 PIN App Lock dinonaktifkan');
									}
								});
							} else {
								errDiv.textContent = 'PIN lama salah!';
							}
						}).catch(function() {
							errDiv.textContent = 'Terjadi kesalahan sistem';
						});
					};

					actions.appendChild(cancelBtn);
					actions.appendChild(saveBtn);

					box.appendChild(heading);
					box.appendChild(desc);
					box.appendChild(formWrap);
					box.appendChild(errDiv);
					box.appendChild(actions);
					modalOverlay.appendChild(box);

					document.body.appendChild(modalOverlay);
					if (oldInput) oldInput.focus();
					else newInput.focus();
				});
			};

			// Initial startup check
			refreshLockConfig().then(function(cfg) {
				if (cfg && cfg.enabled && cfg.hasPasscode) {
					mountLockScreen();
				}
			});
		});

		// Capture diagnostics (issue #57: "the other person can't hear our voice
		// clearly, it's broken like a robot").
		//
		// The app never intercepted capture, so that report had no evidence to
		// work from - only a guess about Chrome-shaped WebRTC paths running on
		// WebKit. This records what the page asks for and what the engine
		// actually grants, so the next report can be answered with numbers.
		//
		// Deliberately a pass-through. The original function is called with the
		// caller's own receiver and arguments, and its return value is handed
		// back untouched. Forcing mono or switching audio processing off here
		// would be an unverified change to every call, and a wrong guess makes
		// calls worse, not better.
		waRunModule('capture-diagnostics', function() {
			var md = navigator.mediaDevices;
			if (!md || typeof md.getUserMedia !== 'function' || md.__waCaptureProbe) return;
			var original = md.getUserMedia;

			function describeConstraints(c) {
				try {
					if (c === undefined || c === null) return 'none';
					if (typeof c !== 'object') return String(c);
					var parts = [];
					['audio', 'video'].forEach(function(k) {
						var v = c[k];
						if (v === undefined) return;
						if (v === true || v === false || typeof v === 'string') {
							parts.push(k + '=' + String(v));
							return;
						}
						if (typeof v !== 'object') { parts.push(k + '=?'); return; }
						var inner = [];
						Object.keys(v).forEach(function(f) {
							inner.push(f + ':' + String(v[f]));
						});
						parts.push(k + '={' + inner.join(',') + '}');
					});
					return parts.join(' ') || 'empty';
				} catch (e) {
					return 'unreadable';
				}
			}
			function describeStream(stream) {
				try {
					if (!stream || typeof stream.getAudioTracks !== 'function') return 'no stream';
					var tracks = stream.getAudioTracks();
					if (!tracks.length) return 'no audio track';
					var out = [];
					for (var i = 0; i < tracks.length; i++) {
						var s = {};
						try { s = (tracks[i].getSettings && tracks[i].getSettings()) || {}; } catch (e) {}
						out.push('ch=' + s.channelCount + ' rate=' + s.sampleRate +
							' ec=' + s.echoCancellation + ' ns=' + s.noiseSuppression +
							' agc=' + s.autoGainControl +
							' device=' + String(tracks[i].label || '?').slice(0, 60));
					}
					return out.join(' | ');
				} catch (e) {
					return 'unreadable';
				}
			}

			md.getUserMedia = function() {
				var args = arguments;
				try { waDiag('mic', 'getUserMedia constraints: ' + describeConstraints(args[0])); } catch (e) {}
				var result = original.apply(md, args);
				try {
					if (result && typeof result.then === 'function') {
						result.then(function(stream) {
							try { waDiag('mic', 'granted: ' + describeStream(stream)); } catch (e) {}
						}, function(err) {
							try { waDiag('mic', 'rejected: ' + String((err && err.name) || err)); } catch (e) {}
						});
					}
				} catch (e) {}
				return result;
			};
			md.__waCaptureProbe = true;
		});

		// Auto-Start at Login Toggle (Cmd/Ctrl + Shift + S)
		waRunModule('launch-on-boot', function() {
			var isAutoStartState = false;
			function refreshAutoStartState() {
				if (window.getAutoStartNative) {
					return Promise.resolve(window.getAutoStartNative()).then(function(val) {
						isAutoStartState = !!val;
						return isAutoStartState;
					}).catch(function() { return isAutoStartState; });
				}
				return Promise.resolve(isAutoStartState);
			}
			window.refreshAutoStartState = refreshAutoStartState;
			refreshAutoStartState();

			window.toggleAutoStart = function() {
				if (window.toggleAutoStartNative) {
					return window.toggleAutoStartNative().then(function(isEnabled) {
						isAutoStartState = isEnabled;
						showFloatingToast(isEnabled ? '🚀 Launch on Boot: Enabled' : '🚀 Launch on Boot: Disabled');
						return isEnabled;
					});
				}
				return Promise.resolve(false);
			};
			window.isAutoStartActive = function() {
				return isAutoStartState;
			};

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 's' || e.key === 'S')) {
					e.preventDefault();
					e.stopPropagation();
					window.toggleAutoStart();
				}
			}, true);
		});

		// In-App Auto Updater UI and Handlers
		waRunModule('app-updater', function() {
			window.showUpdateBanner = function(latestVersion, releaseTitle, downloadUrl) {
				if (document.getElementById('wa-update-banner')) return;
				try {
					if (sessionStorage.getItem('dismissed_update_' + latestVersion) === 'true') return;
				} catch (e) {}

				if (!document.getElementById('wa-update-anim')) {
					var animStyle = document.createElement('style');
					animStyle.id = 'wa-update-anim';
					animStyle.textContent = '@keyframes waSlideDown { from { transform: translateY(-100%); opacity: 0; } to { transform: translateY(0); opacity: 1; } }' +
						'html.wa-update-visible #app { height: calc(100% - var(--wa-update-banner-height, 0px)) !important; margin-top: var(--wa-update-banner-height, 0px) !important; }' +
						'#wa-btn-update:hover { background: #029070 !important; transform: translateY(-1px); }' +
						'#wa-btn-dismiss:hover { color: #e9edef !important; }';
					document.head.appendChild(animStyle);
				}

				var banner = document.createElement('div');
				banner.id = 'wa-update-banner';
				banner.style.cssText = 'position:fixed;top:0;left:0;right:0;box-sizing:border-box;min-height:50px;background:rgba(17,27,33,0.97);backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);border-bottom:1px solid rgba(0,168,132,0.35);padding:9px 18px;display:flex;align-items:center;justify-content:space-between;gap:12px;z-index:9999998;box-shadow:0 6px 24px rgba(0,0,0,0.6);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;color:#e9edef;font-size:13px;animation:waSlideDown 0.25s cubic-bezier(0.16,1,0.3,1);';

				var leftWrap = document.createElement('div');
				leftWrap.style.cssText = 'display:flex;align-items:center;gap:10px;min-width:0;flex:1;';

				var badge = document.createElement('span');
				badge.style.cssText = 'background:rgba(0,168,132,0.15);color:#00a884;border:1px solid rgba(0,168,132,0.35);padding:2px 8px;border-radius:12px;font-size:11px;font-weight:600;letter-spacing:0.3px;flex-shrink:0;';
				badge.textContent = 'v' + latestVersion;

				var msg = document.createElement('span');
				msg.id = 'wa-update-text';
				msg.style.cssText = 'white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-size:12.5px;color:#d1d7db;';
				var titleText = releaseTitle ? releaseTitle : ('WhatsApp Desk v' + latestVersion);
				msg.innerHTML = 'Update available: <strong style="color:#e9edef;">' + escapeHtml(titleText) + '</strong>';

				leftWrap.appendChild(badge);
				leftWrap.appendChild(msg);

				var rightWrap = document.createElement('div');
				rightWrap.style.cssText = 'display:flex;align-items:center;gap:8px;flex-shrink:0;';

				var actionsDiv = document.createElement('div');
				actionsDiv.id = 'wa-update-actions';
				actionsDiv.style.cssText = 'display:flex;align-items:center;gap:8px;';

				var btnUpdate = document.createElement('button');
				btnUpdate.id = 'wa-btn-update';
				btnUpdate.textContent = 'Update Now';
				btnUpdate.style.cssText = 'background:#00a884;color:#111b21;border:none;padding:5px 14px;border-radius:14px;font-size:12px;font-weight:600;cursor:pointer;outline:none;transition:all 0.15s ease;box-shadow:0 2px 8px rgba(0,168,132,0.3);';

				var btnDismiss = document.createElement('button');
				btnDismiss.id = 'wa-btn-dismiss';
				btnDismiss.textContent = 'Later';
				btnDismiss.style.cssText = 'background:transparent;color:#8696a0;border:none;padding:5px 10px;border-radius:14px;font-size:12px;cursor:pointer;outline:none;transition:color 0.15s ease;';

				actionsDiv.appendChild(btnUpdate);
				actionsDiv.appendChild(btnDismiss);

				var progressWrap = document.createElement('div');
				progressWrap.id = 'wa-update-progress-wrap';
				progressWrap.style.cssText = 'display:none;align-items:center;gap:10px;';

				var barTrack = document.createElement('div');
				barTrack.style.cssText = 'width:130px;height:6px;background:rgba(255,255,255,0.12);border-radius:3px;overflow:hidden;';

				var barFill = document.createElement('div');
				barFill.id = 'wa-update-progress-bar';
				barFill.style.cssText = 'width:0%;height:100%;background:#00a884;border-radius:3px;transition:width 0.18s ease;';
				barTrack.appendChild(barFill);

				var pctLabel = document.createElement('span');
				pctLabel.id = 'wa-update-progress-pct';
				pctLabel.style.cssText = 'font-size:11.5px;color:#00a884;font-weight:600;min-width:32px;text-align:right;';
				pctLabel.textContent = '0%';

				progressWrap.appendChild(barTrack);
				progressWrap.appendChild(pctLabel);

				rightWrap.appendChild(actionsDiv);
				rightWrap.appendChild(progressWrap);

				banner.appendChild(leftWrap);
				banner.appendChild(rightWrap);
				var bannerResizeObserver = null;
				var bannerParent = document.body || document.documentElement;
				if (bannerParent) {
					bannerParent.appendChild(banner);
					var layoutRoot = document.documentElement;
					var syncBannerLayout = function() {
						if (!banner.isConnected || !layoutRoot) return;
						layoutRoot.style.setProperty('--wa-update-banner-height', banner.offsetHeight + 'px');
						layoutRoot.classList.add('wa-update-visible');
					};
					syncBannerLayout();
					if (window.ResizeObserver) {
						bannerResizeObserver = new ResizeObserver(syncBannerLayout);
						bannerResizeObserver.observe(banner);
					}
				}

				try {
					if ((!window.isNotificationsEnabled || window.isNotificationsEnabled()) && window.sendNativeNotification) {
						var notifTitle = 'Update Available';
						var notifBody = 'WhatsApp Desk v' + latestVersion + ' is available. Click to update the application.';
						window.sendNativeNotification(notifTitle, notifBody);
					}
				} catch (e) {}

				btnUpdate.onclick = function() {
					if (!downloadUrl && window.triggerCheckForUpdate) {
						window.triggerCheckForUpdate();
						return;
					}
					actionsDiv.style.display = 'none';
					progressWrap.style.display = 'flex';
					btnUpdate.textContent = 'Updating...';
					msg.style.color = '#d1d7db';
					msg.textContent = 'Downloading update package...';
					if (window.startUpdateNative) {
						window.startUpdateNative(downloadUrl);
					}
				};

				btnDismiss.onclick = function() {
					sessionStorage.setItem('dismissed_update_' + latestVersion, 'true');
					if (bannerResizeObserver) {
						bannerResizeObserver.disconnect();
						bannerResizeObserver = null;
					}
					if (banner.parentNode) {
						banner.parentNode.removeChild(banner);
					}
					document.documentElement.classList.remove('wa-update-visible');
					document.documentElement.style.removeProperty('--wa-update-banner-height');
				};
			};

			window.onUpdateProgress = function(pct) {
				var bar = document.getElementById('wa-update-progress-bar');
				var label = document.getElementById('wa-update-progress-pct');
				var msg = document.getElementById('wa-update-text');
				if (bar) bar.style.width = pct + '%';
				if (label) label.textContent = pct + '%';
				if (msg) msg.textContent = 'Downloading update package... ' + pct + '%';
			};

			window.onUpdateStatus = function(statusMsg) {
				var msg = document.getElementById('wa-update-text');
				if (msg) msg.textContent = statusMsg;
			};

			window.onUpdateError = function(errMsg) {
				var actions = document.getElementById('wa-update-actions');
				var prog = document.getElementById('wa-update-progress-wrap');
				var msg = document.getElementById('wa-update-text');
				var retry = document.getElementById('wa-btn-update');
				if (actions) actions.style.display = 'flex';
				if (prog) prog.style.display = 'none';
				if (retry) retry.textContent = 'Retry';
				if (msg) {
					msg.textContent = 'Update failed: ' + (errMsg || 'Unknown error');
					msg.style.color = '#ff8a80';
				}
				showFloatingToast('❌ Failed to update: ' + errMsg);
			};

			// Manual Check Function and Shortcut (Cmd/Ctrl + Shift + U)
			window.triggerCheckForUpdate = function() {
				showFloatingToast('🔍 Checking for updates...');
				if (window.checkForUpdateNative) {
					return window.checkForUpdateNative(true).then(function(res) {
						if (res && res.available) {
							window.showUpdateBanner(res.latest_version, res.release_title, res.download_url);
						} else if (res && res.check_error) {
							showFloatingToast('⚠️ Update check failed: ' + res.check_error);
						} else {
							var cur = (res && res.current_version) ? res.current_version : '__WA_APP_VERSION__';
							showFloatingToast('✅ WhatsApp Desk is up to date (v' + cur + ')');
						}
						return res;
					}).catch(function() {
						showFloatingToast('⚠️ Unable to check for updates at this time.');
					});
				}
				return Promise.resolve(null);
			};

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'u' || e.key === 'U')) {
					e.preventDefault();
					e.stopPropagation();
					window.triggerCheckForUpdate();
				}
			}, true);
		});

		// Dynamic Responsive Desktop Layout (enables seamless shrinking and expanding)
		waRunModule('responsive-css', function() {
			var respStyle = document.createElement('style');
			respStyle.id = 'whatsapp-desktop-responsive';
			respStyle.textContent = '' +
				'html, body, #app { width: 100% !important; height: 100% !important; min-width: 0 !important; overflow: hidden !important; -webkit-font-smoothing: antialiased; }' +
				'#app > div, #app .two { width: 100% !important; height: 100% !important; min-width: 0 !important; max-width: 100% !important; top: 0 !important; margin: 0 !important; border-radius: 0 !important; }' +
				'[data-testid="status-v3"] { min-width: 0 !important; width: 100% !important; height: 100% !important; }' +
				'@media screen and (min-width: 641px) {' +
				'  #pane-side, div[data-testid="chat-list"] { min-width: 200px !important; -webkit-overflow-scrolling: touch !important; }' +
				'  #main { min-width: 240px !important; -webkit-overflow-scrolling: touch !important; }' +
				'}' +
				'@media screen and (max-width: 640px) {' +
				'  #pane-side, div[data-testid="chat-list"], #main { min-width: 0 !important; }' +
				'}';

			// Once <head> exists the style never needs re-injection, so poll only
			// via a cheap head observer instead of an endless 2.5s interval.
			function injectResponsive() {
				var targetHead = document.head || (document.documentElement && document.documentElement.querySelector && document.documentElement.querySelector('head'));
				if (targetHead && !document.getElementById('whatsapp-desktop-responsive')) {
					targetHead.appendChild(respStyle);
					try { observer.disconnect(); } catch (e) {}
				}
			}
			var observer = new MutationObserver(injectResponsive);
			if (document.head) {
				injectResponsive();
			} else if (document.documentElement && document.documentElement.nodeType) {
				try { observer.observe(document.documentElement, { childList: true }); } catch (e) {}
			} else if (document && document.nodeType) {
				try { observer.observe(document, { childList: true, subtree: true }); } catch (e) {}
			}
			document.addEventListener('DOMContentLoaded', function() {
				injectResponsive();
				try { observer.disconnect(); } catch (e) {}
			}, { once: true });
		});

		// Native Spell Check for textareas (macOS NSSpellChecker, Windows ISpellCheckProvider, Linux GTK)
		waRunModule('spellcheck', function() {
			var spellCheckEnabled = true;
			var spellCheckLang = 'auto';

			function applySpellCheck(el) {
				if (!el || el.nodeType !== 1 || el.dataset.spellCheckInitialized) return;
				el.dataset.spellCheckInitialized = 'true';
				el.spellcheck = spellCheckEnabled;
				if (spellCheckLang !== 'auto') {
					el.lang = spellCheckLang;
				}
			}

			function enableSpellCheckOnTextareas(scope) {
				var selector = 'textarea[contenteditable="true"], div[contenteditable="true"][role="textbox"], textarea';
				var root = (scope && scope.querySelectorAll) ? scope : document;
				try {
					if (scope && scope.matches && scope.matches(selector)) applySpellCheck(scope);
					var textareas = root.querySelectorAll(selector);
					for (var i = 0; i < textareas.length; i++) applySpellCheck(textareas[i]);
				} catch (e) {}
			}

			function initSpellCheck() {
				// Initial enable
				enableSpellCheckOnTextareas();

				// Watch for new textareas (WhatsApp Web is SPA). The chat list is
				// virtualized, so scrolling continuously adds and removes rows. The old
				// version ran a document-wide querySelectorAll for every single added
				// node, which is what made long chat-list scrolls stutter. Coalesce to
				// one pass per animation frame and only for subtrees that can hold
				// editable text.
				var spellCheckScheduled = false;
				var pendingSpellRoots = [];
				function spellCheckRelevant(node) {
					if (!node || node.nodeType !== 1) return false;
					try {
						return !!(node.matches && node.matches('[contenteditable="true"], textarea')) ||
							!!(node.querySelector && node.querySelector('[contenteditable="true"], textarea'));
					} catch (e) {
						return false;
					}
				}
				function runSpellCheckScan() {
					spellCheckScheduled = false;
					var roots = pendingSpellRoots.splice(0, pendingSpellRoots.length);
					if (shouldPauseBackgroundWork()) return;
					for (var r = 0; r < roots.length; r++) enableSpellCheckOnTextareas(roots[r]);
				}
				function scheduleSpellCheck() {
					if (spellCheckScheduled || pendingSpellRoots.length === 0) return;
					spellCheckScheduled = true;
					requestAnimationFrame(runSpellCheckScan);
				}
				var observer = new MutationObserver(function(mutations) {
					if (shouldPauseBackgroundWork()) return;
					for (var i = 0; i < mutations.length && pendingSpellRoots.length < 12; i++) {
						var added = mutations[i].addedNodes;
						for (var j = 0; j < added.length && pendingSpellRoots.length < 12; j++) {
							if (spellCheckRelevant(added[j])) pendingSpellRoots.push(added[j]);
						}
					}
					scheduleSpellCheck();
				});
				var target = document.body || document.documentElement || document;
				if (target && target.nodeType) {
					try { observer.observe(target, { childList: true, subtree: true }); } catch (e) {}
				}

				// Also re-check on navigation
				var lastUrl = location.href;
				setInterval(function() {
					if (location.href !== lastUrl) {
						lastUrl = location.href;
						setTimeout(enableSpellCheckOnTextareas, 300);
					}
				}, 1000);
			}

			// Expose toggle for settings
			window.toggleSpellCheck = function(enabled) {
				spellCheckEnabled = !!enabled;
				enableSpellCheckOnTextareas();
				if (window.setSpellCheckEnabledNative) {
					window.setSpellCheckEnabledNative(spellCheckEnabled);
				}
			};

			window.setSpellCheckLanguage = function(lang) {
				spellCheckLang = lang;
				enableSpellCheckOnTextareas();
			};

			if (document.readyState === 'loading') {
				document.addEventListener('DOMContentLoaded', initSpellCheck);
			} else {
				initSpellCheck();
			}
		});

		// Context Menu: Search/Translate selected text
		waRunModule('context-menu', function() {
			var contextMenu = null;
			var lastSelection = '';
			var lastSelectionRect = null;

			function createContextMenu() {
				if (contextMenu) return;
				contextMenu = document.createElement('div');
				contextMenu.id = 'wa-context-menu';
				contextMenu.style.cssText = 'position:fixed;z-index:9999999;background:#202c33;border:1px solid #2a3942;border-radius:8px;padding:6px 0;box-shadow:0 8px 24px rgba(0,0,0,0.4);min-width:180px;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;font-size:13px;color:#e9edef;';
				contextMenu.innerHTML = '' +
					'<div class="wa-cm-item" data-action="search" style="padding:8px 16px;cursor:pointer;display:flex;align-items:center;gap:10px;">' +
					'  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:#00a884;"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>' +
					'  <span>Search on Google</span>' +
					'</div>' +
					'<div class="wa-cm-item" data-action="translate" style="padding:8px 16px;cursor:pointer;display:flex;align-items:center;gap:10px;">' +
					'  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:#00a884;"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8.9h.5a8.48 8.48 0 0 1 8 8v.5z"></path><line x1="12" y1="12" x2="12" y2="12"></line></svg>' +
					'  <span>Translate</span>' +
					'</div>' +
					'<hr style="margin:6px 8px;border:none;border-top:1px solid #2a3942;">' +
					'<div class="wa-cm-item" data-action="copy" style="padding:8px 16px;cursor:pointer;display:flex;align-items:center;gap:10px;">' +
					'  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:#8696a0;"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>' +
					'  <span>Copy</span>' +
					'</div>';
				var parent = document.body || document.documentElement;
				if (parent) parent.appendChild(contextMenu);

				contextMenu.querySelectorAll('.wa-cm-item').forEach(function(item) {
					item.addEventListener('mouseenter', function() {
						this.style.background = '#2a3942';
					});
					item.addEventListener('mouseleave', function() {
						this.style.background = 'transparent';
					});
					item.addEventListener('click', function() {
						var action = this.dataset.action;
						handleContextAction(action);
						hideContextMenu();
					});
				});

				document.addEventListener('click', hideContextMenu, true);
				document.addEventListener('scroll', hideContextMenu, true);
			}

			function showContextMenu(x, y, text) {
				createContextMenu();
				lastSelection = text;
				contextMenu.style.left = x + 'px';
				contextMenu.style.top = y + 'px';
				contextMenu.style.display = 'block';
			}

			function hideContextMenu() {
				if (contextMenu) {
					contextMenu.style.display = 'none';
				}
			}

			function handleContextAction(action) {
				if (!lastSelection) return;
				var encoded = encodeURIComponent(lastSelection);
				if (action === 'search') {
					window.openExternalLink && window.openExternalLink('https://www.google.com/search?q=' + encoded);
				} else if (action === 'translate') {
					window.openExternalLink && window.openExternalLink('https://translate.google.com/?sl=auto&tl=id&text=' + encoded + '&op=translate');
				} else if (action === 'copy') {
					navigator.clipboard.writeText(lastSelection).then(function() {
						if (window.showFloatingToast) window.showFloatingToast('📋 Copied to clipboard');
					});
				}
			}

			function getSelectedText() {
				var selection = window.getSelection();
				if (!selection || selection.rangeCount === 0) return '';
				var text = selection.toString().trim();
				return text.length > 0 && text.length < 500 ? text : '';
			}

			function onContextMenu(e) {
				var text = getSelectedText();
				if (text) {
					e.preventDefault();
					showContextMenu(e.clientX, e.clientY, text);
				}
			}

			document.addEventListener('contextmenu', onContextMenu, true);

			// Also show on long-press for touch devices
			var longPressTimer = null;
			document.addEventListener('touchstart', function(e) {
				var text = getSelectedText();
				if (text) {
					longPressTimer = setTimeout(function() {
						var touch = e.touches[0];
						showContextMenu(touch.clientX, touch.clientY, text);
					}, 500);
				}
			}, { passive: true });
			document.addEventListener('touchend', function() {
				if (longPressTimer) clearTimeout(longPressTimer);
			});
			document.addEventListener('touchmove', function() {
				if (longPressTimer) clearTimeout(longPressTimer);
			});
		});

		// Automatic Download & Document Preview Interceptor for Chat Files & Media
		waRunModule('document-viewer', function() {
			var activeDownloadKeys = Object.create(null);
			// Paths the saver has already reported in this session. The Go saver
			// returns the path of an existing byte-identical file instead of
			// writing a second copy, so seeing the same path a second time means
			// the content was already on disk and the toast should say so.
			var savedDownloadPaths = Object.create(null);

			function downloadRequestKey(href, filename) {
				return String(filename || '') + '\n' + String(href || '');
			}

			function releaseDownloadRequest(requestKey, immediately) {
				delete activeDownloadKeys[requestKey];
			}

			// Toast action: opens the downloads folder in Finder/Explorer.
			function openFolderAction() {
				if (!window.openDownloadDirNative) return null;
				return {
					label: 'Open folder',
					onClick: function() { window.openDownloadDirNative(); }
				};
			}

			function markDownloadComplete(requestKey, savedPath) {
				var completedRequest = { status: 'complete', savedPath: savedPath };
				activeDownloadKeys[requestKey] = completedRequest;
				// Tell the badge layer this filename is now on disk so the next
				// scan badges it without a redundant native stat.
				var savedBase = (savedPath || '').split(/[\\/]/).pop();
				if (savedBase && window.__waMarkSaved) window.__waMarkSaved(savedBase);
				// Retain only the tiny path entry, never the Blob or base64 payload.
				setTimeout(function() {
					if (activeDownloadKeys[requestKey] === completedRequest) {
						delete activeDownloadKeys[requestKey];
					}
				}, 300000);
			}

			function isDocumentFileName(name) {
				if (!name) return false;
				var ext = name.toLowerCase();
				return ext.endsWith('.pdf') || ext.endsWith('.doc') || ext.endsWith('.docx') ||
					   ext.endsWith('.xls') || ext.endsWith('.xlsx') || ext.endsWith('.ppt') ||
					   ext.endsWith('.pptx') || ext.endsWith('.txt') || ext.endsWith('.csv') ||
					   ext.endsWith('.rtf');
			}

			function captureDownload(href, filename, shouldAutoOpen) {
				filename = resolveDownloadFilename(filename, '');
				var isDoc = isDocumentFileName(filename);
				if (shouldAutoOpen === undefined) {
					shouldAutoOpen = isDoc;
				}
				var requestKey = downloadRequestKey(href, filename);
				var existingRequest = activeDownloadKeys[requestKey];
				if (existingRequest && existingRequest.status === 'downloading') {
					return;
				}
				activeDownloadKeys[requestKey] = { status: 'downloading' };

				showFloatingToast(isDoc ? ('📄 Opening preview: ' + filename + '...') : ('⏳ Downloading: ' + filename + '...'));
				fetch(href)
					.then(function(response) {
						if (!response) return null;
						filename = resolveDownloadFilename(filename, response.headers.get('Content-Disposition'));
						return response.blob();
					})
					.then(function(blob) {
						if (!blob) return;
						var bType = (blob.type || '').toLowerCase();
						if (bType.indexOf('image/') === 0) {
							var imgExt = bType.indexOf('png') >= 0 ? '.png' :
							             bType.indexOf('webp') >= 0 ? '.webp' :
							             bType.indexOf('gif') >= 0 ? '.gif' : '.jpg';
							if (filename.toLowerCase().endsWith('.pdf') || !/\.(jpe?g|png|webp|gif)$/i.test(filename)) {
								filename = filename.replace(/\.[^.]+$/, '') + imgExt;
								if (!filename.endsWith(imgExt)) filename += imgExt;
							}
							shouldAutoOpen = false;
						}
						var isPdf = filename.toLowerCase().endsWith('.pdf');
						var previewBlob = isPdf ? blob.slice(0, blob.size, 'application/pdf') : blob;
						var ownedBlobUrl = isPdf ? origCreateObjectURL(previewBlob) : '';
						var reader = new FileReader();
						reader.onloadend = function() {
							var base64data = reader.result;
							if (window.saveDownloadedFileNative) {
								window.saveDownloadedFileNative(filename, base64data).then(function(savedPath) {
									if (savedPath) {
										var alreadySaved = savedDownloadPaths[savedPath] === true;
										savedDownloadPaths[savedPath] = true;
										markDownloadComplete(requestKey, savedPath);
										if (shouldAutoOpen) {
											showInAppDocModal(filename, ownedBlobUrl || href, savedPath, base64data, ownedBlobUrl);
											if (window.dismissStuckViewer) window.dismissStuckViewer();
											showFloatingToast(alreadySaved ? ('📄 Already saved: ' + filename) : ('📄 Preview opened: ' + filename), openFolderAction());
										} else {
											showFloatingToast(alreadySaved ? ('💾 File already saved: ' + filename) : ('💾 Saved successfully: ' + filename), openFolderAction());
										}
									} else {
										if (ownedBlobUrl) URL.revokeObjectURL(ownedBlobUrl);
										showFloatingToast('❌ Failed to save file.');
										releaseDownloadRequest(requestKey);
									}
								}).catch(function() {
									if (ownedBlobUrl) URL.revokeObjectURL(ownedBlobUrl);
									showFloatingToast('❌ Error saving file.');
									releaseDownloadRequest(requestKey);
								});
							} else {
								if (ownedBlobUrl) URL.revokeObjectURL(ownedBlobUrl);
								releaseDownloadRequest(requestKey);
							}
						};
						reader.onerror = function() {
							if (ownedBlobUrl) URL.revokeObjectURL(ownedBlobUrl);
							releaseDownloadRequest(requestKey);
						};
						reader.readAsDataURL(blob);
					})
					.catch(function(err) {
						console.error('Download intercept fetch error:', err);
						releaseDownloadRequest(requestKey);
					});
			}

			var forwardingDocumentDownload = false;
			var pendingViewerDownloadClick = false;
			var viewerDownloadSelector = [
				'button[data-testid*="download"]',
				'[role="button"][data-testid*="download"]',
				'button[aria-label*="Download" i]',
				'button[aria-label*="Unduh" i]',
				'[role="button"][aria-label*="Download" i]',
				'[role="button"][aria-label*="Unduh" i]',
				'button[title*="Download" i]',
				'button[title*="Unduh" i]',
				'[data-icon="download"]',
				'[data-icon="download-refreshed"]',
				'[data-icon*="download"]'
			].join(',');
			function isExplicitDownloadMenuItem(target) {
				if (!target || !target.closest) return false;
				var item = target.closest('[role="menuitem"]');
				if (!item) return false;
				var label = (item.getAttribute('aria-label') || item.getAttribute('title') || item.innerText || '')
					.replace(/\s+/g, ' ').trim();
				return /^(download|unduh)$/i.test(label);
			}
			document.addEventListener('click', function(e) {
				var target = e.target;
				if (target && target.closest &&
					((target.closest('[data-testid="media-viewer"], [role="toolbar"], header') && target.closest(viewerDownloadSelector)) || isExplicitDownloadMenuItem(target))) {
					lastExplicitDownloadAt = Date.now();
				}
			}, true);

			function findVisibleViewerDownloadControl() {
				var candidates = document.querySelectorAll(viewerDownloadSelector);
				var best = null;
				var bestScore = -1;
				for (var i = 0; i < candidates.length; i++) {
					var raw = candidates[i];
					if (raw.closest && raw.closest('#wa-doc-modal-overlay')) continue;
					var control = (raw.closest && raw.closest('button, a, [role="button"]')) || raw;
					var rect = control.getBoundingClientRect();
					if (rect.width < 8 || rect.height < 8 || rect.bottom <= 0 || rect.right <= 0 ||
						rect.top >= window.innerHeight || rect.left >= window.innerWidth) continue;
					var style = window.getComputedStyle(control);
					if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) continue;

					var score = 0;
					if (rect.top < window.innerHeight * 0.3) score += 4;
					if (rect.left > window.innerWidth * 0.55) score += 3;
					if (control.closest && control.closest('[role="dialog"], [data-testid*="viewer"], header, [role="toolbar"]')) score += 5;
					if (score > bestScore) {
						best = control;
						bestScore = score;
					}
				}
				return bestScore >= 4 ? best : null;
			}

			function triggerVisibleViewerDownload() {
				if (pendingViewerDownloadClick || !isRecentPDFIntent()) return false;
				var control = findVisibleViewerDownloadControl();
				if (!control) return false;
				pendingViewerDownloadClick = true;
				control.click();
				setTimeout(function() { pendingViewerDownloadClick = false; }, 1500);
				return true;
			}

			function findDocumentDownloadControl(start) {
				var selector = 'a[download], button[data-testid*="download"], [role="button"][data-testid*="download"], button[aria-label*="Unduh"], button[aria-label*="Download"], [role="button"][aria-label*="Unduh"], [role="button"][aria-label*="Download"], [data-icon="download"], [data-icon="download-refreshed"]';
				var node = start;
				for (var depth = 0; node && node !== document.body && depth < 12; depth++, node = node.parentElement) {
					var found = node.querySelector && node.querySelector(selector);
					if (found) return found.closest('button, a, [role="button"]') || found;
				}
				return null;
			}

			// Hook 1: Override HTMLAnchorElement.prototype.click (programmatic downloads)
			var originalAnchorClick = HTMLAnchorElement.prototype.click;
			HTMLAnchorElement.prototype.click = function() {
				var downloadAttr = this.getAttribute('download');
				var href = this.href || this.getAttribute('href');
				if ((downloadAttr !== null || this.download) && href && (href.indexOf('blob:') === 0 || href.indexOf('data:') === 0)) {
					var rawName = downloadAttr || this.download || '';
					var name = resolveDownloadFilename(rawName || (isRecentPDFIntent() ? lastClickedDocName : '') || 'whatsapp_media', '');
					var isDoc = isDocumentFileName(name);
					var isExplicit = isRecentExplicitDownload() && !forwardingDocumentDownload && !pendingViewerDownloadClick;
					var shouldOpen = isDoc && (forwardingDocumentDownload || isRecentPDFIntent() || !isExplicit);
					if (shouldOpen) {
						captureDownload(href, name, true);
					} else {
						lastExplicitDownloadAt = Date.now();
						captureDownload(href, name, false);
					}
					return;
				}
				return originalAnchorClick.apply(this, arguments);
			};

			// Hook 2: User click event capturing (direct clicks on <a> with download)
			document.addEventListener('click', function(e) {
				var target = e.target;
				while (target && target !== document.body) {
					if (target.tagName === 'A') {
						var downloadAttr = target.getAttribute('download');
						var href = target.href || target.getAttribute('href');
						if ((downloadAttr !== null || target.download) && href && (href.indexOf('blob:') === 0 || href.indexOf('data:') === 0)) {
							e.preventDefault();
							e.stopPropagation();
							var rawName = downloadAttr || target.download || '';
							var name = resolveDownloadFilename(rawName || (isRecentPDFIntent() ? lastClickedDocName : '') || 'whatsapp_media', '');
							var isDoc = isDocumentFileName(name);
							var isExplicit = isRecentExplicitDownload() && !forwardingDocumentDownload && !pendingViewerDownloadClick;
							var shouldOpen = isDoc && (forwardingDocumentDownload || isRecentPDFIntent() || !isExplicit);
							if (shouldOpen) {
								captureDownload(href, name, true);
							} else {
								lastExplicitDownloadAt = Date.now();
								captureDownload(href, name, false);
							}
							return;
						}
					}
					target = target.parentElement;
				}
			}, true);

			// Hook 3: Watch document bubble clicks in chat to handle viewer spinner
			document.addEventListener('click', function(e) {
				if (forwardingDocumentDownload) return;
				var el = e.target;
				if (!el) return;

				// Completely ignore clicks inside media-viewer or custom modal overlay
				if (typeof el.closest === 'function') {
					if (el.closest('[data-testid="media-viewer"]') || el.closest('#wa-doc-modal-overlay')) {
						return;
					}
				}

				var foundName = extractDocumentName(el);
				var clickedDoc = !!foundName;

				if (clickedDoc) {
					lastClickedDocName = foundName;
					lastDocumentIntentAt = Date.now();
					if (isDocumentFileName(foundName)) {
						var directDownload = findDocumentDownloadControl(el);
						if (directDownload && !directDownload.contains(el)) {
							e.preventDefault();
							e.stopImmediatePropagation();
							forwardingDocumentDownload = true;
							directDownload.click();
							forwardingDocumentDownload = false;
							return;
						}
					}

					var checkCount = 0;
					var checkTimer = setInterval(function() {
						if (shouldPauseBackgroundWork()) {
							clearInterval(checkTimer);
							return;
						}
						checkCount++;
						if (checkCount > 30) {
							clearInterval(checkTimer);
							return;
						}

						if (triggerVisibleViewerDownload()) {
							clearInterval(checkTimer);
						}
					}, 200);
				}
			}, true);

			// Hook 4: MutationObserver to auto-dismiss stuck media viewer and trigger download/preview.
			// This observes the whole document body (subtree), which also churns heavily while
			// the chat list is scrolled, so coalesce to at most one check per animation frame
			// instead of running on every individual mutation batch.
			var viewerCheckScheduled = false;
			var viewerObserver = new MutationObserver(function() {
				if (shouldPauseBackgroundWork() || !isRecentPDFIntent() || viewerCheckScheduled) return;
				viewerCheckScheduled = true;
				requestAnimationFrame(function() {
					viewerCheckScheduled = false;
					if (!isRecentPDFIntent()) return;
					if (!document.getElementById('wa-doc-modal-overlay')) triggerVisibleViewerDownload();
				});
			});

			function initViewerObserver() {
				var target = document.body || document.documentElement || document;
				if (target && target.nodeType) {
					try { viewerObserver.observe(target, { childList: true, subtree: true }); } catch (e) {}
				} else {
					document.addEventListener('DOMContentLoaded', initViewerObserver, { once: true });
				}
			}
			initViewerObserver();
		});

		// "Saved to disk" badges on the Media/Docs panel. WhatsApp has no notion
		// of local downloads, so bridge it: for each document/media item shown in
		// the all-chats panel, check whether the same filename exists in the
		// configured downloads folder and tag it with a small green check.
		waRunModule('saved-badges', function() {
			var savedScanQueued = false;
			var lastSavedScanAt = 0;
			var savedCache = {};
			var savedPending = {};
			var badgeStyle = 'display:inline-flex;align-items:center;gap:2px;margin-left:6px;padding:0 6px;border-radius:8px;' +
				'font-size:10px;font-weight:600;line-height:14px;vertical-align:middle;background:rgba(6,174,116,.16);color:#06ae74;';

			function fileExistsOnDisk(name) {
				if (!name || !window.checkFileExistsNative) return Promise.resolve(false);
				if (name in savedCache) return Promise.resolve(savedCache[name]);
				// Coalesce concurrent lookups for the same name: repeated scans
				// while a check is in flight must not spam the native binding.
				if (savedPending[name]) return savedPending[name];
				var p = window.checkFileExistsNative(name).then(function(exists) {
					delete savedPending[name];
					savedCache[name] = !!exists;
					return !!exists;
				}).catch(function() {
					delete savedPending[name];
					return false;
				});
				savedPending[name] = p;
				return p;
			}

			// Called by the download path so a just-saved file badges instantly.
			window.__waMarkSaved = function(name) {
				if (name) savedCache[name] = true;
			};

			function findFileNameElement(el) {
				if (!el) return null;
				if (el.getAttribute && el.getAttribute('title')) return el;
				return el.querySelector && el.querySelector('span[title], div[title]');
			}

			function decorateItem(el, name, filenameEl) {
				if (el.__waSavedBadge) return;
				fileExistsOnDisk(name).then(function(exists) {
					if (!exists) return;
					el.__waSavedBadge = true;
					var badge = document.createElement('span');
					badge.className = 'wa-saved-badge';
					badge.setAttribute('aria-label', 'Already saved to downloads folder');
					badge.style.cssText = badgeStyle;
					badge.textContent = '✓ Saved';
					// Attach the badge to the filename element so it follows the
					// incoming/outgoing bubble instead of landing at the row's left edge.
					var host = filenameEl || el.querySelector('[data-testid="cell-frame-container"], .copyable-text') || el;
					host.style.position = host.style.position || 'relative';
					host.appendChild(badge);
				});
			}

			function itemFileName(el) {
				var titleEl = findFileNameElement(el);
				var t = titleEl ? (titleEl.getAttribute('title') || '') : '';
				if (!t) return '';
				var m = t.match(/([^\n\r<>]{1,180}\.(pdf|docx?|xlsx?|pptx?|txt|csv|rtf|zip|mp4|mkv|mov|mp3|wav|jpe?g|png|webp|heic))\b/i);
				return m ? m[1].trim() : '';
			}

			function scanPanel() {
				// Only scan where items can actually be seen: the open media/docs
				// panel (dialog/viewer) or the current chat pane. Scanning the
				// whole document on every chat-list mutation is exactly the
				// background churn this app is supposed to avoid.
				var scope = document.querySelector('[role="dialog"], [data-testid="media-viewer"]') ||
					document.getElementById('main');
				if (!scope) return;
				var rows = scope.querySelectorAll('[role="row"], [data-testid="cell-frame-outer"], .message-in, .message-out');
				for (var i = 0; i < rows.length; i++) {
					var row = rows[i];
					if (row.__waSavedBadge) continue;
					var name = itemFileName(row);
					if (name) decorateItem(row, name, findFileNameElement(row));
				}
			}

			function scheduleScan() {
				if (savedScanQueued || shouldPauseBackgroundWork()) return;
				// Hard throttle: the panel observer fires on every DOM mutation
				// while WhatsApp virtualizes lists; 2s between scans is plenty
				// for a "saved" badge that is purely informational.
				var now = Date.now();
				if (now - lastSavedScanAt < 2000) return;
				savedScanQueued = true;
				requestAnimationFrame(function() {
					savedScanQueued = false;
					lastSavedScanAt = Date.now();
					scanPanel();
				});
			}

			// Observe only where badges can appear (open dialog/viewer or the
			// chat pane). Chat-list churn in #pane-side never needs a rescan,
			// so ignore mutations outside the relevant scope entirely.
			function panelMutationRelevant(muts) {
				for (var i = 0; i < muts.length; i++) {
					var t = muts[i].target;
					if (t && t.closest) {
						try {
							if (t.closest('#main, [role="dialog"], [data-testid="media-viewer"], #wa-doc-modal-overlay')) return true;
						} catch (e) {}
					}
				}
				return false;
			}
			var panelObserver = new MutationObserver(function(muts) {
				if (panelMutationRelevant(muts)) scheduleScan();
			});
			function watchRoot() {
				var root = document.body || document.documentElement || document;
				if (root && root.nodeType) {
					try { panelObserver.observe(root, { childList: true, subtree: true }); } catch (e) {}
				}
			}
			watchRoot();
			document.addEventListener('DOMContentLoaded', watchRoot, { once: true });
			document.addEventListener('click', function(e) {
				// Rescan when the user opens the media/docs panel from the toolbar.
				if (e.target && e.target.closest && e.target.closest('[data-testid="chat-menu"], [data-icon="default-image"], [data-icon="docs"], [data-icon="image"]')) {
					setTimeout(scheduleScan, 300);
				}
			}, true);
		});

		// Theme Manager, In-Flow Header Toolbar Button & Control Center Modal
		waRunModule('settings-modal', function() {
			var isMac = (__WA_GOOS === 'darwin') || (navigator.platform && navigator.platform.toUpperCase().indexOf('MAC') >= 0);
			var currentTheme = 'dark';
			var themeChoiceVersion = 0;
			var themeLoadStarted = false;
			var themeReloadTimer = null;
			var themeReapplyTimers = [];
			var themeStyle = null;
			var mediaPermissionCard = null;
			// Keep the engine's native MediaQueryList intact. Replacing matchMedia with
			// a partial object breaks framework listeners on some WebView2/WebKitGTK
			// versions and was the main cross-platform difference in theme switching.
			var origMatchMedia = window.matchMedia ? window.matchMedia.bind(window) : null;

			// --- Theme Management ---
			function getSystemIsDark() {
				if (origMatchMedia) {
					return origMatchMedia('(prefers-color-scheme: dark)').matches;
				}
				return true;
			}

			function applyThemeClasses(isDark) {
				var mode = isDark ? 'dark' : 'light';
				var opposite = isDark ? 'light' : 'dark';
				var root = document.documentElement;
				if (!root) return;
				root.classList.add(mode);
				root.classList.remove(opposite);
				root.setAttribute('data-theme', mode);
				root.setAttribute('data-wa-desk-theme', mode);
				root.style.colorScheme = mode;
				if (document.body) {
					document.body.classList.add(mode);
					document.body.classList.remove(opposite);
					document.body.setAttribute('data-theme', mode);
					document.body.setAttribute('data-wa-desk-theme', mode);
					document.body.style.colorScheme = mode;
				}
			}

			function ensureThemeStyle() {
				if (!themeStyle) themeStyle = document.getElementById('wa-desk-theme-style');
				if (!themeStyle) {
					themeStyle = document.createElement('style');
					themeStyle.id = 'wa-desk-theme-style';
					themeStyle.textContent = [
						'html[data-wa-desk-theme="light"], html[data-wa-desk-theme="light"] body { color-scheme: light !important; background: #f7f9fa !important; }',
						'html[data-wa-desk-theme="light"] #app, html[data-wa-desk-theme="light"] #side, html[data-wa-desk-theme="light"] #pane-side, html[data-wa-desk-theme="light"] #main { color-scheme: light !important; }'
					].join('\n');
					var target = document.head || document.documentElement;
					if (target) {
						target.appendChild(themeStyle);
					}
				}
			}

			function persistThemePreference(theme, isDark) {
				storageSet('system-theme-mode', theme === 'system' ? 'true' : 'false');
				storageSet('theme', JSON.stringify(theme === 'system' ? (isDark ? 'dark' : 'light') : theme));
				storageSet('wa-desk-theme', theme);
			}

			function scheduleThemeReapply() {
				while (themeReapplyTimers.length) clearTimeout(themeReapplyTimers.pop());
				[0, 350, 1200, 2600].forEach(function(delay) {
					themeReapplyTimers.push(setTimeout(function() {
						var isDark = currentTheme === 'system' ? getSystemIsDark() : currentTheme === 'dark';
						applyThemeClasses(isDark);
						persistThemePreference(currentTheme, isDark);
						if (window.syncToolbarBtnTheme) window.syncToolbarBtnTheme(isDark);
						if (window.syncRailSettingsBtnTheme) window.syncRailSettingsBtnTheme(isDark);
					}, delay));
				});
			}

			function applyThemeToDOM(theme) {
				currentTheme = theme;
				var isDark = (theme === 'system') ? getSystemIsDark() : (theme === 'dark');

				// 1. Update the document immediately for our controls and current page.
				applyThemeClasses(isDark);
				ensureThemeStyle();

				// 2. Synchronize WhatsApp Web's own localStorage keys before its tree settles.
				persistThemePreference(theme, isDark);

				// 3. Update modal and toolbar button if visible
				if (window.syncModalTheme) {
					window.syncModalTheme(isDark);
				}
				if (window.syncToolbarBtnTheme) {
					window.syncToolbarBtnTheme(isDark);
				}
				if (window.syncRailSettingsBtnTheme) window.syncRailSettingsBtnTheme(isDark);

				// WhatsApp may finish mounting after our script. Reapply a small, bounded
				// number of times instead of observing body classes forever: that old
				// observer could enter a feedback loop and raise CPU on Windows/macOS.
				scheduleThemeReapply();
			}

			window.getAppTheme = function() {
				return currentTheme;
			};

			window.setAppTheme = function(theme) {
				if (theme !== 'dark' && theme !== 'light' && theme !== 'system') {
					theme = 'dark';
				}
				themeChoiceVersion++;
				applyThemeToDOM(theme);
				if (window.setAppThemeNative) {
					Promise.resolve(window.setAppThemeNative(theme)).catch(function() {});
				}
				showFloatingToast(theme === 'dark' ? 'Theme: Dark' : (theme === 'light' ? 'Theme: Light' : 'Theme: System'));
				// WhatsApp keeps theme state inside its running application tree. Reload
				// once after persisting the choice so every engine starts from the same
				// localStorage state instead of leaving part of the UI in the old theme.
				clearTimeout(themeReloadTimer);
				themeReloadTimer = setTimeout(function() {
					window.location.reload();
				}, 300);
			};

			// Listen for system appearance changes
			if (origMatchMedia) {
				var sysMedia = origMatchMedia.call(window, '(prefers-color-scheme: dark)');
				var onSysChange = function() {
					if (currentTheme === 'system') {
						applyThemeToDOM('system');
						if (window.setAppThemeNative) Promise.resolve(window.setAppThemeNative('system')).catch(function() {});
					}
				};
				if (sysMedia.addEventListener) {
					sysMedia.addEventListener('change', onSysChange);
				} else if (sysMedia.addListener) {
					sysMedia.addListener(onSysChange);
				}
			}

			// Load saved theme from native settings and keep synced
			function initTheme() {
				if (themeLoadStarted || !window.getAppThemeNative) return;
				themeLoadStarted = true;
				var requestVersion = themeChoiceVersion;
				window.getAppThemeNative().then(function(savedTheme) {
					if (requestVersion !== themeChoiceVersion) return;
					if (savedTheme) applyThemeToDOM(savedTheme);
				}).catch(function() {
					themeLoadStarted = false;
				});
			}
			initTheme();
			document.addEventListener('DOMContentLoaded', function() {
				initTheme();
				applyThemeToDOM(currentTheme);
			}, { once: true });

			// --- In-Flow Header Toolbar Button (Non-Floating, Clean WhatsApp Style) ---
			function injectHeaderToolbarBtn() {
				if (document.getElementById('wa-toolbar-settings-btn')) return;

				// Target WhatsApp Web's left header above chats
				var header = document.querySelector('#side header') || document.querySelector('header');
				if (!header) return;

				// Header descendants change frequently. Use its direct trailing child, not
				// querySelector('div:last-child'), which can select an invisible nested node.
				var actionsWrap = header.lastElementChild || header;
				if (!actionsWrap) return;

				var btn = document.createElement('button');
				btn.id = 'wa-toolbar-settings-btn';
				btn.setAttribute('aria-label', 'Settings & Controls');
				btn.title = 'Settings & Controls (' + (isMac ? 'Cmd' : 'Ctrl') + ' + ,)';
				btn.style.cssText = 'width:40px;height:40px;border-radius:50%;display:inline-flex;align-items:center;justify-content:center;background:transparent;border:none;cursor:pointer;outline:none;transition:background-color 0.15s ease, color 0.15s ease;flex-shrink:0;margin:0 2px;';
				btn.innerHTML = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">' +
					'<circle cx="12" cy="12" r="3"></circle>' +
					'<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>' +
					'</svg>';

				// The header lives outside WhatsApp Web's own dark/light class toggling on
				// <body>, so this button previously always kept the dark-theme icon color
				// even when the app was switched to Light. Keep its resting color in sync
				// with the current app theme instead of a hardcoded dark-mode gray.
				function restingIconColor() {
					var isDarkNow = currentTheme === 'system' ?
						(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) :
						(currentTheme === 'dark');
					return isDarkNow ? '#aebac1' : '#54656f';
				}
				btn.style.color = restingIconColor();
				window.syncToolbarBtnTheme = function() {
					btn.style.color = restingIconColor();
				};

				btn.onmouseenter = function() {
					btn.style.backgroundColor = document.body.classList.contains('dark') ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)';
					btn.style.color = document.body.classList.contains('dark') ? '#e9edef' : '#111b21';
				};
				btn.onmouseleave = function() {
					btn.style.backgroundColor = 'transparent';
					btn.style.color = restingIconColor();
				};
				btn.onclick = function(e) {
					e.stopPropagation();
					window.showSettingsModal();
				};

				actionsWrap.appendChild(btn);
			}

			function isElementVisible(el) {
				if (!el || !el.isConnected) return false;
				var rect = el.getBoundingClientRect();
				if (rect.width <= 0 || rect.height <= 0) return false;
				var style = window.getComputedStyle(el);
				if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return false;
				var docEl = document.documentElement;
				var vh = window.innerHeight || (docEl && docEl.clientHeight) || 800;
				var vw = window.innerWidth || (docEl && docEl.clientWidth) || 1200;
				if (rect.bottom <= 0 || rect.right <= 0 || rect.top >= vh || rect.left >= vw) return false;
				var p = el.parentElement;
				while (p && p !== document.body && p !== document.documentElement) {
					var ps = window.getComputedStyle(p);
					if (ps && (ps.overflow === 'hidden' || ps.overflowX === 'hidden' || ps.overflowY === 'hidden')) {
						var pr = p.getBoundingClientRect();
						if (rect.bottom <= pr.top || rect.top >= pr.bottom || rect.right <= pr.left || rect.left >= pr.right) {
							return false;
						}
					}
					p = p.parentElement;
				}
				return true;
			}

			// Keep one compact Settings control in the left rail. Header content is
			// routinely rebuilt by WhatsApp, so the rail control exists as a
			// safety net — but it must stay hidden while the header button is
			// present and visible, otherwise the user sees two identical gears.
			function ensureSettingsFallback() {
				var headerButton = document.getElementById('wa-toolbar-settings-btn');
				var headerVisible = isElementVisible(headerButton);
				var fallback = document.getElementById('wa-settings-fallback-btn');
				if (fallback) {
					fallback.setAttribute('data-header-settings-visible', headerVisible ? 'true' : 'false');
					// Hide the rail control whenever the header button works.
					fallback.style.display = headerVisible ? 'none' : 'inline-flex';
					if (window.__waRecheckEmergencySettings) window.__waRecheckEmergencySettings();
					return;
				}
				if (!document.body) return;
				fallback = document.createElement('button');
				fallback.id = 'wa-settings-fallback-btn';
				fallback.type = 'button';
				fallback.setAttribute('aria-label', 'Open Settings and Controls');
				fallback.title = 'Settings & Controls (' + (isMac ? 'Cmd' : 'Ctrl') + ' + ,)';
				fallback.innerHTML = '<svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>';
				fallback.style.cssText = 'position:fixed;left:14px;bottom:14px;z-index:9999998;width:38px;height:38px;padding:0;display:inline-flex;align-items:center;justify-content:center;border:1px solid rgba(134,150,160,.45);border-radius:50%;background:#111b21;color:#aebac1;cursor:pointer;box-shadow:0 4px 12px rgba(0,0,0,.3);';
				fallback.setAttribute('data-header-settings-visible', headerVisible ? 'true' : 'false');
				// Only visible when the header button is missing or unusable.
				fallback.style.display = headerVisible ? 'none' : 'inline-flex';
				if (window.__waRecheckEmergencySettings) window.__waRecheckEmergencySettings();
				window.syncRailSettingsBtnTheme = function(isDark) {
					fallback.style.background = isDark ? '#111b21' : '#ffffff';
					fallback.style.color = isDark ? '#aebac1' : '#54656f';
					fallback.style.borderColor = isDark ? 'rgba(134,150,160,.45)' : 'rgba(84,101,111,.28)';
				};
				window.syncRailSettingsBtnTheme(currentTheme === 'system' ? getSystemIsDark() : currentTheme === 'dark');
				fallback.onclick = function(e) {
					e.preventDefault();
					e.stopPropagation();
					window.showSettingsModal();
				};
				document.body.appendChild(fallback);
			}

			function ensureSettingsEntryPoints() {
				injectHeaderToolbarBtn();
				// A button injected in this same tick has not been laid out yet:
				// getBoundingClientRect() is still 0x0, so isElementVisible would
				// report "hidden" and wrongly reveal the rail fallback. Re-check
				// on the next frame, when the header button has real geometry.
				requestAnimationFrame(ensureSettingsFallback);
			}

			ensureSettingsEntryPoints();
			document.addEventListener('DOMContentLoaded', function() {
				ensureSettingsEntryPoints();
				setTimeout(ensureSettingsEntryPoints, 600);
			});
			window.addEventListener('load', ensureSettingsEntryPoints);
			// WhatsApp rebuilds its header when switching chats, dropping our
			// button. Watch only #side/header region changes (rAF-coalesced)
			// instead of scanning the whole page every 2 seconds forever.
			var toolbarCheckQueued = false;
			var toolbarNarrowed = false;
			var toolbarObserver = new MutationObserver(function() {
				if (toolbarCheckQueued) return;
				toolbarCheckQueued = true;
					requestAnimationFrame(function() {
						toolbarCheckQueued = false;
						// Re-evaluate on any header change: the toolbar button may
						// have been dropped (needs re-injection) or may have become
						// hidden/clipped (rail fallback must take over). Checking the
						// visibility too is what keeps exactly one gear on screen.
						var headerButton = document.getElementById('wa-toolbar-settings-btn');
						if (!headerButton || !isElementVisible(headerButton)) {
							ensureSettingsEntryPoints();
						}
						// Narrow the observed root once the header exists.
						if (!toolbarNarrowed) {
							var hdr = document.querySelector('#side header');
							if (hdr && hdr.nodeType) {
								toolbarNarrowed = true;
								try {
									toolbarObserver.disconnect();
									toolbarObserver.observe(hdr, { childList: true, subtree: true });
								} catch (e) {}
							}
						}
					});
			});
			function watchToolbarRoot() {
				// Prefer the header itself: the chat list churns constantly and
				// never affects our button. Fall back to #side, then body, and
				// narrow down to the header as soon as it exists.
				var root = document.querySelector('#side header') || document.querySelector('#side') || document.body || document.documentElement || document;
				if (root && root.nodeType) {
					toolbarNarrowed = !!document.querySelector('#side header');
					try {
						toolbarObserver.disconnect();
						toolbarObserver.observe(root, { childList: true, subtree: true });
					} catch (e) {}
				}
			}
			watchToolbarRoot();
			document.addEventListener('DOMContentLoaded', watchToolbarRoot, { once: true });

			// --- Minimalist WhatsApp Control Center Modal ---
			window.showSettingsModal = function() {
				if (document.getElementById('wa-settings-overlay')) {
					var ex = document.getElementById('wa-settings-overlay');
					if (ex.parentNode) ex.parentNode.removeChild(ex);
					return;
				}

				var isDark = currentTheme === 'system' ?
					(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) :
					(currentTheme === 'dark');

				var overlay = document.createElement('div');
				overlay.id = 'wa-settings-overlay';
				overlay.style.cssText = 'position:fixed;inset:0;background:rgba(8,15,19,.68);z-index:9999999;display:flex;align-items:center;justify-content:center;padding:16px;box-sizing:border-box;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;';

				var modal = document.createElement('div');
				modal.id = 'wa-settings-container';
				modal.style.cssText = 'width:520px;max-width:96vw;max-height:90vh;border-radius:10px;box-sizing:border-box;display:flex;flex-direction:column;gap:0;overflow-y:auto;padding:0 22px 18px;box-shadow:0 18px 48px rgba(0,0,0,.32);';

				// Header
				var header = document.createElement('div');
				header.id = 'wa-modal-header';
				header.style.cssText = 'display:flex;align-items:center;justify-content:space-between;border-bottom-width:1px;border-bottom-style:solid;padding:18px 0 14px;margin-bottom:2px;';
				header.innerHTML = '' +
					'<div style="display:flex;align-items:center;gap:10px;">' +
					'  <div id="wa-modal-icon-wrap" style="width:10px;height:10px;border-radius:50%;display:flex;align-items:center;justify-content:center;background:#00a884;">' +
					'  </div>' +
					'  <div>' +
					'    <h3 id="wa-modal-title" style="margin:0;font-size:15px;font-weight:600;">WhatsApp Desk</h3>' +
					'    <span id="wa-modal-sub" style="font-size:11px;">Application settings · version __WA_APP_VERSION__</span>' +
					'  </div>' +
					'</div>' +
					'<button id="wa-settings-close-x" style="background:transparent;border:none;cursor:pointer;font-size:18px;line-height:1;padding:4px 8px;border-radius:4px;">✕</button>';
				modal.appendChild(header);

				// Section 0: Theme Switcher Segmented Control
				var themeBox = document.createElement('div');
				themeBox.className = 'wa-modal-card';
				themeBox.style.cssText = 'display:flex;align-items:center;justify-content:space-between;padding:14px 0;border-radius:0;border-width:0 0 1px;border-style:solid;gap:16px;';
				themeBox.innerHTML = '' +
					'<div>' +
					'  <strong class="wa-text-primary" style="font-size:12.5px;display:block;">Appearance</strong>' +
					'  <span class="wa-text-muted" style="font-size:11px;">Application interface theme</span>' +
					'</div>' +
					'<div style="display:flex;align-items:center;gap:4px;">' +
					'  <button id="wa-theme-btn-dark" class="wa-theme-btn" style="padding:5px 10px;border-radius:6px;font-size:11.5px;cursor:pointer;border-width:1px;border-style:solid;font-weight:500;">Dark</button>' +
					'  <button id="wa-theme-btn-light" class="wa-theme-btn" style="padding:5px 10px;border-radius:6px;font-size:11.5px;cursor:pointer;border-width:1px;border-style:solid;font-weight:500;">Light</button>' +
					'  <button id="wa-theme-btn-system" class="wa-theme-btn" style="padding:5px 10px;border-radius:6px;font-size:11.5px;cursor:pointer;border-width:1px;border-style:solid;font-weight:500;">System</button>' +
					'</div>';
				modal.appendChild(themeBox);

				// Section 1: Quick Interactive Controls (2-Column Grid)
				var quickGrid = document.createElement('div');
				quickGrid.style.cssText = 'display:flex;flex-direction:column;';

				if (isMac && window.getCameraPermissionNative && window.getMicrophonePermissionNative) {
					mediaPermissionCard = document.createElement('div');
					mediaPermissionCard.className = 'wa-modal-card';
					mediaPermissionCard.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;flex-direction:column;gap:8px;';
					mediaPermissionCard.innerHTML = '' +
						'<div style="display:flex;align-items:center;justify-content:space-between;gap:16px;">' +
						'  <div>' +
						'    <strong class="wa-text-primary" style="font-size:12.5px;display:block;">Camera & Microphone</strong>' +
						'    <span id="wa-media-permission-summary" class="wa-text-muted" style="font-size:11px;display:block;margin-top:2px;">Checking macOS permissions...</span>' +
						'  </div>' +
						'  <div style="display:flex;gap:6px;flex-shrink:0;">' +
						'    <button id="wa-media-permission-settings" class="wa-card-btn" style="padding:4px 8px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">System Settings</button>' +
						'    <button id="wa-media-permission-retry" class="wa-card-btn" style="padding:4px 8px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Retry</button>' +
						'  </div>' +
						'</div>' +
						'<div id="wa-media-permission-details" class="wa-text-muted" style="font-size:10.5px;line-height:1.45;"></div>';
					quickGrid.appendChild(mediaPermissionCard);
				}

				// Card 1: Desktop Notifications
				var cardNotifications = document.createElement('div');
				cardNotifications.className = 'wa-modal-card';
				cardNotifications.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;align-items:center;justify-content:space-between;gap:16px;';
				cardNotifications.innerHTML = '' +
					'<div>' +
					'  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'    <strong class="wa-text-primary" style="font-size:12.5px;">Desktop Notifications</strong>' +
					'    <span id="wa-badge-notifications" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'  </div>' +
					'  <div class="wa-text-muted" style="font-size:11px;">Allow or block system notifications on macOS, Linux, and Windows.</div>' +
					'</div>' +
					'<button id="wa-action-toggle-notifications" class="wa-card-btn" style="flex-shrink:0;min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Disable</button>';
				quickGrid.appendChild(cardNotifications);

				// Card 1: Privacy Mode
				var cardPrivacy = document.createElement('div');
				cardPrivacy.className = 'wa-modal-card';
				cardPrivacy.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;flex-direction:column;gap:8px;';
				cardPrivacy.innerHTML = '' +
					'<div style="display:flex;align-items:center;justify-content:space-between;gap:16px;">' +
					'  <div style="flex:1;min-width:0;">' +
					'    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'      <strong class="wa-text-primary" style="font-size:12.5px;">Privacy Mode</strong>' +
					'      <span id="wa-badge-priv" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'    </div>' +
					'    <div class="wa-text-muted" style="font-size:11px;">Hide names, previews, timestamps & message text until you turn this off. Hover a chat to reveal its details; reply box stays usable.</div>' +
					'  </div>' +
					'  <div style="display:flex;align-items:center;justify-content:flex-end;gap:12px;min-width:150px;flex-shrink:0;">' +
					'    <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+Shift+P</span>' +
					'    <button id="wa-action-toggle-priv" class="wa-card-btn" style="min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Toggle</button>' +
					'  </div>' +
					'</div>' +
					'<label style="display:flex;align-items:center;gap:8px;cursor:pointer;user-select:none;">' +
					'  <input type="checkbox" id="wa-priv-autolock" style="width:14px;height:14px;accent-color:#00a884;cursor:pointer;margin:0;" />' +
					'  <span class="wa-text-muted" style="font-size:11px;">Auto-lock when window loses focus (unblurs on focus)</span>' +
					'</label>' +
					'<label style="display:flex;align-items:center;gap:8px;cursor:pointer;user-select:none;">' +
					'  <input type="checkbox" id="wa-blur-chatlist-only" style="width:14px;height:14px;accent-color:#00a884;cursor:pointer;margin:0;" />' +
					'  <span class="wa-text-muted" style="font-size:11px;">Blur chat list only (keep opened chat clear)</span>' +
					'</label>' +
					'<label style="display:flex;align-items:center;gap:8px;cursor:pointer;user-select:none;">' +
					'  <input type="checkbox" id="wa-blur-avatars" style="width:14px;height:14px;accent-color:#00a884;cursor:pointer;margin:0;" />' +
					'  <span class="wa-text-muted" style="font-size:11px;">Also blur profile photos (hover to peek)</span>' +
					'</label>';
				quickGrid.appendChild(cardPrivacy);

				// Card 2: Always on Top
				var cardPin = document.createElement('div');
				cardPin.className = 'wa-modal-card';
				cardPin.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;align-items:center;justify-content:space-between;gap:16px;';
				cardPin.innerHTML = '' +
					'<div style="flex:1;min-width:0;">' +
					'  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'    <strong class="wa-text-primary" style="font-size:12.5px;">Always on Top</strong>' +
					'    <span id="wa-badge-pin" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'  </div>' +
					'  <div class="wa-text-muted" style="font-size:11px;">Keep window floating above other applications.</div>' +
					'</div>' +
					'<div style="display:flex;align-items:center;justify-content:flex-end;gap:12px;min-width:150px;flex-shrink:0;">' +
					'  <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+Shift+T</span>' +
					'  <button id="wa-action-toggle-pin" class="wa-card-btn" style="min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Toggle</button>' +
					'</div>';
				quickGrid.appendChild(cardPin);

				// Card 3: Audio Mute
				var cardMute = document.createElement('div');
				cardMute.className = 'wa-modal-card';
				cardMute.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;align-items:center;justify-content:space-between;gap:16px;';
				cardMute.innerHTML = '' +
					'<div style="flex:1;min-width:0;">' +
					'  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'    <strong class="wa-text-primary" style="font-size:12.5px;">Notification Audio</strong>' +
					'    <span id="wa-badge-mute" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'  </div>' +
					'  <div class="wa-text-muted" style="font-size:11px;">Mute all notification sounds and media audio.</div>' +
					'</div>' +
					'<div style="display:flex;align-items:center;justify-content:flex-end;gap:12px;min-width:150px;flex-shrink:0;">' +
					'  <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+Shift+M</span>' +
					'  <button id="wa-action-toggle-mute" class="wa-card-btn" style="min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Toggle</button>' +
					'</div>';
				quickGrid.appendChild(cardMute);

				// Card 4: Auto-Start
				var cardAuto = document.createElement('div');
				cardAuto.className = 'wa-modal-card';
				cardAuto.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;align-items:center;justify-content:space-between;gap:16px;';
				cardAuto.innerHTML = '' +
					'<div style="flex:1;min-width:0;">' +
					'  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'    <strong class="wa-text-primary" style="font-size:12.5px;">Launch at Startup</strong>' +
					'    <span id="wa-badge-auto" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'  </div>' +
					'  <div class="wa-text-muted" style="font-size:11px;">Automatically start WhatsApp Desk on system login.</div>' +
					'</div>' +
					'<div style="display:flex;align-items:center;justify-content:flex-end;gap:12px;min-width:150px;flex-shrink:0;">' +
					'  <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+Shift+S</span>' +
					'  <button id="wa-action-toggle-auto" class="wa-card-btn" style="min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Toggle</button>' +
					'</div>';
				quickGrid.appendChild(cardAuto);

				// Card 5: App Lock (PIN / Passcode)
				var cardLock = document.createElement('div');
				cardLock.className = 'wa-modal-card';
				cardLock.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;flex-direction:column;gap:8px;';
				cardLock.innerHTML = '' +
					'<div style="display:flex;align-items:center;justify-content:space-between;gap:16px;">' +
					'  <div style="flex:1;min-width:0;">' +
					'    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'      <strong class="wa-text-primary" style="font-size:12.5px;">App Lock (PIN)</strong>' +
					'      <span id="wa-badge-lock" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'    </div>' +
					'    <div class="wa-text-muted" style="font-size:11px;">Lock application with a PIN. Unlock requires your PIN.</div>' +
					'  </div>' +
					'  <div style="display:flex;align-items:center;justify-content:flex-end;gap:8px;min-width:160px;flex-shrink:0;">' +
					'    <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+L</span>' +
					'    <button id="wa-action-toggle-lock" class="wa-card-btn" style="min-width:68px;padding:4px 8px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Lock</button>' +
					'    <button id="wa-action-config-lock" class="wa-card-btn" style="min-width:64px;padding:4px 8px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Set PIN</button>' +
					'  </div>' +
					'</div>' +
					'<label style="display:flex;align-items:center;gap:8px;cursor:pointer;user-select:none;">' +
					'  <input type="checkbox" id="wa-lock-on-focus-loss" style="width:14px;height:14px;accent-color:#00a884;cursor:pointer;margin:0;" />' +
					'  <span class="wa-text-muted" style="font-size:11px;">Auto-lock with PIN when window loses focus</span>' +
					'</label>';
				quickGrid.appendChild(cardLock);

				modal.appendChild(quickGrid);

				// Section 2: Download Folder Settings
				var folderSection = document.createElement('div');
				folderSection.className = 'wa-modal-card';
				folderSection.style.cssText = 'display:flex;flex-direction:column;gap:8px;border-radius:0;border-width:0 0 1px;border-style:solid;padding:14px 0;';
				folderSection.innerHTML = '' +
					'<div style="display:flex;align-items:center;justify-content:space-between;">' +
					'  <strong class="wa-text-primary" style="font-size:12.5px;">Downloads folder</strong>' +
					'  <button id="wa-btn-reset-folder" style="background:transparent;border:none;color:#00a884;font-size:11px;cursor:pointer;padding:2px 4px;">Use default</button>' +
					'</div>' +
					'<div class="wa-text-muted" style="font-size:11px;">Files & media downloaded from chat are permanently saved here:</div>' +
					'<div id="wa-folder-box" style="display:flex;align-items:center;border-width:1px;border-style:solid;border-radius:6px;padding:6px 8px;min-width:0;">' +
					'  <span id="wa-folder-path" style="font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;flex:1;font-family:monospace;">Loading...</span>' +
					'</div>' +
					'<div style="display:flex;align-items:center;gap:6px;margin-top:2px;">' +
					'  <button id="wa-btn-change-folder" class="wa-card-btn" style="flex:1;padding:6px 10px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;">Change Folder Location...</button>' +
					'  <button id="wa-btn-open-folder" style="background:#00a884;color:#111b21;border:none;padding:6px 12px;border-radius:6px;font-size:11.5px;font-weight:600;cursor:pointer;">' + (isMac ? 'Open in Finder' : 'Open Folder') + '</button>' +
					'</div>' +
					'<label style="display:flex;align-items:center;gap:8px;cursor:pointer;user-select:none;margin-top:2px;">' +
					'  <input type="checkbox" id="wa-organize-month" style="width:14px;height:14px;accent-color:#00a884;cursor:pointer;margin:0;" />' +
					'  <span class="wa-text-muted" style="font-size:11px;">Organize into monthly subfolders (2026-09)</span>' +
					'</label>';
				modal.appendChild(folderSection);

				// Section 3: Maintenance & Update Actions
				var actionsSection = document.createElement('div');
				actionsSection.className = 'wa-modal-card';
				actionsSection.style.cssText = 'display:flex;flex-direction:column;gap:8px;border-radius:0;border-width:0 0 1px;border-style:solid;padding:14px 0;';
				actionsSection.innerHTML = '' +
					'<strong class="wa-text-primary" style="font-size:12.5px;">Maintenance</strong>' +
					'<div style="display:grid;grid-template-columns:1fr 1fr;gap:6px;">' +
					'  <button id="wa-btn-check-updates-modal" class="wa-card-btn" style="padding:6px 8px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;text-align:center;">Check for updates</button>' +
					'  <button id="wa-btn-reload-modal" class="wa-card-btn" style="padding:6px 8px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;text-align:center;">Reload chat</button>' +
					'  <button id="wa-btn-hardref-modal" class="wa-card-btn" style="padding:6px 8px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;text-align:center;">Clear cache</button>' +
					'  <button id="wa-btn-onboard-modal" class="wa-card-btn" style="padding:6px 8px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;text-align:center;">View welcome guide</button>' +
					'</div>';
				modal.appendChild(actionsSection);

				// Section 4: Help & local diagnostics. This intentionally performs no
				// network request and never reads chat data; it only validates the
				// small native bridge surface used by the application.
				var helpSection = document.createElement('div');
				helpSection.className = 'wa-modal-card';
				helpSection.style.cssText = 'display:flex;flex-direction:column;gap:8px;border-radius:0;border-width:0 0 1px;border-style:solid;padding:14px 0;';
				helpSection.innerHTML = '' +
					'<div style="display:flex;align-items:center;justify-content:space-between;gap:12px;">' +
					'  <div><strong class="wa-text-primary" style="font-size:12.5px;">Help & diagnostics</strong><div class="wa-text-muted" style="font-size:11px;margin-top:2px;">Check the app surface locally. No chats or files are sent.</div></div>' +
					'  <button id="wa-btn-run-diagnostics" class="wa-card-btn" style="padding:6px 10px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;white-space:nowrap;">Run quick check</button>' +
					'</div>' +
					'<div id="wa-diagnostics-result" class="wa-text-muted" aria-live="polite" style="display:none;font-size:10.5px;line-height:1.45;border-radius:6px;padding:7px 8px;"></div>' +
					'<button id="wa-btn-show-shortcuts" style="align-self:flex-start;background:transparent;border:none;color:#00a884;font-size:11px;cursor:pointer;padding:2px 0;">View keyboard shortcuts</button>' +
					'<div id="wa-shortcuts-list" class="wa-text-muted" style="display:none;font-size:10.5px;line-height:1.65;"></div>';
				modal.appendChild(helpSection);

				// Disclaimer
				var disclaimer = document.createElement('div');
				disclaimer.className = 'wa-text-muted';
				disclaimer.style.cssText = 'font-size:10px;line-height:1.4;border-top-width:1px;border-top-style:solid;padding-top:8px;margin-top:2px;';
				disclaimer.innerHTML = '<strong>WhatsApp Desk</strong> is an independent application and is not affiliated with Meta.';
				modal.appendChild(disclaimer);

				// Footer
				var footer = document.createElement('div');
				footer.style.cssText = 'display:flex;justify-content:space-between;align-items:center;margin-top:2px;';
				footer.innerHTML = '<span class="wa-text-muted" style="font-size:10.5px;">Press <kbd style="padding:1px 3px;border-radius:3px;font-family:monospace;">Esc</kbd> to close</span>';
				var footLeft = document.createElement('div');
				footLeft.style.cssText = 'display:flex;align-items:center;gap:8px;';
				var btnReport = document.createElement('button');
				btnReport.textContent = '🐞 Report issue';
				btnReport.id = 'wa-btn-report';
				btnReport.title = 'Open a pre-filled GitHub issue with recent errors (nothing is sent automatically)';
				btnReport.style.cssText = 'background:transparent;border:none;color:#8696a0;font-size:11px;cursor:pointer;padding:5px 8px;';
				btnReport.onclick = function() {
					closeSettings();
					if (window.reportIssueNow) window.reportIssueNow();
				};
				var btnDone = document.createElement('button');
				btnDone.textContent = 'Done';
				btnDone.id = 'wa-btn-done';
				btnDone.style.cssText = 'padding:5px 16px;border-radius:6px;font-size:11.5px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;';
				footLeft.appendChild(btnReport);
				footLeft.appendChild(btnDone);
				footer.appendChild(footLeft);
				modal.appendChild(footer);

				overlay.appendChild(modal);
				document.body.appendChild(overlay);
				modal.addEventListener('pointerdown', function(e) { e.stopPropagation(); });
				modal.addEventListener('click', function(e) { e.stopPropagation(); });

				function closeSettings() {
					window.removeEventListener('keydown', onKeyClose);
					window.syncModalTheme = null;
					if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
				}
				function onKeyClose(e) {
					if (e.key === 'Escape') closeSettings();
				}
				window.addEventListener('keydown', onKeyClose);
				btnDone.onclick = closeSettings;
				document.getElementById('wa-settings-close-x').onclick = closeSettings;
				overlay.onclick = function(e) {
					if (e.target === overlay) closeSettings();
				};

				// Styling Synchronizer for Modal (Dark / Light Theme)
				window.syncModalTheme = function(isThemeDark) {
					var bg = isThemeDark ? '#111b21' : '#ffffff';
					var cardBg = bg;
					var border = isThemeDark ? '#2a3942' : '#d1d7db';
					var textPri = isThemeDark ? '#e9edef' : '#111b21';
					var textMut = isThemeDark ? '#8696a0' : '#667781';
					var accent = isThemeDark ? '#00a884' : '#008069';

					modal.style.background = bg;
					modal.style.border = '1px solid ' + border;
					header.style.borderBottomColor = border;
					document.getElementById('wa-modal-title').style.color = textPri;
					document.getElementById('wa-modal-sub').style.color = textMut;
					document.getElementById('wa-modal-icon-wrap').style.background = accent;
					document.getElementById('wa-modal-icon-wrap').style.color = accent;
					document.getElementById('wa-settings-close-x').style.color = textMut;

					document.querySelectorAll('.wa-modal-card').forEach(function(el) {
						el.style.background = cardBg;
						el.style.borderColor = border;
					});
					document.querySelectorAll('.wa-text-primary').forEach(function(el) {
						el.style.color = textPri;
					});
					document.querySelectorAll('.wa-text-muted').forEach(function(el) {
						el.style.color = textMut;
					});

					var fBox = document.getElementById('wa-folder-box');
					if (fBox) {
						fBox.style.background = isThemeDark ? '#111b21' : '#ffffff';
						fBox.style.borderColor = border;
					}
					var fPath = document.getElementById('wa-folder-path');
					if (fPath) fPath.style.color = textMut;

					var btnOpen = document.getElementById('wa-btn-open-folder');
					if (btnOpen) {
						btnOpen.style.background = accent;
						btnOpen.style.color = isThemeDark ? '#111b21' : '#ffffff';
					}

					document.querySelectorAll('.wa-card-btn').forEach(function(el) {
						el.style.background = isThemeDark ? '#111b21' : '#ffffff';
						el.style.borderColor = border;
						el.style.color = textPri;
					});

					btnDone.style.background = isThemeDark ? '#202c33' : '#e9edef';
					btnDone.style.borderColor = border;
					btnDone.style.color = textPri;

					// Theme segment buttons
					['dark', 'light', 'system'].forEach(function(mode) {
						var tBtn = document.getElementById('wa-theme-btn-' + mode);
						if (tBtn) {
							var active = (currentTheme === mode);
							tBtn.style.background = active ? accent : (isThemeDark ? '#111b21' : '#ffffff');
							tBtn.style.color = active ? (isThemeDark ? '#111b21' : '#ffffff') : textPri;
							tBtn.style.borderColor = active ? accent : border;
						}
					});
				};

				// Synchronize Toggle Badges & Button States
				function updateBadges() {
					var isThemeDark = currentTheme === 'system' ?
						(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) :
						(currentTheme === 'dark');
					var accent = isThemeDark ? '#00a884' : '#008069';

					var privActive = window.isPrivacyModeActive ? window.isPrivacyModeActive() : false;
					var notificationActive = window.isNotificationsEnabled ? window.isNotificationsEnabled() : true;
					var badgeNotifications = document.getElementById('wa-badge-notifications');
					var btnNotifications = document.getElementById('wa-action-toggle-notifications');
					if (badgeNotifications && btnNotifications) {
						badgeNotifications.textContent = notificationActive ? 'Enabled' : 'Disabled';
						badgeNotifications.style.background = notificationActive ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
						badgeNotifications.style.color = notificationActive ? accent : '#8696a0';
						btnNotifications.textContent = notificationActive ? 'Disable' : 'Enable';
					}
					var badgePriv = document.getElementById('wa-badge-priv');
					var btnPriv = document.getElementById('wa-action-toggle-priv');
					if (badgePriv && btnPriv) {
						badgePriv.textContent = privActive ? 'Enabled' : 'Disabled';
						badgePriv.style.background = privActive ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
						badgePriv.style.color = privActive ? accent : '#8696a0';
						btnPriv.textContent = privActive ? 'Disable' : 'Enable';
					}

					var pinActive = window.isAlwaysOnTopActive ? window.isAlwaysOnTopActive() : false;
					var badgePin = document.getElementById('wa-badge-pin');
					var btnPin = document.getElementById('wa-action-toggle-pin');
					if (badgePin && btnPin) {
						badgePin.textContent = pinActive ? 'Pinned' : 'Unpinned';
						badgePin.style.background = pinActive ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
						badgePin.style.color = pinActive ? accent : '#8696a0';
						btnPin.textContent = pinActive ? 'Unpin' : 'Pin';
					}

					var muteActive = window.isAudioMuted ? window.isAudioMuted() : false;
					var badgeMute = document.getElementById('wa-badge-mute');
					var btnMute = document.getElementById('wa-action-toggle-mute');
					if (badgeMute && btnMute) {
						badgeMute.textContent = muteActive ? 'Muted' : 'Unmuted';
						badgeMute.style.background = muteActive ? 'rgba(234,0,56,0.15)' : 'transparent';
						badgeMute.style.color = muteActive ? '#ff5252' : accent;
						btnMute.textContent = muteActive ? 'Unmute' : 'Mute';
					}

					var autoActive = window.isAutoStartActive ? window.isAutoStartActive() : false;
					var badgeAuto = document.getElementById('wa-badge-auto');
					var btnAuto = document.getElementById('wa-action-toggle-auto');
					if (badgeAuto && btnAuto) {
						badgeAuto.textContent = autoActive ? 'Enabled' : 'Disabled';
						badgeAuto.style.background = autoActive ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
						badgeAuto.style.color = autoActive ? accent : '#8696a0';
						btnAuto.textContent = autoActive ? 'Disable' : 'Enable';
					}

					var badgeLock = document.getElementById('wa-badge-lock');
					var btnToggleLock = document.getElementById('wa-action-toggle-lock');
					var btnConfigLock = document.getElementById('wa-action-config-lock');
					var lockFocusBox = document.getElementById('wa-lock-on-focus-loss');
					if (window.hasAppLockPasscodeNative && window.getAppLockEnabledNative) {
						Promise.all([
							Promise.resolve(window.hasAppLockPasscodeNative()),
							Promise.resolve(window.getAppLockEnabledNative()),
							Promise.resolve(window.getAppLockOnFocusLossNative())
						]).then(function(res) {
							var hasPin = !!res[0];
							var enabled = !!res[1];
							var focusLoss = !!res[2];
							if (badgeLock) {
								badgeLock.textContent = hasPin ? (enabled ? 'Enabled' : 'Disabled') : 'Not Set';
								badgeLock.style.background = enabled ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
								badgeLock.style.color = enabled ? accent : '#8696a0';
							}
							if (btnToggleLock) {
								btnToggleLock.textContent = hasPin ? (enabled ? 'Lock Now' : 'Enable') : 'Set PIN';
							}
							if (btnConfigLock) {
								btnConfigLock.textContent = hasPin ? 'Change PIN' : 'Set PIN';
							}
							if (lockFocusBox) {
								lockFocusBox.checked = focusLoss;
							}
						}).catch(function() {});
					}

					window.syncModalTheme(isThemeDark);
				}
				function mediaPermissionText(status) {
					if (status === 'authorized') return 'Allowed';
					if (status === 'denied') return 'Blocked';
					if (status === 'restricted') return 'Restricted';
					return 'Not requested';
				}

				function refreshMediaPermissions() {
					if (!mediaPermissionCard) return Promise.resolve();
					var summary = document.getElementById('wa-media-permission-summary');
					var details = document.getElementById('wa-media-permission-details');
					return Promise.all([
						Promise.resolve(window.getCameraPermissionNative()).catch(function() { return 'unknown'; }),
						Promise.resolve(window.getMicrophonePermissionNative()).catch(function() { return 'unknown'; })
					]).then(function(statuses) {
						var camera = statuses[0];
						var microphone = statuses[1];
						var blocked = camera === 'denied' || camera === 'restricted' || microphone === 'denied' || microphone === 'restricted';
						if (summary) summary.textContent = 'Camera: ' + mediaPermissionText(camera) + ' · Microphone: ' + mediaPermissionText(microphone);
						if (details) details.textContent = blocked ?
							'Permission is blocked by macOS. Open System Settings → Privacy & Security → Camera/Microphone, enable WhatsApp Desk, then click Retry.' :
							'WhatsApp Desk needs these permissions for calls and voice messages. If macOS asks, allow access and click Retry.';
					});
				}

				if (mediaPermissionCard) {
					document.getElementById('wa-media-permission-settings').onclick = function() {
						if (window.openMediaPrivacySettingsNative) window.openMediaPrivacySettingsNative('camera');
					};
					document.getElementById('wa-media-permission-retry').onclick = function() {
						var retry = document.getElementById('wa-media-permission-retry');
						if (retry) { retry.disabled = true; retry.textContent = 'Checking...'; }
						if (window.requestMediaPermissionsNative) {
							try { window.requestMediaPermissionsNative(); } catch (e) {}
						}
						var request = navigator.mediaDevices && navigator.mediaDevices.getUserMedia ?
							navigator.mediaDevices.getUserMedia({ audio: true, video: true }).then(function(stream) {
								stream.getTracks().forEach(function(track) { track.stop(); });
							}).catch(function() {}) : Promise.resolve();
						request.then(refreshMediaPermissions).then(function() {
							if (retry) { retry.disabled = false; retry.textContent = 'Retry'; }
						});
					};
					refreshMediaPermissions();
				}

				document.getElementById('wa-action-toggle-notifications').onclick = function() {
					if (window.setNotificationsEnabled) {
						window.setNotificationsEnabled(!window.isNotificationsEnabled()).then(function() {
							updateBadges();
							showFloatingToast(window.isNotificationsEnabled() ? '🔔 Desktop notifications enabled' : '🔕 Desktop notifications disabled');
						});
					}
				};
				if (window.refreshNotificationsEnabled) {
					window.refreshNotificationsEnabled().then(function() { updateBadges(); });
				}
				updateBadges();
				if (window.refreshAutoStartState) {
					window.refreshAutoStartState().then(function() { updateBadges(); });
				}

				// Hook Theme Segmented Control
				document.getElementById('wa-theme-btn-dark').onclick = function() {
					window.setAppTheme('dark');
					updateBadges();
				};
				document.getElementById('wa-theme-btn-light').onclick = function() {
					window.setAppTheme('light');
					updateBadges();
				};
				document.getElementById('wa-theme-btn-system').onclick = function() {
					window.setAppTheme('system');
					updateBadges();
				};

				// Hook Click Actions
				document.getElementById('wa-action-toggle-priv').onclick = function() {
					if (window.togglePrivacyMode) window.togglePrivacyMode();
					updateBadges();
				};
				var autoLockBox = document.getElementById('wa-priv-autolock');
				if (autoLockBox) {
					autoLockBox.checked = !!(window.isPrivacyAutoLock && window.isPrivacyAutoLock());
					autoLockBox.onchange = function() {
						if (window.setPrivacyAutoLock) window.setPrivacyAutoLock(autoLockBox.checked);
						showFloatingToast(autoLockBox.checked ?
							'🔒 Privacy auto-lock: on (blurs when window loses focus)' :
							'🔓 Privacy auto-lock: off');
					};
				}
				var chatListBox = document.getElementById('wa-blur-chatlist-only');
				if (chatListBox) {
					chatListBox.checked = !!(window.isBlurChatListOnly && window.isBlurChatListOnly());
					chatListBox.onchange = function() {
						if (window.setBlurChatListOnly) window.setBlurChatListOnly(chatListBox.checked);
						showFloatingToast(chatListBox.checked ?
							'🔒 Blur chat list only: on (opened chat stays clear)' :
							'🔓 Blur chat list only: off (full privacy mode)');
					};
				}
				var avatarBox = document.getElementById('wa-blur-avatars');
				if (avatarBox) {
					avatarBox.checked = !!(window.isBlurAvatars && window.isBlurAvatars());
					avatarBox.onchange = function() {
						if (window.setBlurAvatars) window.setBlurAvatars(avatarBox.checked);
						showFloatingToast(avatarBox.checked ?
							'🙈 Profile photos: blurred (hover to peek)' :
							'🙉 Profile photos: visible');
					};
				}
				document.getElementById('wa-action-toggle-pin').onclick = function() {
					if (window.toggleAlwaysOnTop) {
						window.toggleAlwaysOnTop().then(function() { updateBadges(); });
					}
				};
				document.getElementById('wa-action-toggle-mute').onclick = function() {
					if (window.toggleMuteAudio) window.toggleMuteAudio();
					updateBadges();
				};
				document.getElementById('wa-action-toggle-auto').onclick = function() {
					if (window.toggleAutoStart) {
						window.toggleAutoStart().then(function() { updateBadges(); });
					}
				};
				document.getElementById('wa-action-toggle-lock').onclick = function() {
					if (!window.hasAppLockPasscodeNative) return;
					Promise.resolve(window.hasAppLockPasscodeNative()).then(function(hasPin) {
						if (!hasPin) {
							if (window.promptConfigureAppLock) {
								window.promptConfigureAppLock(updateBadges);
							}
						} else {
							closeSettings();
							if (window.lockApp) window.lockApp();
						}
					});
				};
				document.getElementById('wa-action-config-lock').onclick = function() {
					if (window.promptConfigureAppLock) {
						window.promptConfigureAppLock(updateBadges);
					}
				};
				var lockFocusBoxEl = document.getElementById('wa-lock-on-focus-loss');
				if (lockFocusBoxEl) {
					lockFocusBoxEl.onchange = function() {
						if (window.setAppLockOnFocusLossNative) {
							window.setAppLockOnFocusLossNative(lockFocusBoxEl.checked);
							if (typeof window.refreshAppLockState === 'function') {
								window.refreshAppLockState();
							}
							showFloatingToast(lockFocusBoxEl.checked ?
								'🔒 Auto-lock saat jendela kehilangan fokus: Aktif' :
								'🔓 Auto-lock saat jendela kehilangan fokus: Nonaktif');
						}
					};
				}

				document.getElementById('wa-btn-check-updates-modal').onclick = function() {
					closeSettings();
					if (window.triggerCheckForUpdate) window.triggerCheckForUpdate();
				};
				document.getElementById('wa-btn-reload-modal').onclick = function() {
					if (window.reloadWhatsApp) window.reloadWhatsApp();
				};
				document.getElementById('wa-btn-hardref-modal').onclick = function() {
					if (window.hardRefreshWhatsApp) window.hardRefreshWhatsApp();
				};
				document.getElementById('wa-btn-onboard-modal').onclick = function() {
					closeSettings();
					if (window.showOnboardingModal) window.showOnboardingModal();
				};

				var shortcutsBtn = document.getElementById('wa-btn-show-shortcuts');
				var shortcutsList = document.getElementById('wa-shortcuts-list');
				if (shortcutsBtn && shortcutsList) {
					var modifier = isMac ? 'Cmd' : 'Ctrl';
					shortcutsList.innerHTML =
						'<div><strong class="wa-text-primary">' + modifier + '+,</strong> &mdash; Settings &amp; Controls</div>' +
						'<div><strong class="wa-text-primary">' + modifier + '+L</strong> &mdash; Lock application with PIN</div>' +
						'<div><strong class="wa-text-primary">' + modifier + '+Shift+D</strong> &mdash; Open downloads folder</div>' +
						'<div><strong class="wa-text-primary">' + modifier + '+Shift+U</strong> &mdash; Check for updates</div>' +
						'<div><strong class="wa-text-primary">' + modifier + '+Shift+P / T / M / S</strong> &mdash; Privacy / on top / mute / startup</div>' +
						'<div><strong class="wa-text-primary">Esc</strong> &mdash; Close this window</div>';
					shortcutsBtn.onclick = function() {
						var open = shortcutsList.style.display !== 'none';
						shortcutsList.style.display = open ? 'none' : 'block';
						shortcutsBtn.textContent = open ? 'View keyboard shortcuts' : 'Hide keyboard shortcuts';
					};
				}

				var diagnosticsBtn = document.getElementById('wa-btn-run-diagnostics');
				var diagnosticsResult = document.getElementById('wa-diagnostics-result');
				if (diagnosticsBtn && diagnosticsResult) {
					diagnosticsBtn.onclick = function() {
						var checks = [];
					var requiredBindings = ['getDownloadDirNative', 'openDownloadDirNative', 'checkForUpdateNative', 'checkFileExistsNative'];
						var missing = requiredBindings.filter(function(name) { return typeof window[name] !== 'function'; });
						checks.push(missing.length ? 'Native bridge: unavailable (' + missing.join(', ') + ')' : 'Native bridge: ready');
						checks.push(navigator.onLine === false ? 'Network: offline (chat may not refresh)' : 'Network: available');
						try {
							var key = 'wa-desk-diagnostic-probe';
							localStorage.setItem(key, '1');
							localStorage.removeItem(key);
							checks.push('Local settings storage: ready');
						} catch (e) { checks.push('Local settings storage: unavailable (preferences will not persist)'); }
						var healthy = !missing.length && navigator.onLine !== false;
						diagnosticsResult.style.display = 'block';
						diagnosticsResult.style.background = healthy ? 'rgba(0,168,132,.10)' : 'rgba(234,0,56,.10)';
						diagnosticsResult.innerHTML = '<strong class="wa-text-primary">' + (healthy ? 'Quick check complete' : 'Attention needed') + '</strong><br>' + checks.map(function(line) { return '• ' + line; }).join('<br>');
					};
				}

				// Populate current download dir
				var pathLabel = document.getElementById('wa-folder-path');
				if (window.getDownloadDirNative) {
					window.getDownloadDirNative().then(function(dir) {
						if (pathLabel) pathLabel.textContent = dir;
					});
				}

				// Change folder action
				document.getElementById('wa-btn-change-folder').onclick = function() {
					if (window.chooseDownloadDirNative) {
						window.chooseDownloadDirNative().then(function(newDir) {
							if (newDir && pathLabel) {
								pathLabel.textContent = newDir;
								showFloatingToast('📁 Downloads folder updated!');
							}
						});
					}
				};

				// Open folder action
				document.getElementById('wa-btn-open-folder').onclick = function() {
					if (window.openDownloadDirNative) {
						window.openDownloadDirNative();
						showFloatingToast('📁 Opening folder in file manager...');
					}
				};

				// Reset folder action
				document.getElementById('wa-btn-reset-folder').onclick = function() {
					if (window.resetDownloadDirNative) {
						window.resetDownloadDirNative().then(function(defDir) {
							if (pathLabel) pathLabel.textContent = defDir;
							showFloatingToast('📁 Downloads folder reset to default.');
						});
					}
				};

				var organizeBox = document.getElementById('wa-organize-month');
				if (organizeBox) {
					if (window.getOrganizeByMonthNative) {
						window.getOrganizeByMonthNative().then(function(on) {
							organizeBox.checked = !!on;
						}).catch(function() {});
					}
					organizeBox.onchange = function() {
						if (!window.setOrganizeByMonthNative) return;
						window.setOrganizeByMonthNative(organizeBox.checked).then(function(applied) {
							showFloatingToast(applied ?
								'🗂️ Downloads will be organized into monthly folders.' :
								'🗂️ Downloads save directly to the folder again.');
						}).catch(function() {});
					};
				}
			};

		});

		} catch (waInitError) {
			// A module above failed (an engine API difference, denied
			// storage, ...). Record it and keep going: everything below
			// must still be installed.
			waNoteRecoverable('init-body', waInitError);
		}

		// Escape must close the active WhatsApp chat without invoking macOS's
		// default fullscreen exit behavior. When no chat is active, keep the
		// window fullscreen as well while allowing WhatsApp to handle its own UI.
		// Native/app-owned overlays keep their existing Escape handlers and are
		// deliberately excluded here.
		waRunModule('escape-chat', function() {
			if (__WA_GOOS !== 'darwin') return;
			function nativeOverlayOpen() {
				return !!document.querySelector('#wa-settings-overlay, #wa-doc-modal-overlay, #wa-onboarding-overlay, #wa-recovery-overlay, [data-testid="media-viewer"]');
			}
			function activeChatHeader() {
				var main = document.getElementById('main');
				return main && main.querySelector('header');
			}
			function closeChatFromEscape() {
				if (nativeOverlayOpen()) return false;
				var header = activeChatHeader();
				if (!header) return false;
				var back = header.querySelector([
					'button[data-testid="back"]', '[data-testid="back"]',
					'[data-icon="back"]', 'button[aria-label*="Back" i]',
					'[role="button"][aria-label*="Back" i]',
					'button[aria-label*="Kembali" i]',
					'[role="button"][aria-label*="Kembali" i]',
					'button[title*="Back" i]'
				].join(','));
				if (back) {
					var control = back.closest && back.closest('button, [role="button"]');
					(control || back).click();
					return true;
				}
				return false;
			}
			window.addEventListener('keydown', function(e) {
				if (e.key !== 'Escape' || !e.isTrusted || nativeOverlayOpen()) return;
				e.preventDefault();
				if (closeChatFromEscape()) e.stopImmediatePropagation();
			}, true);
		});

		// Quoted Message Jump & Highlight (matches WhatsApp Official Desktop)
		// When clicking a quoted reply bubble in chat, smoothly scrolls to the
		// original message and pulses a highlight animation. If the target message
		// is outside the virtualized DOM buffer, paginates backwards to load earlier
		// history until the message is located.
		waRunModule('quoted-message-jump', function() {
			var style = document.createElement('style');
			style.textContent = '' +
				'@keyframes wa-quoted-highlight-pulse {' +
				'  0% { background-color: rgba(0, 168, 132, 0.45) !important; outline: 2px solid #00a884 !important; }' +
				'  35% { background-color: rgba(0, 168, 132, 0.3) !important; outline: 2px solid rgba(0, 168, 132, 0.4) !important; }' +
				'  100% { background-color: transparent !important; outline: 2px solid transparent !important; }' +
				'}' +
				'.wa-quoted-highlight-flash {' +
				'  animation: wa-quoted-highlight-pulse 1.8s cubic-bezier(0.2, 0.8, 0.2, 1) !important;' +
				'  border-radius: 8px !important;' +
				'  transition: background-color 0.3s ease !important;' +
				'}' +
				'[data-testid="quoted-message"], [role="button"]._ak8j, div._ak8j {' +
				'  cursor: pointer !important;' +
				'}';

			function injectStyle() {
				var head = document.head || document.documentElement || document.body;
				if (head) {
					head.appendChild(style);
				} else {
					document.addEventListener('DOMContentLoaded', function() {
						var h = document.head || document.documentElement || document.body;
						if (h) h.appendChild(style);
					}, { once: true });
				}
			}
			injectStyle();

			function getChatScrollContainer() {
				var main = document.getElementById('main');
				if (!main) return null;
				var candidates = main.querySelectorAll('div');
				for (var i = 0; i < candidates.length; i++) {
					var el = candidates[i];
					if (el.scrollHeight > el.clientHeight && el.clientHeight > 180) {
						var overflow = window.getComputedStyle(el).overflowY;
						if (overflow === 'auto' || overflow === 'scroll') return el;
					}
				}
				return main.querySelector('[data-testid="conversation-panel-messages"]') || main;
			}

			function extractQuotedInfo(quoteEl, msgContainer) {
				var info = {
					stanzaId: '',
					fullId: '',
					sender: '',
					text: ''
				};

				// 1. Fiber inspection
				try {
					var checkNodes = [quoteEl, quoteEl.parentElement, msgContainer];
					for (var n = 0; n < checkNodes.length; n++) {
						var node = checkNodes[n];
						if (!node) continue;
						var fiberKey = Object.keys(node).find(function(k) {
							return k.indexOf('__reactFiber') === 0 || k.indexOf('__reactInternalInstance') === 0;
						});
						if (!fiberKey) continue;
						var curr = node[fiberKey];
						var depth = 0;
						while (curr && depth < 20) {
							var props = curr.memoizedProps;
							if (props) {
								var m = props.msg || props.message || (props.item && props.item.id ? props.item : null);
								if (m) {
									if (m.quotedStanzaID) info.stanzaId = String(m.quotedStanzaID);
									if (m.quotedParticipant) info.sender = String(m.quotedParticipant);
									if (m.quotedMsg) {
										if (typeof m.quotedMsg.id === 'string') info.fullId = m.quotedMsg.id;
										else if (m.quotedMsg.id && m.quotedMsg.id._serialized) info.fullId = m.quotedMsg.id._serialized;
										if (m.quotedMsg.body) info.text = String(m.quotedMsg.body);
										else if (m.quotedMsg.caption) info.text = String(m.quotedMsg.caption);
									}
								}
								if (props.quotedMsg) {
									var qm = props.quotedMsg;
									if (typeof qm.id === 'string') info.fullId = qm.id;
									else if (qm.id && qm.id._serialized) info.fullId = qm.id._serialized;
									if (qm.stanzaId) info.stanzaId = String(qm.stanzaId);
									if (qm.body) info.text = String(qm.body);
									else if (qm.caption) info.text = String(qm.caption);
								}
							}
							if (info.stanzaId || info.fullId) break;
							curr = curr.return;
							depth++;
						}
						if (info.stanzaId || info.fullId) break;
					}
				} catch (e) {}

				// 2. DOM text extraction fallback
				try {
					var senderEl = quoteEl.querySelector('[dir="auto"], strong, ._ak8l, [class*="author"], [class*="sender"]');
					if (senderEl && !info.sender) {
						info.sender = (senderEl.textContent || '').trim();
					}
					var spans = quoteEl.querySelectorAll('span, div');
					for (var i = 0; i < spans.length; i++) {
						var s = spans[i];
						if (s === senderEl || (senderEl && senderEl.contains(s))) continue;
						if (!s.children || s.children.length === 0) {
							var txt = (s.textContent || '').trim();
							txt = txt.replace(/^[\uD800-\uDBFF\uDC00-\uDFFF\s\W\u2500-\u2BFF]+/, '').trim();
							if (txt && txt.length > 2 && (!info.text || txt.length > info.text.length)) {
								info.text = txt;
							}
						}
					}
				} catch (e2) {}

				return info;
			}

			function findTargetMessageInDom(info, quoteEl, msgContainer) {
				if (!info) return null;
				var main = document.getElementById('main');
				if (!main) return null;

				// Match by stanza ID
				if (info.stanzaId) {
					var matchStanza = main.querySelector('div[data-id*="' + info.stanzaId + '"], [role="row"][data-id*="' + info.stanzaId + '"]');
					if (matchStanza && matchStanza !== msgContainer && !matchStanza.contains(quoteEl)) {
						return matchStanza;
					}
				}

				// Match by full ID
				if (info.fullId) {
					var matchFull = main.querySelector('div[data-id="' + info.fullId + '"], [role="row"][data-id="' + info.fullId + '"]');
					if (matchFull && matchFull !== msgContainer && !matchFull.contains(quoteEl)) {
						return matchFull;
					}
				}

				// Match by text snippet
				if (info.text && info.text.length > 2) {
					var query = info.text.toLowerCase();
					var rows = main.querySelectorAll('[role="row"], [data-testid*="msg-container"], div[data-id]');
					for (var i = rows.length - 1; i >= 0; i--) {
						var row = rows[i];
						if (row === msgContainer || row.contains(quoteEl)) continue;
						var textNodes = row.querySelectorAll('span.selectable-text, span[dir="ltr"], span[dir="rtl"], div.copyable-text, span[dir="auto"]');
						for (var j = 0; j < textNodes.length; j++) {
							var node = textNodes[j];
							if (node.closest && node.closest('[data-testid="quoted-message"], [data-testid*="quoted"], [role="button"][class*="_ak8j"], div._ak8j')) continue;
							var content = (node.textContent || '').trim().toLowerCase();
							if (content && (content === query || content.indexOf(query) !== -1 || query.indexOf(content) !== -1)) {
								return row;
							}
						}
					}
				}

				return null;
			}

			function highlightAndScrollTo(target) {
				if (!target) return;
				try {
					target.scrollIntoView({ behavior: 'smooth', block: 'center' });
				} catch (e) {
					target.scrollIntoView(true);
				}
				var bubble = target.querySelector('div[class*="_ak"], div[class*="message"], [data-testid="msg-container"]') || target;
				bubble.classList.remove('wa-quoted-highlight-flash');
				void bubble.offsetWidth;
				bubble.classList.add('wa-quoted-highlight-flash');
				setTimeout(function() {
					bubble.classList.remove('wa-quoted-highlight-flash');
				}, 1900);
			}

			function jumpToQuotedMessage(info, quoteEl, msgContainer) {
				var target = findTargetMessageInDom(info, quoteEl, msgContainer);
				if (target) {
					highlightAndScrollTo(target);
					return;
				}

				var scrollContainer = getChatScrollContainer();
				if (!scrollContainer) return;

				var attempts = 0;
				var maxAttempts = 25;
				var timer = setInterval(function() {
					attempts++;
					var found = findTargetMessageInDom(info, quoteEl, msgContainer);
					if (found) {
						clearInterval(timer);
						highlightAndScrollTo(found);
						return;
					}
					if (attempts >= maxAttempts) {
						clearInterval(timer);
						return;
					}
					scrollContainer.scrollTop = 5;
					try {
						scrollContainer.dispatchEvent(new Event('scroll', { bubbles: true }));
					} catch (e) {}
				}, 120);
			}

			document.addEventListener('click', function(e) {
				var target = e.target;
				if (!target || typeof target.closest !== 'function') return;

				var quoteBox = target.closest(
					'[data-testid="quoted-message"], ' +
					'[data-testid*="quoted"], ' +
					'[role="button"][class*="_ak8j"], ' +
					'div._ak8j, ' +
					'[aria-label*="Quoted Message" i], ' +
					'[aria-label*="Pesan yang dikutip" i]'
				);
				if (!quoteBox) return;

				var main = document.getElementById('main');
				if (!main || !main.contains(quoteBox)) return;

				var msgContainer = quoteBox.closest('[data-testid*="msg-container"], [role="row"], div[data-id], .message-in, .message-out');
				var info = extractQuotedInfo(quoteBox, msgContainer);

				setTimeout(function() {
					jumpToQuotedMessage(info, quoteBox, msgContainer);
				}, 40);
			}, false);
		});

		// --- Core shortcuts (self-contained) ------------------------------
		// Registered outside the modules above on purpose. The Settings button
		// and Cmd/Ctrl+, used to disappear together on Windows because a single
		// exception in an earlier module aborted the rest of the injected
		// script. The controls the user needs to RECOVER from such a state must
		// not depend on the modules that can break.
		waRunModule('core-shortcuts', function() {
			// Cmd/Ctrl+, opens Settings; Cmd/Ctrl+Shift+D opens the downloads
			// folder. Both are also reachable from the Control Center itself.
			function isSettingsChord(e) {
				return (e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey &&
					(e.key === ',' || e.key === '<' || e.code === 'Comma');
			}
			window.addEventListener('keydown', function(e) {
				if (isSettingsChord(e)) {
					e.preventDefault();
					e.stopPropagation();
					// Never let a broken Control Center swallow the chord: the
					// shortcut is a recovery path, so it must not throw.
					try {
						if (typeof window.showSettingsModal === 'function') {
							window.showSettingsModal();
						} else if (typeof window.openRecoveryPanel === 'function') {
							window.openRecoveryPanel();
						}
					} catch (err) {
						waNoteRecoverable('shortcut-settings', err);
						if (typeof window.openRecoveryPanel === 'function') {
							window.openRecoveryPanel();
						}
					}
				} else if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'd' || e.key === 'D')) {
					e.preventDefault();
					e.stopPropagation();
					try {
						if (typeof window.openDownloadDirNative === 'function') {
							window.openDownloadDirNative();
							if (typeof window.showFloatingToast === 'function') {
								window.showFloatingToast('📁 Opening downloads folder...');
							}
						}
					} catch (err) {
						waNoteRecoverable('shortcut-downloads', err);
					}
				}
			}, true);
		});
		// --- Emergency Settings entry point -------------------------------
		// Guarantees a way into Settings even when the Control Center module
		// above failed to install. Deliberately depends on nothing but the DOM:
		// if the real button exists it defers to it, otherwise it mounts a
		// minimal launcher, and if even the modal is missing the launcher opens
		// a recovery panel with the recorded failures.
		waRunModule('emergency-settings', function() {
			function realEntryPointPresent() {
				return !!document.getElementById('wa-toolbar-settings-btn') ||
					!!document.getElementById('wa-settings-fallback-btn');
			}
			function showRecoveryPanel() {
				var existing = document.getElementById('wa-recovery-overlay');
				if (existing && existing.parentNode) { existing.parentNode.removeChild(existing); return; }
				var fails = (typeof window.__waRecoverable === 'function') ? window.__waRecoverable() : [];
				var overlay = document.createElement('div');
				overlay.id = 'wa-recovery-overlay';
				overlay.style.cssText = 'position:fixed;inset:0;background:rgba(8,15,19,.72);z-index:2147483647;display:flex;align-items:center;justify-content:center;padding:20px;box-sizing:border-box;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;';
				var card = document.createElement('div');
				card.style.cssText = 'width:440px;max-width:94vw;max-height:80vh;overflow:auto;background:#111b21;color:#e9edef;border:1px solid rgba(134,150,160,.35);border-radius:10px;padding:18px 20px;box-shadow:0 18px 48px rgba(0,0,0,.4);font-size:12.5px;line-height:1.6;';
				var rows = fails.length
					? fails.map(function(f) { return '<li>' + String(f).replace(/[<>&]/g, '') + '</li>'; }).join('')
					: '<li>No failures recorded.</li>';
				card.innerHTML =
					'<strong style="font-size:14px;">WhatsApp Desk — recovery</strong>' +
					'<p style="opacity:.8;margin:8px 0 10px;">The Settings panel did not load. Recorded problems:</p>' +
					'<ul style="margin:0 0 14px;padding-left:18px;opacity:.9;">' + rows + '</ul>' +
					'<button id="wa-recovery-reload" style="background:#00a884;color:#111b21;border:none;padding:8px 14px;border-radius:6px;font-weight:600;cursor:pointer;">Reload WhatsApp Web</button>';
				overlay.appendChild(card);
				document.body.appendChild(overlay);
				var btn = document.getElementById('wa-recovery-reload');
				if (btn) {
					btn.onclick = function() {
						if (typeof window.reloadWhatsApp === 'function') { window.reloadWhatsApp(); }
						else { window.location.reload(); }
					};
				}
			}
			function openSettings() {
				if (typeof window.showSettingsModal === 'function') {
					try { window.showSettingsModal(); return; } catch (e) { waNoteRecoverable('showSettingsModal', e); }
				}
				showRecoveryPanel();
			}
			window.openRecoveryPanel = showRecoveryPanel;
			// The last-resort launcher must never sit next to a working control:
			// it is a lifeline for the case where the other entry points failed,
			// not an extra gear. So it only mounts while no *visible* entry point
			// exists, and unmounts again as soon as one appears.
			function visibleEntryPointPresent() {
				var ids = ['wa-toolbar-settings-btn', 'wa-settings-fallback-btn'];
				for (var i = 0; i < ids.length; i++) {
					var el = document.getElementById(ids[i]);
					if (!el || el.style.display === 'none') continue;
					var r = el.getBoundingClientRect();
					if (r.width > 0 && r.height > 0) return true;
				}
				return false;
			}
			function mount() {
				var existing = document.getElementById('wa-emergency-settings-btn');
				if (visibleEntryPointPresent()) {
					if (existing && existing.parentNode) existing.parentNode.removeChild(existing);
					return;
				}
				if (existing) return;
				if (!document.body) return;
				var btn = document.createElement('button');
				btn.id = 'wa-emergency-settings-btn';
				btn.type = 'button';
				btn.setAttribute('aria-label', 'Open Settings and Controls');
				// This module has its own scope; compute the modifier locally
				// instead of referencing the outer isMac, which is undefined here
				// and previously threw, preventing the last-resort button mounting.
				var emgIsMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0;
				btn.title = 'Settings & Controls (' + (emgIsMac ? 'Cmd' : 'Ctrl') + ' + ,)';
				btn.innerHTML = '<svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>';
				btn.style.cssText = 'position:fixed;left:14px;bottom:14px;z-index:2147483646;width:38px;height:38px;padding:0;display:inline-flex;align-items:center;justify-content:center;border:1px solid rgba(134,150,160,.45);border-radius:50%;background:#111b21;color:#aebac1;cursor:pointer;box-shadow:0 4px 12px rgba(0,0,0,.3);';
				btn.onclick = function(e) { e.preventDefault(); e.stopPropagation(); openSettings(); };
				document.body.appendChild(btn);
			}
			// The rail fallback announces every mount/hide so this launcher can
			// re-evaluate: it must disappear the moment a real control appears.
			window.__waRecheckEmergencySettings = mount;
			mount();
			document.addEventListener('DOMContentLoaded', mount);
			window.addEventListener('load', mount);
			// The Control Center mounts asynchronously after WhatsApp's own boot.
			setTimeout(mount, 1200);
			setTimeout(mount, 4000);
		});

	` + "\n" + getOnboardingScript()
	// Single source of truth: every UI version string flows from appVersion
	// (overridable at link time via -ldflags "-X main.appVersion=...").
	return strings.ReplaceAll(script, "__WA_APP_VERSION__", appVersion)
}

type WindowState struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Maximized bool    `json:"maximized,omitempty"`
	// Screens maps a stable display identifier (macOS NSScreenNumber) to the
	// frame the window had on that monitor. Only macOS populates it; other
	// platforms round-trip it unchanged.
	Screens map[string]WindowState `json:"screens,omitempty"`
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			writeCrashReport("main", r)
		}
	}()
	runApp()
}
