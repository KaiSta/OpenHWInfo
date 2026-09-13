// OpenHWInfo frontend — plain JS (no build step), loaded as ES module.
// The work is deliberately kept in one file so embedding stays trivial and the
// Wails webview has no bundler dependency.

// ---- Helpers -------------------------------------------------------
function num(v) {
    if (v === null || v === undefined) return null;
    var n = parseFloat(String(v).replace(",", "."));
    return isNaN(n) ? null : n;
}

function frac(v, max) {
    if (!max || max <= 0) return null;
    var f = Math.max(0, Math.min(1, v / max));
    var cls = "good";
    if (f > 0.85) cls = "bad";
    else if (f > 0.6) cls = "warn";
    return { pct: Math.round(f * 100), cls: cls };
}

// Temperature sensors do not share one safe ceiling: a junction/hotspot
// sensor is expected to run hotter than a CPU package sensor, and memory
// runs hotter than a core. These are the 0..100% scale ceilings.
function temperatureLimit(key) {
    if (/hotspot|junction/i.test(key)) return 110;
    if (/vram|memory/i.test(key)) return 105;
    return 100;
}

// Decide a 0..1 fraction + color class for a metric, using the card's
// full value map so sibling metrics (e.g. vram.total) are available.
function gauge(key, valStr, values) {
    var val = num(valStr);
    if (val === null) return null;

    // "temp" also covers the per-core keys; "tccd" and "pch" are
    // temperatures whose keys carry no "temp" substring.
    if (/temp|tccd|pch/i.test(key)) return frac(val, temperatureLimit(key));
    if (/usage|percent/i.test(key)) return frac(val, 100);

    // A pressure figure is the share of the last 10 seconds during which tasks
    // were stalled on memory, so it is a percentage even though its key says
    // neither "usage" nor "percent".
    if (/pressure/i.test(key)) return frac(val, 100);

    // A drive's consumed endurance behaves like a usage figure: more is worse.
    if (/life/i.test(key)) return frac(val, 100);

    // Spare capacity is the one reading that is better the higher it is, so it
    // needs its own scale: a drive whose spare pool is nearly gone is the drive
    // about to fail, and a full pool is not a drive at 100% load.
    if (/spare/i.test(key)) {
        var spare = Math.max(0, Math.min(1, val / 100));
        return {
            pct: Math.round(spare * 100),
            cls: spare < 0.15 ? "bad" : (spare < 0.3 ? "warn" : "good")
        };
    }

    if (/power/i.test(key)) {
        var max = num(values["gpu.power.max"]) || val;
        if (max > 0 && key.indexOf("avg") !== -1) return frac(val, max);
        return null;
    }

    // A "used" metric is scaled against its own total, which lives under
    // a sibling key rather than in the key name.
    var totals = {
        "vram.used": "vram.total",
        "disk.fs.used": "disk.fs.total",
        "mem.used": "mem.total",
        "swap.used": "swap.total"
    };
    var totalKey = totals[key];
    if (totalKey) {
        var total = num(values[totalKey]);
        if (total && total > 0) return frac(val, total);
    }

    return null;
}

// ---- In-place rendering --------------------------------------------
// The view is reconciled rather than rebuilt. A freshly created element is
// painted at its final width and never animates, so rebuilding the grid on
// every tick would make the bar transitions dead code; the data-key
// attributes are the handles used to find and update existing rows.

function keyOf(k) {
    return k.Key;
}

// Key -> display label, so an existing row's label can be refreshed
// without keeping the template objects around.
function labelsOf(keys) {
    var labels = {};
    keys.forEach(function (k) { labels[k.Key] = k.Label; });
    return labels;
}

function makeEl(tag, className, text) {
    var el = document.createElement(tag);
    if (className) el.className = className;
    if (text !== undefined) el.textContent = text;
    return el;
}

// Makes container hold exactly one child per key, in order. Children are
// reused when the key sequence is unchanged; otherwise the container is
// rebuilt, because the shape of the view has changed. Either way every
// child is updated on this pass: build() only creates the empty shell of
// a row, so returning straight after a rebuild would leave the table
// blank until the next poll.
function reconcile(container, keys, build, update) {
    var children = container.children;
    var rebuild = children.length !== keys.length;
    for (var i = 0; !rebuild && i < keys.length; i++) {
        if (children[i].getAttribute("data-key") !== keys[i]) rebuild = true;
    }

    if (rebuild) {
        while (container.firstChild) container.removeChild(container.firstChild);
        keys.forEach(function (key) { container.appendChild(build(key)); });
    }
    keys.forEach(function (key, index) { update(container.children[index], key); });
}

