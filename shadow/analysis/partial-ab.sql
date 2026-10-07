-- A/B of classic gossip (A) against partial-message bundles (B).
-- {A}, {B}: parquet dirs from logs-to-parquet.py. {FIRST}..{LAST}: slot window.
-- {POOL}: seats per slot. Every section names the pass criterion it serves.
.mode box
.maxrows 400

.print == 1. P2: FFG summary per node and slot, A against B
SELECT a.node, a.slot, a.votes AS votes_a, b.votes AS votes_b,
       a.seats AS seats_a, b.seats AS seats_b,
       round(a.seats / {POOL}, 3) AS frac_a, round(b.seats / {POOL}, 3) AS frac_b
FROM '{A}/ffg_summary.parquet' a JOIN '{B}/ffg_summary.parquet' b USING (node, slot)
WHERE slot BETWEEN {FIRST} AND {LAST} ORDER BY a.node_index, a.slot;

.print == 1b. P2: mismatches (zero rows = pass)
SELECT a.node, a.slot, a.votes AS votes_a, b.votes AS votes_b, a.seats AS seats_a, b.seats AS seats_b
FROM '{A}/ffg_summary.parquet' a JOIN '{B}/ffg_summary.parquet' b USING (node, slot)
WHERE slot BETWEEN {FIRST} AND {LAST} AND (a.votes <> b.votes OR a.seats <> b.seats)
ORDER BY a.node_index, a.slot;

.print == 2a. P3: votes seen in A and missing in B (zero rows = pass)
SELECT a.node, a.att_slot, a.validator, a.arrived_ms AS arrived_a
FROM '{A}/ffg_votes.parquet' a
LEFT JOIN '{B}/ffg_votes.parquet' b
  ON a.node = b.node AND a.att_slot = b.att_slot AND a.validator = b.validator
     AND b.outcome = 'gossip'
WHERE a.outcome = 'gossip' AND a.att_slot BETWEEN {FIRST} AND {LAST} AND b.validator IS NULL
ORDER BY a.node_index, a.att_slot, a.validator;

.print == 2b. P3: transport mix per arm (A all gossip, B all bundle = pass)
SELECT 'A' AS arm, transport, count(*) AS votes FROM '{A}/ffg_votes.parquet'
WHERE outcome = 'gossip' AND att_slot BETWEEN {FIRST} AND {LAST} GROUP BY 1, 2
UNION ALL
SELECT 'B', transport, count(*) FROM '{B}/ffg_votes.parquet'
WHERE outcome = 'gossip' AND att_slot BETWEEN {FIRST} AND {LAST} GROUP BY 1, 2
ORDER BY 1, 2;

.print == 3. P7: FFG vote arrival, ms into the slot, per arm
WITH v AS (
    SELECT 'A' AS arm, * FROM '{A}/ffg_votes.parquet'
    UNION ALL SELECT 'B', * FROM '{B}/ffg_votes.parquet')
SELECT arm, transport, count(*) AS n,
       round(quantile_cont(arrived_ms, 0.5), 1) AS p50,
       round(quantile_cont(arrived_ms, 0.9), 1) AS p90,
       round(quantile_cont(arrived_ms, 0.99), 1) AS p99, max(arrived_ms) AS max
FROM v WHERE outcome = 'gossip' AND att_slot BETWEEN {FIRST} AND {LAST}
GROUP BY 1, 2 ORDER BY 1, 2;

.print == 3b. P7: arrival histogram, 20 ms buckets, first 20
WITH v AS (
    SELECT 'A' AS arm, * FROM '{A}/ffg_votes.parquet'
    UNION ALL SELECT 'B', * FROM '{B}/ffg_votes.parquet')
