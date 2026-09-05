#!/usr/bin/env bash
set -euo pipefail

base_url=${ROUND2_ARCHIVE_BASE_URL:-https://pub-6a398d8195fb4cffa21779149b904258.r2.dev}
output_dir=${1:-/tmp/prysm-r2-extra-logs.Rd7MjT}
first=${2:-201}
last=${3:-1000}
parallel=${4:-12}
max_bytes=${ROUND2_ARCHIVE_MAX_BYTES:-5242880}

mkdir -p "$output_dir"
export base_url output_dir max_bytes

seq "$first" "$last" | xargs -P "$parallel" -n 1 bash -c '
  n=$1
  dst=$output_dir/round2-prysm-geth-$n.tar.gz
  if [[ -e $dst ]]; then
    printf "skip-existing %s\n" "$n"
    exit 0
  fi
  url=$base_url/round2-prysm-geth-$n.tar.gz
  size=$(curl --fail --silent --show-error --location --retry 2 --retry-delay 1 \
    --head "$url" | awk "BEGIN{IGNORECASE=1} /^content-length:/ {gsub(/\\r/,\"\",\$2); v=\$2} END{print v}")
  if [[ ! $size =~ ^[0-9]+$ ]]; then
    printf "skip-no-size %s\n" "$n" >&2
    exit 0
  fi
  if (( size > max_bytes )); then
    printf "skip-oversize %s %s\n" "$n" "$size" >&2
    exit 0
  fi
  tmp=$dst.part.$$
  trap "rm -f \"$tmp\"" EXIT
  curl --fail --silent --show-error --location --retry 2 --retry-delay 1 \
    --max-filesize "$max_bytes" --output "$tmp" "$url"
  actual=$(wc -c < "$tmp")
  if (( actual != size )); then
    printf "skip-size-mismatch %s expected=%s actual=%s\n" "$n" "$size" "$actual" >&2
    exit 0
  fi
  mv "$tmp" "$dst"
  trap - EXIT
  printf "downloaded %s %s\n" "$n" "$actual"
' _