function cardHead(iconSrc, title, subtitle) {
    var head = makeEl("div", "card-head");
    var img = makeEl("img");
    img.src = iconSrc;
    img.alt = title + " Icon";
    var text = makeEl("div", "card-text");
    text.appendChild(makeEl("div", "card-title", title));
    text.appendChild(makeEl("div", "card-sub", subtitle));
    head.appendChild(img);
    head.appendChild(text);
    // The health pill is always present so a verdict can appear, change or be
    // dropped without rebuilding the card — a rebuild would restart the bar
    // animations, and a drive's health is exactly the thing that changes.
    head.appendChild(statusPill(null));
    return head;
}

// A device's health verdict as the collector reported it: a level the palette
// already styles (good, warn, bad, unknown), a short text and the reason. The
// reason is the tooltip, because a warning a user cannot account for is worse
// than no warning at all.
function statusPill(status) {
    var pill = makeEl("span", "status-pill");
    setStatusPill(pill, status);
    return pill;
}

function setStatusPill(pill, status) {
    var level = status && status.Level;
    if (!level) {
        pill.hidden = true;
        pill.className = "status-pill";
        pill.textContent = "";
        pill.removeAttribute("title");
        return;
    }
    pill.hidden = false;
    pill.className = "status-pill " + level;
    pill.textContent = status.Text || "";
    if (status.Detail) {
        pill.title = status.Detail;
    } else {
        pill.removeAttribute("title");
    }
}

function updateStatus(card, status) {
    var head = card.querySelector(".card-head");
    if (!head) return;
    var pill = head.querySelector(".status-pill");
    if (!pill) {
        head.appendChild(statusPill(status));
        return;
    }
    setStatusPill(pill, status);
}

// The card shell never changes between ticks: rows are reconciled into
// its tbody and core grids appended to it.
function buildCardShell(iconSrc, title, subtitle) {
    var section = makeEl("section", "card");
    section.appendChild(cardHead(iconSrc, title, subtitle));

    var table = makeEl("table");
    var thead = makeEl("thead");
    thead.innerHTML = "<tr><th>Metric</th><th>Current</th><th>Min</th><th>Max</th></tr>";
    table.appendChild(thead);
    table.appendChild(makeEl("tbody"));
    section.appendChild(table);
    return section;
}

function buildMetricRow(key) {
    var tr = makeEl("tr");
    tr.setAttribute("data-key", key);
    tr.appendChild(makeEl("td", "label-cell"));

    // The value cell keeps its text in a child node so the bar can be
    // updated without being recreated (which would kill its transition).
    var tdValue = makeEl("td", "value-cell");
    tdValue.appendChild(makeEl("span", "value-text"));
    tr.appendChild(tdValue);

    tr.appendChild(makeEl("td", "minmax"));
    tr.appendChild(makeEl("td", "minmax"));
    return tr;
}

// bars are created/dropped as a metric gains or loses a usable scale,
// but when the bar already exists only its width and class are written.
function setBar(container, scale) {
    var wrap = container.querySelector(".bar-wrap");
    if (!scale) {
        if (wrap) container.removeChild(wrap);
        return;
    }
    if (!wrap) {
        wrap = makeEl("div", "bar-wrap");
        wrap.appendChild(makeEl("div"));
        container.appendChild(wrap);
    }
    var bar = wrap.firstChild;
    bar.className = "bar " + scale.cls;
    bar.style.width = scale.pct + "%";
}

function updateMetricRow(tr, key, labels, values, minValues, maxValues) {
    var label = labels[key];
    if (label !== undefined) tr.children[0].textContent = label;

    tr.children[1].firstChild.textContent =
        values[key] !== undefined ? values[key] : "";
    tr.children[2].textContent =
        minValues && minValues[key] !== undefined ? minValues[key] : "";
    tr.children[3].textContent =
        maxValues && maxValues[key] !== undefined ? maxValues[key] : "";

    setBar(tr.children[1], gauge(key, values[key], values));
}

