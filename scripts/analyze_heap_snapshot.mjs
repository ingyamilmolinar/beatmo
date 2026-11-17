#!/usr/bin/env node
// Quick-and-dirty Chromium heap snapshot analyzer. Reads a .heapsnapshot
// JSON dump, aggregates self_size by (type, name), and prints the top-N
// constructors by retained bytes. Optionally diffs two snapshots to
// surface what grew.
//
// Usage:
//   node analyze_heap_snapshot.mjs <snapshot.heapsnapshot> [topN]
//   node analyze_heap_snapshot.mjs --diff <warmup.heapsnapshot> <end.heapsnapshot>
//
// Output is the dominant retainers. For a Go-WASM workload, "system /
// Map" and "wasmtable" entries dominate; what we care about is whether
// any user-named constructor grows monotonically (leak indicator).

import fs from "node:fs";

function readSnap(path) {
    const raw = JSON.parse(fs.readFileSync(path, "utf8"));
    const meta = raw.snapshot.meta;
    const nodeFields = meta.node_fields;
    const nodeTypes = meta.node_types;
    const fieldStride = nodeFields.length;
    const typeIdx = nodeFields.indexOf("type");
    const nameIdx = nodeFields.indexOf("name");
    const sizeIdx = nodeFields.indexOf("self_size");
    const nodes = raw.nodes;
    const strings = raw.strings;
    const types = nodeTypes[typeIdx];
    // Aggregate by (typeName, name) -> { count, bytes }
    const agg = new Map();
    const numNodes = nodes.length / fieldStride;
    for (let i = 0; i < numNodes; i++) {
        const t = types[nodes[i * fieldStride + typeIdx]];
        const n = strings[nodes[i * fieldStride + nameIdx]] ?? "";
        const sz = nodes[i * fieldStride + sizeIdx];
        const key = `${t}::${n}`;
        const prev = agg.get(key);
        if (prev) {
            prev.count++;
            prev.bytes += sz;
        } else {
            agg.set(key, { type: t, name: n, count: 1, bytes: sz });
        }
    }
    return { agg, totalNodes: numNodes };
}

function topN(agg, n = 30) {
    return [...agg.values()].sort((a, b) => b.bytes - a.bytes).slice(0, n);
}

function fmtBytes(b) {
    if (b > 1024 * 1024) return (b / (1024 * 1024)).toFixed(2) + " MB";
    if (b > 1024) return (b / 1024).toFixed(2) + " KB";
    return b + " B";
}

const args = process.argv.slice(2);
if (args[0] === "--diff" && args.length >= 3) {
    const a = readSnap(args[1]);
    const b = readSnap(args[2]);
    const diff = new Map();
    for (const [key, val] of b.agg) {
        const prev = a.agg.get(key);
        const prevBytes = prev?.bytes ?? 0;
        const prevCount = prev?.count ?? 0;
        diff.set(key, {
            type: val.type,
            name: val.name,
            count: val.count - prevCount,
            bytes: val.bytes - prevBytes,
        });
    }
    const top = [...diff.values()].sort((a, b) => b.bytes - a.bytes).slice(0, 30);
    console.log(`Diff: ${args[1]} → ${args[2]}`);
    console.log(`Total nodes: ${a.totalNodes} → ${b.totalNodes} (Δ ${b.totalNodes - a.totalNodes})`);
    console.log();
    console.log("Top 30 growers (constructor, count Δ, bytes Δ):");
    console.log(
        "%-40s %15s %15s",
        "constructor",
        "Δ count",
        "Δ bytes"
    );
    for (const e of top) {
        const ctor = `${e.type}::${e.name || "<empty>"}`.slice(0, 40);
        console.log(
            "%-40s %15d %15s",
            ctor,
            e.count,
            fmtBytes(e.bytes)
        );
    }
} else if (args.length >= 1) {
    const a = readSnap(args[0]);
    const n = args[1] ? Number(args[1]) : 30;
    console.log(`Snapshot: ${args[0]}`);
    console.log(`Total nodes: ${a.totalNodes}`);
    console.log();
    console.log(`Top ${n} constructors by retained bytes:`);
    console.log(
        "%-40s %15s %15s",
        "constructor",
        "count",
        "bytes"
    );
    for (const e of topN(a.agg, n)) {
        const ctor = `${e.type}::${e.name || "<empty>"}`.slice(0, 40);
        console.log(
            "%-40s %15d %15s",
            ctor,
            e.count,
            fmtBytes(e.bytes)
        );
    }
} else {
    console.error("Usage: analyze_heap_snapshot.mjs <snapshot> [topN]");
    console.error("       analyze_heap_snapshot.mjs --diff <a> <b>");
    process.exit(2);
}
