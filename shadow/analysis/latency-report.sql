-- One latency table covering every object type in a run.
--
--     sed 's#{DIR}#parquet/run29#g' latency-report.sql | duckdb
--
-- {DIR} is the parquet dir written by logs-to-parquet.py. Every figure is
-- milliseconds since the object's own slot start, so rows are comparable:
-- object, n, p50, p90, p99, max, the deadline that applies, and how many
-- landed past it.
--
-- Where the number comes from, per object:
--
--   blocks                    received_blocks.since_slot_start_ms, logged by the
--                             node on gossip arrival. No hard deadline; 3000 ms
--                             is reported because that is when the availability
--                             vote is due, so a block later than that starts
--                             costing votes.
--   payload envelopes         payload_ledger.arrived_ms, the envelope ledger
--                             line, one row per envelope per node whatever
--                             route delivered it. Reported against 3000 ms for
--                             the same reason as blocks.
--   data columns              data_columns.arrived_ms, one row per sidecar the
--                             node took in. Reported against 3000 ms like the
--                             other availability inputs.
--   availability attestations goldfish_votes.arrived_ms, outcome accepted or
--                             replayed, vote_slot >= 1. Deadline 12000 ms: fork
--                             choice counts a slot's votes at the next slot start.
--   PTC votes                 payload_attestations has no arrived_ms, so the
--                             delay is DERIVED from the log timestamp:
--                             ts - (genesis + 12s * slot), with genesis at
--                             simulated 00:05:00. This measures when the node
--                             logged the submission, not a gossip arrival, so
--                             it is a submission-time series and is not
--                             directly comparable with the gossip rows.
--                             Deadline 9000 ms (PAYLOAD_ATTESTATION_DUE_BPS
--                             7500 of a 12 s slot).
--   FFG attestations          ffg_votes.arrived_ms, outcome gossip.
--                             Deadline ffg_due_ms, AGGREGATE_DUE_BPS_GLOAS as
--                             milliseconds: the aggregator reads the pool then.
--   FFG aggregates            ffg_aggregates.arrived_ms, outcome gossip. No
--                             hard deadline; aggregates are published at
--                             ffg_due_ms, so arrival - ffg_due_ms is the propagation
--                             figure and is reported alongside the raw
--                             percentiles.
--
-- Genesis and slot length are the two knobs; change them here if a run differs.
SET VARIABLE genesis = TIMESTAMP '2000-01-01 00:05:00';
SET VARIABLE slot_ms = 12000;
-- AGGREGATE_DUE_BPS_GLOAS of the run, as milliseconds: FFG votes count at this point.
SET VARIABLE ffg_due_ms = 6000;

.mode box
.maxrows 100

CREATE OR REPLACE TABLE latency AS
WITH blocks AS (
    SELECT 'blocks (gossip arrival)' AS object, since_slot_start_ms AS ms,
           3000 AS deadline_ms, 'soft: availability vote due' AS deadline_kind
    FROM '{DIR}/received_blocks.parquet' WHERE since_slot_start_ms IS NOT NULL
), envelopes AS (
    -- The payload ledger line carries arrivedMs directly, on the same clock
    -- basis as the vote lines, so no derivation is needed. Empty on runs built
    -- before prysm 280777ae.
    SELECT 'payload envelopes' AS object, arrived_ms::DOUBLE AS ms,
           3000 AS deadline_ms, 'soft: availability vote due' AS deadline_kind
    FROM '{DIR}/payload_ledger.parquet' WHERE arrived_ms IS NOT NULL
), columns AS (
    -- Data column sidecars. gossip covers live gossip and the Gloas
    -- pending-queue drain (true arrival time); local is a column the node
    -- built or reconstructed itself. Empty before prysm 9d0ffd7c.
    SELECT 'data columns' AS object, arrived_ms::DOUBLE AS ms,
           3000 AS deadline_ms, 'soft: availability vote due' AS deadline_kind
    FROM '{DIR}/data_columns.parquet' WHERE arrived_ms IS NOT NULL
), avail AS (
    SELECT 'availability attestations' AS object, arrived_ms::DOUBLE AS ms,
           12000 AS deadline_ms, 'hard: next slot start' AS deadline_kind
    FROM '{DIR}/goldfish_votes.parquet'
    WHERE vote_slot >= 1 AND outcome IN ('accepted', 'replayed')
      AND arrived_ms IS NOT NULL
), ptc AS (
    SELECT 'PTC votes (derived, submit)' AS object,
           date_diff('millisecond',
                     getvariable('genesis') + to_milliseconds(
                         (getvariable('slot_ms') * slot)::BIGINT), ts)::DOUBLE AS ms,
           9000 AS deadline_ms, 'hard: payload attestation due' AS deadline_kind
    FROM '{DIR}/payload_attestations.parquet' WHERE slot >= 1 AND ts IS NOT NULL
), ffg AS (
    SELECT 'FFG attestations' AS object, arrived_ms::DOUBLE AS ms,
           getvariable('ffg_due_ms') AS deadline_ms, 'hard: aggregate due' AS deadline_kind
    FROM '{DIR}/ffg_votes.parquet'
    WHERE outcome = 'gossip' AND arrived_ms IS NOT NULL
), ffgagg AS (
    SELECT 'FFG aggregates' AS object, arrived_ms::DOUBLE AS ms,
           NULL AS deadline_ms, 'none (published at ' || getvariable('ffg_due_ms') || ')' AS deadline_kind
    FROM '{DIR}/ffg_aggregates.parquet'
    WHERE outcome = 'gossip' AND arrived_ms IS NOT NULL
)
SELECT * FROM blocks UNION ALL SELECT * FROM envelopes
UNION ALL SELECT * FROM columns UNION ALL SELECT * FROM avail UNION ALL SELECT * FROM ptc
UNION ALL SELECT * FROM ffg UNION ALL SELECT * FROM ffgagg;