function updateMetrics(tbody, metricKeys, labels, values, minValues, maxValues) {
    reconcile(
        tbody,
        metricKeys.map(keyOf),
        buildMetricRow,
        function (tr, key) { updateMetricRow(tr, key, labels, values, minValues, maxValues); }
    );
}

// ---- Per-core clocks & temperatures --------------------------------
// Cores are rendered as compact grids rather than one table row each: a
// 32-thread CPU would otherwise add 32 rows to the card.
function isClockKey(key) {
    return /^cpu\.clock\d+$/.test(key);
}

function isCoreTempKey(key) {
    return /^cpu\.coreTemp\d+$/.test(key);
}

function trailingIndex(key) {
    var match = key.match(/(\d+)$/);
    return match ? parseInt(match[1], 10) : null;
}

// Severity class for a temperature, reusing the table's gauge thresholds.
function tempSeverity(value) {
    var g = frac(value, 100);
    return g ? g.cls : "good";
}

// Header plus an empty grid container for one compact core block.
function coreSection(titleText, noteText) {
    var section = document.createElement("div");
    section.className = "cores";

    var head = document.createElement("div");
    head.className = "cores-head";

    var title = document.createElement("span");
    title.className = "cores-title";
    title.textContent = titleText;

    var note = document.createElement("span");
    note.className = "cores-note";
    note.textContent = noteText;

    head.appendChild(title);
    head.appendChild(note);

    var grid = document.createElement("div");
    grid.className = "core-grid";

    section.appendChild(head);
    section.appendChild(grid);
    return { section: section, grid: grid };
}

// min – max line shared by every core cell.
function coreRange(minValues, maxValues, key) {
    var lo = num(minValues && minValues[key]);
    var hi = num(maxValues && maxValues[key]);
    var range = document.createElement("div");
    range.className = "core-range";
    if (lo !== null || hi !== null) {
        range.textContent = (lo === null ? "—" : String(Math.round(lo))) +
            " – " + (hi === null ? "—" : String(Math.round(hi)));
    }
    return range;
}

// Highest clock any core has reached, used as the shared bar scale so the
// bars show boost headroom instead of pinning one core at 100%.
function clockPeak(values, maxValues) {
    var peak = 0;
    Object.keys(values).forEach(function (key) {
        if (!isClockKey(key)) return;
        var seen = num(maxValues && maxValues[key]);
        var current = num(values[key]);
        var candidate = seen !== null ? seen : (current !== null ? current : 0);
        if (candidate > peak) peak = candidate;
    });
    return peak;
}

// Decides how the core sensors relate to the threads, which is what
// determines whether a temperature can sit inside a clock tile:
//   "paired" - threads map to cores via /proc/cpuinfo topology
//   "index"  - no topology, but exactly one sensor per thread
//   "block"  - unsafe to pair, so the sensors get their own block
function tempLayout(values, clockKeys, coreTempKeys) {
    if (coreTempKeys.length === 0) return "none";
    if (clockKeys.length === 0) return "block";
    var hasTopology = clockKeys.every(function (k) {
        var index = trailingIndex(k.Key);
        return index !== null && values["cpu.coreOf" + index] !== undefined;
    });
    if (hasTopology) return "paired";
    return coreTempKeys.length === clockKeys.length ? "index" : "block";
}

// Temperature of the physical core a given thread runs on, or null when
// it cannot be determined without guessing.
function tempForClock(values, layout, key) {
    if (layout === "paired") {
        var index = trailingIndex(key);
        if (index === null) return null;
        var coreID = values["cpu.coreOf" + index];
        if (coreID === undefined || coreID === null) return null;
        var byCore = values["cpu.coreTemp" + String(coreID).trim()];
        return byCore === undefined ? null : num(byCore);
    }
    if (layout === "index") {
        var paired = values["cpu.coreTemp" + trailingIndex(key)];
        return paired === undefined ? null : num(paired);
    }
    return null;
}

