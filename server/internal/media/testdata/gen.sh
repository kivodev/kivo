#!/bin/sh
# Regenerates the tiny fixtures the media tests use without ffmpeg installed.
# Run from anywhere with ffmpeg and ffprobe on PATH; commit the results.
# ffmpeg's output differs across versions, so regenerate only when a fixture
# has to change, and keep everything a few kilobytes.
set -eu
cd "$(dirname "$0")"

ff="ffmpeg -hide_banner -loglevel error -y"
video="testsrc2=size=64x64:rate=10:duration=8"
audio="sine=frequency=440:sample_rate=8000:duration=8"
keys="0,1.6,2.4,4,6.2"

cat > chapters.txt <<'EOF'
;FFMETADATA1
[CHAPTER]
TIMEBASE=1/1000
START=0
END=2400
title=Intro
[CHAPTER]
TIMEBASE=1/1000
START=2400
END=8000
title=Main
EOF

# H.264 with B-frames, so presentation and decode times differ, and irregular
# keyframes, so segment boundaries are visible.
$ff -f lavfi -i "$video" -f lavfi -i "$audio" -i chapters.txt -map 0:v -map 1:a -map_chapters 2 \
	-c:v libx264 -preset ultrafast -crf 45 -bf 2 -x264-params scenecut=0:keyint=100 -force_key_frames "$keys" \
	-c:a aac -ac 1 -b:a 8k h264.mkv
$ff -i h264.mkv -map 0 -c copy -movflags +faststart h264.mp4
$ff -i h264.mkv -map 0 -c copy -movflags +frag_keyframe+empty_moov+default_base_moof h264-frag.mp4
$ff -f lavfi -i "$video" -f lavfi -i "$audio" -map 0:v -map 1:a \
	-c:v libx265 -preset ultrafast -crf 45 -x265-params log-level=error:scenecut=0:keyint=100 -force_key_frames "$keys" \
	-c:a aac -ac 1 -b:a 8k hevc.mkv
rm chapters.txt

# h264.mkv with three attachments: a font, an image and a text file, each
# declared with a media type.
printf '\000\001\000\000\000\012OS/2fake-font-bytes-for-a-test' > font.ttf
printf '\211PNG\r\n\032\n\000\000\000\015IHDR\000\000\000\001\000\000\000\001\010\006\000\000\000\037\025\304\211' > cover.png
printf 'not a font\n' > notes.txt
$ff -nostdin -i h264.mkv -map 0 -c copy -attach font.ttf -metadata:s:t:0 mimetype=application/x-truetype-font \
	-attach cover.png -metadata:s:t:1 mimetype=image/png -attach notes.txt -metadata:s:t:2 mimetype=text/plain attachments.mkv
rm font.ttf cover.png notes.txt

# What ffprobe calls keyframes, for the container readers to match.
for f in h264.mkv h264.mp4 h264-frag.mp4 hevc.mkv; do
	ffprobe -v error -select_streams v:0 -show_entries packet=pts_time,flags -of csv=p=0 "$f" |
		awk -F, '$2 ~ /K/ {print $1}' | sort -n > "$f.keyframes"
done

ffprobe -v error -print_format json -show_format -show_streams -show_chapters h264.mkv > h264.mkv.probe.json

# A remux exactly as the transcode argv writes it, for the fake ffmpeg to
# replay: 2-second segments of h264.mkv from 2.4 s, and hevc.mkv from zero.
remux="-copyts -map 0:v:0 -map 0:a:0 -c copy -map_metadata -1 -map_chapters -1 -max_muxing_queue_size 2048
	-avoid_negative_ts disabled -output_ts_offset 10 -use_editlist 0
	-movflags +frag_keyframe+empty_moov+default_base_moof+frag_discont -f mp4"
$ff -ss 2.4 -i h264.mkv $remux remux-h264.mp4
$ff -i hevc.mkv $remux remux-hevc.mp4
# The same seek with the audio encoded to E-AC-3, which takes delay_moov.
# ffmpeg's seek lands on the keyframe at 1.6 s, and delay_moov runs its
# first fragment on past the one at 2.4 s, to the one at 4 s.
$ff -ss 2.4 -copyts -i h264.mkv -map 0:v:0 -c:v copy -map 0:a:0 -c:a eac3 -b:a 32k -map_metadata -1 -map_chapters -1 \
	-max_muxing_queue_size 2048 -avoid_negative_ts disabled -output_ts_offset 10 -use_editlist 0 \
	-movflags +frag_keyframe+empty_moov+delay_moov+default_base_moof+frag_discont -f mp4 remux-merged.mp4

# What a hardware-test encode writes, for the fake ffmpeg to replay: a
# 5-second clip from 1 s, with keyframes on a 1-second grid, and with
# keyframes every 2 seconds, as an encoder ignoring forced keyframes leaves
# them.
$ff -f lavfi -i "testsrc2=size=64x64:rate=24:duration=5" -c:v libx264 -preset ultrafast -crf 45 -g 1000 hwtest.mkv
mux="-map_metadata -1 -map_chapters -1 -avoid_negative_ts disabled -output_ts_offset 10 -use_editlist 0
	-movflags +frag_keyframe+empty_moov+default_base_moof+frag_discont -f mp4"
x264="-c:v libx264 -preset ultrafast -crf 45 -sc_threshold 0 -g 48"
$ff -ss 1 -copyts -i hwtest.mkv $x264 -force_key_frames "expr:gte(t,n_forced*1)" $mux hw-grid.mp4
$ff -ss 1 -copyts -i hwtest.mkv $x264 $mux hw-offgrid.mp4
rm hwtest.mkv

# Error output to classify, named for the error it should map to. busy-*.txt
# and failed-*.txt are kept by hand from logs of hardware encoders, which
# these tests don't need.
mkdir -p stderr
probe="ffprobe -hide_banner -loglevel error -protocol_whitelist file -i"
log="ffmpeg -hide_banner -nostats -loglevel warning -protocol_whitelist file"
$probe file:missing.mkv 2> stderr/unreadable-missing.txt || true
head -c 2000 h264.mkv > unreadable.mkv && chmod 000 unreadable.mkv
$probe file:unreadable.mkv 2> stderr/unreadable-permission.txt || true
rm -f unreadable.mkv
head -c 3000 /dev/urandom > garbage.bin
$probe file:garbage.bin 2> stderr/corrupt-garbage.txt || true
rm garbage.bin
head -c 1000 h264.mp4 > truncated.mp4
$probe file:truncated.mp4 2> stderr/corrupt-truncated.txt || true
rm truncated.mp4
$log -i file:h264.mkv -map 0:7 -c copy -f mp4 pipe:1 2> stderr/unsupported-no-stream.txt > /dev/null || true
# A plain ffmpeg's answer to one of jellyfin-ffmpeg's filters.
$log -f lavfi -i testsrc2=duration=1 -filter_complex '[0:0]tonemapx=format=yuv420p[v]' -map '[v]' -f null - 2> stderr/unsupported-no-filter.txt || true