.print
.print == THE LATENCY TABLE: ms since the object's own slot start
-- deadline_ms and deadline_kind are constant per object, so they group cleanly
-- and the FILTER can compare against the column directly.
SELECT object, count(*) AS n,
       round(quantile_cont(ms, 0.50), 1) AS p50,
       round(quantile_cont(ms, 0.90), 1) AS p90,
       round(quantile_cont(ms, 0.99), 1) AS p99,
       round(max(ms), 1) AS max,
       deadline_ms,
       count(*) FILTER (WHERE ms > deadline_ms) AS past_deadline,
       round(100.0 * count(*) FILTER (WHERE ms > deadline_ms) / count(*), 4)
           AS pct_past,
       deadline_kind
FROM latency GROUP BY object, deadline_ms, deadline_kind
ORDER BY CASE object WHEN 'blocks (gossip arrival)' THEN 1
                     WHEN 'payload envelopes' THEN 2
                     WHEN 'data columns' THEN 3
                     WHEN 'availability attestations' THEN 4
                     WHEN 'PTC votes (derived, submit)' THEN 5
                     WHEN 'FFG attestations' THEN 6 ELSE 7 END;

.print
.print == 2b. availability attestations past 3000 ms, by class
.print ==     a "replayed" vote is one seen twice -- parked, then landed. When its
.print ==     own arrival was timely the lateness is in the decision, not the wire.
SELECT class, count(*) AS rows,
       count(*) FILTER (WHERE arrived_ms > 3000) AS arrived_past_3000,
       count(*) FILTER (WHERE arrived_ms <= 3000) AS arrived_timely,
       round(quantile_cont(arrived_ms, 0.5), 1) AS p50_arrived,
       round(max(arrived_ms), 1) AS max_arrived,
       round(quantile_cont(decided_ms, 0.5), 1) AS p50_decided
FROM '{DIR}/goldfish_votes.parquet'
WHERE vote_slot >= 1 AND outcome IN ('accepted', 'replayed')
GROUP BY class ORDER BY class;

.print == 2c. the same split stated once: genuinely late on the wire vs late decision only
SELECT count(*) FILTER (WHERE outcome = 'replayed' AND arrived_ms <= 3000)
           AS replayed_timely_original,
       count(*) FILTER (WHERE outcome = 'replayed' AND arrived_ms > 3000)
           AS replayed_genuinely_late,
       count(*) FILTER (WHERE outcome = 'accepted' AND arrived_ms > 3000)
           AS accepted_genuinely_late
FROM '{DIR}/goldfish_votes.parquet'
WHERE vote_slot >= 1 AND outcome IN ('accepted', 'replayed');

.print
.print == 5b. FFG aggregate propagation: arrival - ffg_due_ms publish time
SELECT count(*) AS n,
       round(quantile_cont(ms - getvariable('ffg_due_ms'), 0.50), 1) AS p50_delta,
       round(quantile_cont(ms - getvariable('ffg_due_ms'), 0.90), 1) AS p90_delta,
       round(quantile_cont(ms - getvariable('ffg_due_ms'), 0.99), 1) AS p99_delta,
       round(max(ms - getvariable('ffg_due_ms')), 1) AS max_delta,
       count(*) FILTER (WHERE ms < getvariable('ffg_due_ms')) AS arrived_before_due
FROM latency WHERE object = 'FFG aggregates';

.print
.print == 6. per-object anomaly scan: the low band, by round position of the slot
.print ==     (slot % 8 = 0 is a round's first slot). Reported factually.
SELECT 'FFG attestations' AS object, att_slot % 8 AS slot_in_round,
       count(*) AS n, round(quantile_cont(arrived_ms, 0.5), 1) AS p50,
       round(min(arrived_ms), 1) AS min, round(max(arrived_ms), 1) AS max
FROM '{DIR}/ffg_votes.parquet' WHERE outcome = 'gossip'
GROUP BY 2 ORDER BY 2;

.print == 6b. FFG attestation arrival histogram, 100 ms buckets, first 12 buckets
SELECT (arrived_ms // 100) * 100 AS bucket_ms, count(*) AS n
FROM '{DIR}/ffg_votes.parquet' WHERE outcome = 'gossip'
GROUP BY 1 ORDER BY 1 LIMIT 12;