function buildCoreGrid(clockKeys, coreTempKeys, values, minValues, maxValues, layout) {
    var sections = [];
    var peak = clockPeak(values, maxValues);
    var pairTemps = layout === "paired" || layout === "index";

    if (clockKeys.length > 0) {
        var threads = clockKeys.length + (clockKeys.length === 1 ? " thread" : " threads");
        var clocks = coreSection(
            "Per-core clocks",
            peak > 0 ? threads + " · peak " + Math.round(peak) + " MHz" : threads
        );
        clocks.section.setAttribute("data-key", "cores:clocks");

        clockKeys.forEach(function (k) {
            var current = num(values[k.Key]);

            var cell = document.createElement("div");
            cell.className = "core";
            cell.setAttribute("data-key", k.Key);

            var head = document.createElement("div");
            head.className = "core-head";

            var name = document.createElement("span");
            name.className = "core-name";
            name.textContent = "Core " + k.Key.replace(/^cpu\.clock/, "");
            head.appendChild(name);

            if (pairTemps) {
                var temp = tempForClock(values, layout, k.Key);
                if (temp !== null && temp !== undefined) {
                    var tempEl = document.createElement("span");
                    tempEl.className = "core-temp " + tempSeverity(temp);
                    tempEl.textContent = Math.round(temp) + "°C";
                    head.appendChild(tempEl);
                }
            }

            var value = document.createElement("div");
            value.className = "core-value";
            value.textContent = current === null ? "—" : String(Math.round(current));
            if (current !== null) {
                var unit = document.createElement("span");
                unit.className = "core-unit";
                unit.textContent = "MHz";
                value.appendChild(unit);
            }

            var barWrap = document.createElement("div");
            barWrap.className = "bar-wrap";
            var bar = document.createElement("div");
            bar.className = "bar clock";
            bar.style.width = (peak > 0 && current !== null)
                ? Math.round(Math.max(0, Math.min(1, current / peak)) * 100) + "%"
                : "0%";
            barWrap.appendChild(bar);

            cell.appendChild(head);
            cell.appendChild(value);
            cell.appendChild(barWrap);
            cell.appendChild(coreRange(minValues, maxValues, k.Key));
            clocks.grid.appendChild(cell);
        });

        sections.push(clocks.section);
    }

    // Core sensors that were not folded into a clock cell get their own
    // block, so they stay visible and are never mislabeled.
    if (layout === "block") {
        var sensors = coreTempKeys.length === 1 ? " sensor" : " sensors";
        var temps = coreSection("Per-core temperatures", coreTempKeys.length + sensors);
        temps.section.setAttribute("data-key", "cores:temps");

        coreTempKeys.forEach(function (k) {
            var current = num(values[k.Key]);

            var cell = document.createElement("div");
            cell.className = "core";
            cell.setAttribute("data-key", k.Key);

            var head = document.createElement("div");
            head.className = "core-head";

            var name = document.createElement("span");
            name.className = "core-name";
            name.textContent = "Core " + trailingIndex(k.Key);
            head.appendChild(name);

            var value = document.createElement("div");
            value.className = "core-value";
            value.textContent = current === null ? "—" : String(Math.round(current));
            if (current !== null) {
                var unit = document.createElement("span");
                unit.className = "core-unit";
                unit.textContent = "°C";
                value.appendChild(unit);
            }

            var barWrap = document.createElement("div");
            barWrap.className = "bar-wrap";
            var bar = document.createElement("div");
            bar.className = "bar " + tempSeverity(current === null ? 0 : current);
            var g = current === null ? null : frac(current, 100);
            bar.style.width = (g ? g.pct : 0) + "%";
            barWrap.appendChild(bar);

            cell.appendChild(head);
            cell.appendChild(value);
            cell.appendChild(barWrap);
            cell.appendChild(coreRange(minValues, maxValues, k.Key));
            temps.grid.appendChild(cell);
        });

        sections.push(temps.section);
    }

    return sections;
}

// ---- Refreshing an existing view -----------------------------------
// Writes a value and its unit into a cell's value line, adding or dropping
// the unit node as the reading appears and disappears.
function setValueLine(el, text, unit) {
    var first = el.firstChild;
    if (first && first.nodeType === 3) {
        first.nodeValue = text;
    } else {
        el.insertBefore(document.createTextNode(text), first);
    }

    var unitEl = el.querySelector(".core-unit");
    if (unit) {
        if (unitEl) {
            unitEl.textContent = unit;
        } else {
            el.appendChild(makeEl("span", "core-unit", unit));
        }
    } else if (unitEl) {
        el.removeChild(unitEl);
    }
}