SELECT (arrived_ms // 20) * 20 AS bucket_ms,
       count(*) FILTER (WHERE arm = 'A') AS a, count(*) FILTER (WHERE arm = 'B') AS b
FROM v WHERE outcome = 'gossip' AND att_slot BETWEEN {FIRST} AND {LAST}
GROUP BY 1 ORDER BY 1 LIMIT 20;

.print == 4. P4: bundles in B per node, slot and path
SELECT node, slot, path, sum(bundles) AS bundles, sum(signatures) AS signatures,
       round(sum(signatures) / sum(bundles), 2) AS sigs_per_bundle, sum(new) AS new
FROM '{B}/att_bundles.parquet' WHERE slot BETWEEN {FIRST} AND {LAST}
GROUP BY node_index, node, slot, path ORDER BY node_index, slot, path;

.print == 4b. P4: nodes and slots where recv bundles are not fewer than signatures (zero rows = pass)
SELECT node, slot, sum(bundles) AS bundles, sum(signatures) AS signatures
FROM '{B}/att_bundles.parquet' WHERE path = 'recv' AND slot BETWEEN {FIRST} AND {LAST}
GROUP BY node_index, node, slot HAVING sum(signatures) >= 2 AND sum(bundles) >= sum(signatures)
ORDER BY node_index, slot;

.print == 4c. P4: nodes and slots in B with no recv bundle (zero rows = pass)
SELECT s.node, s.slot FROM '{B}/ffg_summary.parquet' s
LEFT JOIN (SELECT node, slot FROM '{B}/att_bundles.parquet' WHERE path = 'recv' GROUP BY 1, 2) r
  USING (node, slot)
WHERE s.slot BETWEEN {FIRST} AND {LAST} AND r.node IS NULL ORDER BY s.node_index, s.slot;

.print == 5a. P6: bytes per node over the run, attestation family, A against B
WITH d AS (
    SELECT 'A' AS arm, node, node_index,
           arg_max(att_publish_in + att_publish_out + att_bundle_in + att_bundle_out
                   + att_meta_in + att_meta_out
                   + coalesce(att_ihave_in + att_ihave_out, 0), ts)
         - arg_min(att_publish_in + att_publish_out + att_bundle_in + att_bundle_out
                   + att_meta_in + att_meta_out
                   + coalesce(att_ihave_in + att_ihave_out, 0), ts) AS att_total,
           arg_max(meshsub_in + meshsub_out, ts) - arg_min(meshsub_in + meshsub_out, ts) AS meshsub,
           arg_max(total_in + total_out, ts) - arg_min(total_in + total_out, ts) AS total,
           coalesce(arg_max(iwant_in + iwant_out + idontwant_in + idontwant_out, ts)
                    - arg_min(iwant_in + iwant_out + idontwant_in + idontwant_out, ts), 0) AS want_ctl
    FROM '{A}/p2p_bandwidth.parquet' GROUP BY 1, 2, 3
    UNION ALL
    SELECT 'B', node, node_index,
           arg_max(att_publish_in + att_publish_out + att_bundle_in + att_bundle_out
                   + att_meta_in + att_meta_out
                   + coalesce(att_ihave_in + att_ihave_out, 0), ts)
         - arg_min(att_publish_in + att_publish_out + att_bundle_in + att_bundle_out
                   + att_meta_in + att_meta_out
                   + coalesce(att_ihave_in + att_ihave_out, 0), ts),
           arg_max(meshsub_in + meshsub_out, ts) - arg_min(meshsub_in + meshsub_out, ts),
           arg_max(total_in + total_out, ts) - arg_min(total_in + total_out, ts),
           coalesce(arg_max(iwant_in + iwant_out + idontwant_in + idontwant_out, ts)
                    - arg_min(iwant_in + iwant_out + idontwant_in + idontwant_out, ts), 0)
    FROM '{B}/p2p_bandwidth.parquet' GROUP BY 1, 2, 3)
SELECT a.node, a.att_total AS att_total_a, b.att_total AS att_total_b,
       round(b.att_total / a.att_total, 3) AS att_ratio,
       a.meshsub AS meshsub_a, b.meshsub AS meshsub_b, a.total AS total_a, b.total AS total_b,
       a.want_ctl AS want_ctl_a, b.want_ctl AS want_ctl_b,
       b.att_total < a.att_total AS pass
FROM d a JOIN d b ON a.node = b.node AND a.arm = 'A' AND b.arm = 'B'
ORDER BY a.node_index;

.print == 5b. P6: attestation-family bytes by kind, summed over nodes, per arm
WITH d AS (
    SELECT 'A' AS arm, node,
           arg_max(att_publish_in, ts) - arg_min(att_publish_in, ts) AS publish_in,
           arg_max(att_publish_out, ts) - arg_min(att_publish_out, ts) AS publish_out,
           arg_max(att_bundle_in, ts) - arg_min(att_bundle_in, ts) AS bundle_in,
           arg_max(att_bundle_out, ts) - arg_min(att_bundle_out, ts) AS bundle_out,
           arg_max(att_meta_in, ts) - arg_min(att_meta_in, ts) AS meta_in,
           arg_max(att_meta_out, ts) - arg_min(att_meta_out, ts) AS meta_out,
           arg_max(att_ihave_in, ts) - arg_min(att_ihave_in, ts) AS ihave_in,
           arg_max(iwant_in, ts) - arg_min(iwant_in, ts) AS iwant_in,
           arg_max(idontwant_in, ts) - arg_min(idontwant_in, ts) AS idontwant_in
    FROM '{A}/p2p_bandwidth.parquet' GROUP BY 1, 2
    UNION ALL
    SELECT 'B', node,
           arg_max(att_publish_in, ts) - arg_min(att_publish_in, ts),
           arg_max(att_publish_out, ts) - arg_min(att_publish_out, ts),
           arg_max(att_bundle_in, ts) - arg_min(att_bundle_in, ts),
           arg_max(att_bundle_out, ts) - arg_min(att_bundle_out, ts),
           arg_max(att_meta_in, ts) - arg_min(att_meta_in, ts),
           arg_max(att_meta_out, ts) - arg_min(att_meta_out, ts),
           arg_max(att_ihave_in, ts) - arg_min(att_ihave_in, ts),
           arg_max(iwant_in, ts) - arg_min(iwant_in, ts),
           arg_max(idontwant_in, ts) - arg_min(idontwant_in, ts)
    FROM '{B}/p2p_bandwidth.parquet' GROUP BY 1, 2)
SELECT arm, sum(publish_in) AS publish_in, sum(publish_out) AS publish_out,
       sum(bundle_in) AS bundle_in, sum(bundle_out) AS bundle_out,
       sum(meta_in) AS meta_in, sum(meta_out) AS meta_out, sum(ihave_in) AS ihave_in,
       sum(iwant_in) AS iwant_in_all, sum(idontwant_in) AS idontwant_in_all
FROM d GROUP BY arm ORDER BY arm;

.print == 6. P8: block gossip arrival per arm, ms into the slot
WITH b AS (
    SELECT 'A' AS arm, * FROM '{A}/blocks_received.parquet'
    UNION ALL SELECT 'B', * FROM '{B}/blocks_received.parquet')
SELECT arm, count(*) AS n, round(quantile_cont(arrived_ms, 0.5), 1) AS p50,
       round(quantile_cont(arrived_ms, 0.9), 1) AS p90, max(arrived_ms) AS max
FROM b WHERE arrived_ms IS NOT NULL AND slot BETWEEN {FIRST} AND {LAST}
GROUP BY 1 ORDER BY 1;
