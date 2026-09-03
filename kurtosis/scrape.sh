#!/usr/bin/env bash
# Poll every beacon node's /metrics in an enclave and append the lines the
# step-6 measurements need to one file per node.
#
#   kurtosis/scrape.sh <enclave> <outdir> [interval_seconds]
#
# Each sample is prefixed with "# TS <unix_seconds>", so a reader can turn the
# counters into per-slot rates by differencing consecutive samples. Only the
# families the summary uses are kept: a full prysm /metrics is ~200 KB and a
# 200-slot run would be gigabytes of text otherwise.
set -euo pipefail

enclave=$1
outdir=$2
interval=${3:-6}

# p2p_pubsub_undeliverable_total is the only trace of a message gossipsub
# dropped because the subscriber's buffer was full: it never reaches validation
# and never reaches the app, so no other counter moves. It is what a missing
# Goldfish head vote looks like from outside.
keep='^(p2p_pubsub_deliver_total|p2p_pubsub_duplicate_total|p2p_pubsub_undeliverable_total'
keep+='|p2p_pubsub_rpc_recv_pub_total|p2p_pubsub_rpc_recv_pub_bytes_total'
keep+='|p2p_pubsub_rpc_sent_pub_total|gossipsub_topic_msg_sent_bytes'
keep+='|p2p_pubsub_topic_active|p2p_peer_count|p2p_message_received_total'
keep+='|p2p_message_failed_validation_total|goldfish_|beacon_reorgs_total'
keep+='|beacon_head_slot|beacon_clock_time_slot|beacon_finalized_epoch'
keep+='|beacon_current_justified_epoch|data_column_|rpc_data_columns'
# The round-valued FFG family. The checkpoints carry rounds, so the *_round
# gauges are the ones that move once per round; finality_latency_slots is the
# headline (clock slot minus the finalized round's first slot) and the two
# advance counters say how often each checkpoint actually moved.
keep+='|beacon_finalized_round|beacon_current_justified_round'
keep+='|beacon_previous_justified_round|finality_latency_slots'
keep+='|justified_round_advance_total|finalized_round_advance_total'
keep+='|beacon_prev_round_'
keep+='|forkchoice_|process_cpu_seconds_total|process_resident_memory_bytes'
# The blob/column families: a run with blob traffic has to show the columns
# were built, gossip-verified, stored and served, not just that the subnets
# were subscribed.
keep+='|beacon_data_column|beacon_data_availability|beacon_engine_getBlobs'
keep+='|data_columns_recovered|blob_disk_count|blob_recovered_from_el_total)'

mkdir -p "$outdir"

# service name -> host:port for the "metrics" port, beacon nodes only.
kurtosis enclave inspect "$enclave" |
    awk '/^[0-9a-f]{12} +cl-[0-9]+-prysm/ {svc=$2}
         svc && /metrics: 8080\/tcp/ {
             url=$NF; sub(/^http:\/\//, "", url); print svc, url; svc="" }' \
    > "$outdir/targets.txt"

echo "scraping $(wc -l < "$outdir/targets.txt") nodes every ${interval}s"
cat "$outdir/targets.txt"

while true; do
    ts=$(date +%s)
    while read -r svc hostport; do
        {
            echo "# TS $ts"
            curl -sf --max-time 4 "http://$hostport/metrics" |
                grep -E "$keep" |
                grep -Ev '^goldfish_vote_arrival_milliseconds_bucket' || echo "# SCRAPE_FAILED"
        } >> "$outdir/$svc.metrics"
    done < "$outdir/targets.txt"
    sleep "$interval"
done