function setRange(el, minValues, maxValues, key) {
    var lo = num(minValues && minValues[key]);
    var hi = num(maxValues && maxValues[key]);
    if (lo === null && hi === null) {
        el.textContent = "";
        return;
    }
    el.textContent = (lo === null ? "—" : String(Math.round(lo))) +
        " – " + (hi === null ? "—" : String(Math.round(hi)));
}

// Cells are looked up by their data-key, so a tick rewrites text and bar
// widths on the nodes already on screen instead of replacing them.
function cellsByKey(container) {
    var map = {};
    Array.prototype.forEach.call(container.children, function (el) {
        map[el.getAttribute("data-key")] = el;
    });
    return map;
}

function updateClocks(card, clockKeys, values, minValues, maxValues, layout) {
    var section = card.querySelector('[data-key="cores:clocks"]');
    if (!section) return;

    var peak = clockPeak(values, maxValues);

    var note = section.querySelector(".cores-note");
    if (note) {
        var threads = clockKeys.length + (clockKeys.length === 1 ? " thread" : " threads");
        note.textContent = peak > 0
            ? threads + " · peak " + Math.round(peak) + " MHz"
            : threads;
    }

    var cells = cellsByKey(section.querySelector(".core-grid"));
    clockKeys.forEach(function (k) {
        var cell = cells[k.Key];
        if (!cell) return;
        var current = num(values[k.Key]);

        var head = cell.querySelector(".core-head");
        var temp = tempForClock(values, layout, k.Key);
        var tempEl = head.querySelector(".core-temp");
        if (temp === null || temp === undefined) {
            if (tempEl) head.removeChild(tempEl);
        } else {
            if (!tempEl) tempEl = head.appendChild(makeEl("span"));
            tempEl.className = "core-temp " + tempSeverity(temp);
            tempEl.textContent = Math.round(temp) + "°C";
        }

        setValueLine(
            cell.querySelector(".core-value"),
            current === null ? "—" : String(Math.round(current)),
            "MHz"
        );

        cell.querySelector(".bar").style.width = (peak > 0 && current !== null)
            ? Math.round(Math.max(0, Math.min(1, current / peak)) * 100) + "%"
            : "0%";

        setRange(cell.querySelector(".core-range"), minValues, maxValues, k.Key);
    });
}

function updateTemps(card, coreTempKeys, values, minValues, maxValues) {
    var section = card.querySelector('[data-key="cores:temps"]');
    if (!section) return;

    var cells = cellsByKey(section.querySelector(".core-grid"));
    coreTempKeys.forEach(function (k) {
        var cell = cells[k.Key];
        if (!cell) return;
        var current = num(values[k.Key]);

        setValueLine(
            cell.querySelector(".core-value"),
            current === null ? "—" : String(Math.round(current)),
            "°C"
        );

        var scale = current === null ? null : frac(current, temperatureLimit(k.Key));
        var bar = cell.querySelector(".bar");
        bar.className = "bar " + (scale ? scale.cls : "good");
        bar.style.width = (scale ? scale.pct : 0) + "%";

        setRange(cell.querySelector(".core-range"), minValues, maxValues, k.Key);
    });
}

