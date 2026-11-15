/*!
 * Catch Browser SDK v1.0
 * Standalone, dependency-free error/log collector for third-party web apps.
 *
 * Usage:
 *   <script src="https://catch.example.com/catch-sdk.js"
 *           data-key="ck_xxx"
 *           data-env="production"
 *           data-release="web@1.2.3"
 *           data-service="partner-portal"
 *           crossorigin="anonymous"></script>
 *
 * The ingest key is write-only and safe to expose in browser code.
 */
(function (global) {
  "use strict";

  var script = document.currentScript;
  if (!script) {
    script = document.querySelector("script[data-key]");
  }
  if (!script) {
    return;
  }

  var ATTR = function (name, fallback) {
    var value = script.getAttribute(name);
    return value === null || value === "" ? fallback : value;
  };

  var KEY = ATTR("data-key", "");
  if (!KEY) {
    if (global.console) global.console.warn("[Catch] missing data-key, SDK disabled");
    return;
  }

  var SOURCE_URL = script.src ? new URL(script.src, global.location.href) : null;
  var ENDPOINT = ATTR(
    "data-endpoint",
    SOURCE_URL ? SOURCE_URL.origin + "/api/ingest" : "/api/ingest",
  );
  var ENVIRONMENT = ATTR("data-env", "production");
  var RELEASE = ATTR("data-release", "");
  var SERVICE = ATTR("data-service", global.location.hostname);
  var SAMPLE_RATE = Number(ATTR("data-sample-rate", "1"));
  var MAX_BREADCRUMBS = 30;
  var MAX_QUEUE = 100;
  var BATCH_SIZE = 20;
  var STORAGE_KEY = "catch:sdk:queue:" + KEY;
  var breadcrumbs = [];
  var queue = [];
  var flushing = false;
  var timer = null;
  var currentUser = null;

  function nowIso() {
    return new Date().toISOString();
  }

  function makeId() {
    try {
      return global.crypto.randomUUID();
    } catch (e) {
      return Date.now().toString(36) + Math.random().toString(36).slice(2);
    }
  }

  function safeStringify(value) {
    try {
      return JSON.stringify(value);
    } catch (e) {
      return String(value);
    }
  }

  function truncate(value, max) {
    if (typeof value !== "string") return value;
    return value.length > max ? value.slice(0, max) + "…" : value;
  }

  function readQueue() {
    try {
      var raw = global.localStorage.getItem(STORAGE_KEY);
      var parsed = raw ? JSON.parse(raw) : [];
      if (!Array.isArray(parsed)) return [];
      return parsed
        .filter(function (item) {
          return item && item.event && item.event.id;
        })
        .slice(-MAX_QUEUE);
    } catch (e) {
      return [];
    }
  }

  function saveQueue() {
    try {
      global.localStorage.setItem(STORAGE_KEY, JSON.stringify(queue.slice(-MAX_QUEUE)));
    } catch (e) {
      /* storage may be unavailable */
    }
  }

  function addBreadcrumb(category, message, data) {
    breadcrumbs.push({
      timestamp: nowIso(),
      category: category,
      message: truncate(message, 500),
      level: "info",
      data: data,
    });
    if (breadcrumbs.length > MAX_BREADCRUMBS) breadcrumbs.shift();
  }

  function enqueue(event) {
    if (SAMPLE_RATE < 1 && event.kind === "log" && Math.random() > SAMPLE_RATE) return;
    event.breadcrumbs = breadcrumbs.slice(-MAX_BREADCRUMBS);
    queue.push({ event: event, attempts: 0 });
    if (queue.length > MAX_QUEUE) queue.splice(0, queue.length - MAX_QUEUE);
    saveQueue();
    schedule(300);
  }

  function baseEvent(kind, level, message) {
    return {
      id: makeId(),
      kind: kind,
      source: "frontend",
      level: level,
      message: truncate(message, 8000),
      timestamp: nowIso(),
      environment: ENVIRONMENT,
      release: RELEASE,
      service: SERVICE,
      platform: "web",
      url: global.location.href,
      user: currentUser || undefined,
    };
  }

  function log(level, message, context, tags) {
    var event = baseEvent("log", level, message);
    if (context) event.context = context;
    if (tags) event.tags = tags;
    enqueue(event);
  }

  function captureError(error, context) {
    var message = "Unknown error";
    var errorType = "Error";
    var stack;
    if (error && error.stack) {
      stack = error.stack;
      errorType = error.name || "Error";
      message = error.message || errorType;
    } else if (typeof error === "string") {
      message = error;
    } else if (error && error.message) {
      message = String(error.message);
      errorType = error.name || "Error";
    } else if (error) {
      message = safeStringify(error);
    }
    var event = baseEvent("error", "error", message);
    event.errorType = errorType;
    event.stack = stack;
    if (context) event.context = context;
    enqueue(event);
  }

  function schedule(delay) {
    if (timer) return;
    timer = global.setTimeout(function () {
      timer = null;
      flush();
    }, delay);
  }

  function markFailed(ids) {
    queue = queue.filter(function (item) {
      if (ids.indexOf(item.event.id) === -1) return true;
      item.attempts = (item.attempts || 0) + 1;
      return item.attempts < 8;
    });
    saveQueue();
  }

  function flush() {
    if (flushing || !queue.length) return;
    flushing = true;
    var batch = queue.slice(0, BATCH_SIZE);
    var ids = batch.map(function (item) {
      return item.event.id;
    });
    global
      .fetch(ENDPOINT, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Ingest-Key": KEY,
        },
        body: JSON.stringify({
          events: batch.map(function (item) {
            return item.event;
          }),
        }),
        keepalive: true,
      })
      .then(function (res) {
        if (res.ok) {
          queue = queue.filter(function (item) {
            return ids.indexOf(item.event.id) === -1;
          });
          saveQueue();
        } else {
          markFailed(ids);
        }
      })
      .catch(function () {
        markFailed(ids);
      })
      .then(function () {
        flushing = false;
        if (queue.length) schedule(5000);
      });
  }

  function beacon() {
    if (!queue.length) return;
    try {
      var batch = queue.slice(0, 50);
      var ids = batch.map(function (item) {
        return item.event.id;
      });
      var url = ENDPOINT + (ENDPOINT.indexOf("?") === -1 ? "?" : "&") + "key=" + encodeURIComponent(KEY);
      var ok = global.navigator.sendBeacon(
        url,
        new Blob([JSON.stringify({ events: batch.map(function (item) { return item.event; }) })], {
          type: "text/plain;charset=UTF-8",
        }),
      );
      if (ok) {
        queue = queue.filter(function (item) {
          return ids.indexOf(item.event.id) === -1;
        });
        saveQueue();
      }
    } catch (e) {
      /* ignore */
    }
  }

  function trackPageView() {
    addBreadcrumb("navigation", "page_view", { path: global.location.pathname });
    log("info", "page_view", {
      path: global.location.pathname,
      search: global.location.search,
      referrer: global.document.referrer || undefined,
    });
  }

  function instrumentFetch() {
    if (!global.fetch) return;
    var nativeFetch = global.fetch;
    global.fetch = function (input, init) {
      var started = Date.now();
      var url = typeof input === "string" ? input : input && input.url;
      var method = (init && init.method) || (input && input.method) || "GET";
      var isCatch = url && url.indexOf(ENDPOINT) === 0;
      return nativeFetch.apply(this, arguments).then(
        function (response) {
          var duration = Date.now() - started;
          if (!isCatch) {
            addBreadcrumb("http", method + " " + url, {
              status: response.status,
              durationMs: duration,
            });
            if (response.status >= 500) {
              captureError(
                new Error("HTTP " + response.status + " " + method + " " + url),
                { kind: "fetch", status: response.status, durationMs: duration },
              );
            }
          }
          return response;
        },
        function (err) {
          if (!isCatch) {
            addBreadcrumb("http", method + " " + url, { error: String(err), durationMs: Date.now() - started });
            captureError(err, { kind: "fetch", method: method, url: url });
          }
          throw err;
        },
      );
    };
  }

  function instrumentNavigation() {
    ["pushState", "replaceState"].forEach(function (method) {
      var original = global.history[method];
      if (!original) return;
      global.history[method] = function () {
        var result = original.apply(this, arguments);
        trackPageView();
        return result;
      };
    });
    global.addEventListener("popstate", trackPageView);
  }

  global.addEventListener("error", function (event) {
    if (event.error) {
      captureError(event.error, {
        mechanism: "window.onerror",
        filename: event.filename,
        lineno: event.lineno,
        colno: event.colno,
      });
    } else {
      captureError(String(event.message || "Script error"), {
        mechanism: "window.onerror",
        filename: event.filename,
        lineno: event.lineno,
        colno: event.colno,
      });
    }
  });

  global.addEventListener("unhandledrejection", function (event) {
    captureError(event.reason, { mechanism: "unhandledrejection" });
  });

  global.addEventListener("click", function (event) {
    var target = event.target;
    if (!target || target.nodeType !== 1) return;
    var label = target.getAttribute("aria-label") || target.id || target.textContent || target.tagName;
    addBreadcrumb("ui", "click " + target.tagName.toLowerCase(), {
      label: truncate(String(label).trim(), 120),
    });
  }, true);

  global.addEventListener("pagehide", beacon);
  global.document.addEventListener("visibilitychange", function () {
    if (global.document.visibilityState === "hidden") beacon();
  });

  queue = readQueue();

  instrumentFetch();
  instrumentNavigation();
  trackPageView();
  schedule(1000);

  global.Catch = {
    version: "1.0.0",
    log: log,
    captureError: captureError,
    addBreadcrumb: addBreadcrumb,
    setUser: function (user) {
      currentUser = user
        ? {
            id: user.id ? truncate(String(user.id), 200) : undefined,
            email: user.email ? truncate(String(user.email), 320) : undefined,
            username: user.username ? truncate(String(user.username), 200) : undefined,
          }
        : null;
    },
    flush: flush,
  };
})(window);
