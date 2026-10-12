#!/usr/bin/env bash
set -euo pipefail
output=${1:-/tmp/ape-smoke.mkv}
subtitle=$(mktemp --suffix=.srt)
trap 'unlink "$subtitle"' EXIT
cat > "$subtitle" <<'SUB'
1
00:00:00,200 --> 00:00:03,000
Ape Player GPU subtitle test

2
00:00:04,000 --> 00:00:12,000
第二条字幕 / second subtitle
SUB
ffmpeg -hide_banner -loglevel error \
 -f lavfi -i testsrc2=size=1920x1080:rate=30 \
 -f lavfi -i sine=frequency=440 -f lavfi -i sine=frequency=660 \
 -i "$subtitle" -t 14 -map 0:v -map 1:a -map 2:a -map 3:s \
 -c:v libx264 -preset ultrafast -crf 27 -c:a aac -c:s srt \
 -metadata:s:a:0 language=eng -metadata:s:a:1 language=jpn \
 -metadata:s:s:0 language=zho "$output"