// ---- Rendering -----------------------------------------------------
// Each tick describes the cards that should be on screen. A card is only
// rebuilt when its shape changes (different metrics, or temperatures
// moving between the tiles and their own block); otherwise its rows are
// updated in place so the bar transitions keep animating.
function render(data) {
    var grid = document.getElementById("cards");

    var cpuValues = data.CpuValues || {};
    var cpuMinValues = data.CpuMinValues || {};
    var cpuMaxValues = data.CpuMaxValues || {};
    var cpuKeys = data.CpuKeys || [];

    var clockKeys = cpuKeys.filter(function (k) { return isClockKey(k.Key); });
    var coreTempKeys = cpuKeys.filter(function (k) { return isCoreTempKey(k.Key); });
    var metricKeys = cpuKeys.filter(function (k) {
        return !isClockKey(k.Key) && !isCoreTempKey(k.Key);
    });

    var cpuBrand = cpuValues["cpu.brand"] ? cpuValues["cpu.brand"] : "CPU";
    var cpuSubtitle = cpuBrand + " · " + cpuKeys.length + " metrics";
    var cpuLabels = labelsOf(metricKeys);
    var layout = tempLayout(cpuValues, clockKeys, coreTempKeys);

    var specs = [{
        key: "cpu",
        signature: [
            cpuSubtitle,
            metricKeys.map(keyOf).join(","),
            clockKeys.map(keyOf).join(","),
            coreTempKeys.map(keyOf).join(","),
            layout
        ].join("|"),
        create: function () {
            var card = buildCardShell("/resources/cpu.png", "CPU", cpuSubtitle);
            updateMetrics(card.querySelector("tbody"), metricKeys, cpuLabels,
                cpuValues, cpuMinValues, cpuMaxValues);
            buildCoreGrid(clockKeys, coreTempKeys, cpuValues, cpuMinValues,
                cpuMaxValues, layout).forEach(function (section) {
                card.appendChild(section);
            });
            return card;
        },
        update: function (card) {
            updateMetrics(card.querySelector("tbody"), metricKeys, cpuLabels,
                cpuValues, cpuMinValues, cpuMaxValues);
            updateClocks(card, clockKeys, cpuValues, cpuMinValues, cpuMaxValues, layout);
            updateTemps(card, coreTempKeys, cpuValues, cpuMinValues, cpuMaxValues);
        }
    }];

    // The memory card is a single device like the mainboard, so it shares the
    // metric renderer and keeps its own history under no prefix.
    var memory = data.Memory;
    if (memory && memory.Keys && memory.Keys.length) {
        var memKeys = memory.Keys;
        var memValues = memory.Values || {};
        var memMinValues = memory.MinValues || {};
        var memMaxValues = memory.MaxValues || {};
        var memLabels = labelsOf(memKeys);
        var memTitle = memory.Name || "Memory";
        var memSubtitle = "System memory · " + memKeys.length + " metrics";
        specs.push({
            key: "memory",
            signature: [memTitle, memSubtitle, memKeys.map(keyOf).join(",")].join("|"),
            create: function () {
                var card = buildCardShell("/resources/mem.svg", memTitle, memSubtitle);
                updateMetrics(card.querySelector("tbody"), memKeys, memLabels,
                    memValues, memMinValues, memMaxValues);
                return card;
            },
            update: function (card) {
                updateMetrics(card.querySelector("tbody"), memKeys, memLabels,
                    memValues, memMinValues, memMaxValues);
            }
        });
    }

    // The mainboard card is only present when the board reports something
    // (some systems have no SMBIOS and no board sensors). Its rows are one
    // table like a GPU's, so it shares the metric renderer.
    var board = data.Board;
    if (board && board.Keys && board.Keys.length) {
        var boardKeys = board.Keys;
        var boardValues = board.Values || {};
        var boardMinValues = board.MinValues || {};
        var boardMaxValues = board.MaxValues || {};
        var boardLabels = labelsOf(boardKeys);
        var boardTitle = board.Name || "Mainboard";
        var boardSubtitle = "Mainboard · " + boardKeys.length + " metrics";
        specs.push({
            key: "board",
            signature: [boardTitle, boardSubtitle, boardKeys.map(keyOf).join(",")].join("|"),
            create: function () {
                var card = buildCardShell("/resources/board.png", boardTitle, boardSubtitle);
                updateMetrics(card.querySelector("tbody"), boardKeys, boardLabels,
                    boardValues, boardMinValues, boardMaxValues);
                return card;
            },
            update: function (card) {
                updateMetrics(card.querySelector("tbody"), boardKeys, boardLabels,
                    boardValues, boardMinValues, boardMaxValues);
            }
        });
    }

    (data.GpuViews || []).forEach(function (gpu, index) {
        var values = gpu.Values || {};
        var minValues = gpu.MinValues || {};
        var maxValues = gpu.MaxValues || {};
        var keys = gpu.Keys || [];
        var labels = labelsOf(keys);
        var title = gpu.Name;
        var subtitle = "GPU · " + keys.length + " metrics";
        specs.push({
            key: "gpu" + index,
            signature: [title, subtitle, keys.map(keyOf).join(",")].join("|"),
            create: function () {
                var card = buildCardShell("/resources/gpu.png", title, subtitle);
                updateMetrics(card.querySelector("tbody"), keys, labels,
                    values, minValues, maxValues);
                return card;
            },
            update: function (card) {
                updateMetrics(card.querySelector("tbody"), keys, labels,
                    values, minValues, maxValues);
            }
        });
    });

    // Drives are one card each, like GPUs. The medium (SSD, SSD (NVMe),
    // HDD) is collected as a row, so the card head can say what kind of
    // drive this is instead of repeating the model name.
    (data.DiskViews || []).forEach(function (disk, index) {
        var values = disk.Values || {};
        var minValues = disk.MinValues || {};
        var maxValues = disk.MaxValues || {};
        var keys = disk.Keys || [];
        var labels = labelsOf(keys);
        var title = disk.Name;
        var medium = values["disk.medium"] || "Storage";
        var subtitle = medium + " · " + keys.length + " metrics";
        // The verdict is deliberately not part of the signature: it is written
        // onto the pill in place, so a drive going from Good to Warning updates
        // its badge without the whole card being rebuilt.
        var status = disk.Status || null;
        specs.push({
            key: "disk" + index,
            signature: [title, subtitle, keys.map(keyOf).join(",")].join("|"),
            create: function () {
                var card = buildCardShell("/resources/disk.png", title, subtitle);
                updateMetrics(card.querySelector("tbody"), keys, labels,
                    values, minValues, maxValues);
                updateStatus(card, status);
                return card;
            },
            update: function (card) {
                updateMetrics(card.querySelector("tbody"), keys, labels,
                    values, minValues, maxValues);
                updateStatus(card, status);
            }
        });
    });

    var children = grid.children;
    var rebuild = children.length !== specs.length;
    for (var i = 0; !rebuild && i < specs.length; i++) {
        if (children[i].getAttribute("data-key") !== specs[i].key) rebuild = true;
    }

    if (rebuild) {
        while (grid.firstChild) grid.removeChild(grid.firstChild);
        specs.forEach(function (spec) {
            var card = spec.create();
            card.setAttribute("data-key", spec.key);
            card.setAttribute("data-signature", spec.signature);
            grid.appendChild(card);
        });
        return;
    }

    specs.forEach(function (spec, index) {
        var card = children[index];
        if (card.getAttribute("data-signature") === spec.signature) {
            spec.update(card);
            return;
        }
        var replacement = spec.create();
        replacement.setAttribute("data-key", spec.key);
        replacement.setAttribute("data-signature", spec.signature);
        grid.replaceChild(replacement, card);
    });
}

// ---- Polling loop --------------------------------------------------
// Wails v2 exposes bound methods under window.go.<package>.<StructName>.<Method>()
const POLL_MS = 5000;

function getData() {
    if (window.go && window.go.main && window.go.main.App) {
        return window.go.main.App.GetData();
    }
    // Fallback for older/alternate binding layouts.
    if (window.goApp && window.goApp.GetData) {
        return window.goApp.GetData();
    }
    throw new Error("Wails bindings not ready yet");
}

let timer = null;

function tick() {
    try {
        getData().then(function (data) {
            render(data);
        }).catch(function (err) {
            console.error("GetData failed:", err);
        });
    } catch (e) {
        // Bindings may not be ready on the very first tick; retry shortly.
        console.warn(e.message || e);
        setTimeout(tick, 500);
    }
}

function startPolling() {
    if (timer !== null) return;
    tick();
    timer = setInterval(tick, POLL_MS);
}

function stopPolling() {
    if (timer === null) return;
    clearInterval(timer);
    timer = null;
}

function updatePauseBadge(paused) {
    var badge = document.querySelector(".refresh-note");
    if (!badge) return;
    if (paused) {
        badge.classList.add("paused");
        badge.title = "Paused — tab is hidden";
        var text = badge.childNodes[badge.childNodes.length - 1];
        if (text && text.nodeType === 3) text.nodeValue = " Paused";
    } else {
        badge.classList.remove("paused");
        badge.title = "";
        var text2 = badge.childNodes[badge.childNodes.length - 1];
        if (text2 && text2.nodeType === 3) text2.nodeValue = " Auto-refresh \u00b7 5s";
    }
}

document.addEventListener("visibilitychange", function () {
    if (document.hidden) {
        stopPolling();
        updatePauseBadge(true);
    } else {
        updatePauseBadge(false);
        startPolling();
    }
});

startPolling();
