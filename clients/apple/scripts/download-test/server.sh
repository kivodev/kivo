#!/bin/zsh
# The local server the UI tests run against (FIRST-RUN.md, The download
# test): Kivo built from this checkout on 127.0.0.1:18484
# ($KIVO_TEST_SERVER_PORT; KIVO_TEST_LISTEN=0.0.0.0 listens on every
# address, which it needs to advertise itself by Bonjour), behind proxy.go
# on :18490 ($KIVO_TEST_PORT), with
# generated media: a three-minute 1080p MKV movie with an English subtitle,
# a two-minute movie with five audio and eighteen subtitle tracks,
# a ten-minute book in five chapters in a series, a show of three
# twenty-second MP4 episodes over two seasons, twelve two-minute home
# videos, eight of them in two folders, and a fifteen-minute home video
# with 5.1 AC-3 audio and an English subtitle. "stop" stops both. The media
# is made again whenever this script changes.
set -u
here=${0:A:h}
root=${KIVO_DOWNLOAD_TEST_DIR:-$HOME/kivo-download-test}
port=${KIVO_TEST_SERVER_PORT:-18484}
proxy_port=${KIVO_TEST_PORT:-18490}
stop() {
  [[ -f $root/proxy.pid ]] && kill $(<$root/proxy.pid) 2>/dev/null
  [[ -f $root/kivo.pid ]] && kill $(<$root/kivo.pid) 2>/dev/null
  rm -f $root/proxy.pid $root/kivo.pid
}
stop
[[ ${1:-} == stop ]] && exit 0

rm -rf $root/data $root/cache $root/backups
[[ $root/media/.made -nt ${0:A} ]] || rm -rf $root/media
media=$root/media
mkdir -p $media
printf '1\n00:00:01,000 --> 00:00:30,000\nHello from the subtitle\n' > $root/sub.srt

movie="$media/movies/Big Test (2020)/Big Test (2020).mkv"
if [[ ! -f $movie ]]; then
  mkdir -p ${movie:h}
  ffmpeg -nostdin -loglevel error -f lavfi -i testsrc2=size=1920x1080:rate=24 -f lavfi -i sine=frequency=440:sample_rate=48000 \
    -i $root/sub.srt -map 0 -map 1 -map 2 -t 180 -c:v libx264 -preset ultrafast -b:v 10M -g 48 -pix_fmt yuv420p \
    -c:a aac -b:a 128k -c:s srt -metadata:s:s:0 language=eng $movie || exit 1
fi

# A film with many tracks, as a Blu-ray rip has them: five English audio
# tracks titled by the release, two of them commentaries, and eighteen
# subtitle tracks, SDH among them, two Chinese told apart only by their
# titles.
movie="$media/movies/Many Tracks (1990)/Many Tracks (1990).mkv"
if [[ ! -f $movie ]]; then
  mkdir -p ${movie:h}
  audio=(
    'eac3 6 E-AC3 DD+5.1 1024kbs'
    'eac3 4 E-AC3 DD+4.0 1024kbs Surround Mix'
    'flac 2 Flac 2.0 Original Stereo Audio 627 kb/s'
    'ac3 2 Commentary by director Ron Underwood, and writers/producers Brent Maddock and S.S. Wilson'
    'ac3 2 Commentary by Jonathan Melville (author of Seeking Perfection: The Unofficial Guide to Tremors)'
  )
  subtitles=(
    'eng Full .srt' 'eng SDH .srt' jpn fre ita ger 'spa Castilian' dut 'chi Simplified'
    dan fin gre nor ice kor 'por Portuguese (Portugal)' swe 'chi Traditional'
  )
  args=(-f lavfi -i testsrc2=size=1920x800:rate=24)
  maps=(-map 0)
  for i in {1..${#audio}}; do
    args+=(-f lavfi -i sine=frequency=$(( 200 + i * 60 )):sample_rate=48000)
    maps+=(-map $i)
  done
  for i in {1..${#subtitles}}; do
    args+=(-i $root/sub.srt)
    maps+=(-map $(( ${#audio} + i )))
  done
  codecs=(-c:v libx264 -preset ultrafast -b:v 2M -g 48 -pix_fmt yuv420p)
  for i in {1..${#audio}}; do
    words=(${=audio[$i]})
    codecs+=(-c:a:$(( i - 1 )) ${words[1]} -ac:a:$(( i - 1 )) ${words[2]} -metadata:s:a:$(( i - 1 )) language=eng
      -metadata:s:a:$(( i - 1 )) "title=${words[3,-1]}")
  done
  codecs+=(-disposition:a:0 default)
  for i in {1..${#subtitles}}; do
    words=(${=subtitles[$i]})
    codecs+=(-metadata:s:s:$(( i - 1 )) language=${words[1]})
    (( ${#words} > 1 )) && codecs+=(-metadata:s:s:$(( i - 1 )) "title=${words[2,-1]}")
  done
  codecs+=(-c:s srt -disposition:s:0 0 -disposition:s:1 hearing_impaired)
  ffmpeg -nostdin -loglevel error $args $maps -t 120 $codecs $movie || exit 1
fi

# Author/Series/Title, so the library has an author and a series.
book="$media/books/Ann Author/Test Series/Test Book/Test Book.m4b"
if [[ ! -f $book ]]; then
  mkdir -p ${book:h}
  {
    print ';FFMETADATA1'
    print 'title=Test Book'
    print 'artist=Ann Author'
    for n in {1..5}; do
      print "[CHAPTER]\nTIMEBASE=1/1000\nSTART=$(( (n - 1) * 120000 ))\nEND=$(( n * 120000 ))\ntitle=Chapter $n"
    done
  } > $root/chapters.txt
  ffmpeg -nostdin -loglevel error -f lavfi -i sine=frequency=330:sample_rate=44100 -i $root/chapters.txt \
    -map 0 -map_metadata 1 -map_chapters 1 -t 600 -c:a aac -b:a 64k $book || exit 1
fi

for code in S01E01 S01E02 S02E01; do
  episode="$media/shows/Test Show/Season ${code[2,3]}/Test Show - $code.mp4"
  mkdir -p ${episode:h}
  if [[ ! -f $episode ]]; then
    ffmpeg -nostdin -loglevel error -f lavfi -i testsrc2=size=640x360:rate=24 -f lavfi -i sine=frequency=550:sample_rate=48000 \
      -t 20 -c:v libx264 -preset ultrafast -b:v 1M -pix_fmt yuv420p -c:a aac -b:a 96k -movflags +faststart $episode || exit 1
  fi
done

# Titles across the alphabet for the letter scrubber; a folder of a videos
# library is a collection.
tone=200
for name in 'Apple Orchard' 'Dinner Party' 'Garden Tour' 'Kite Festival' \
  'Holidays/Beach Day' 'Holidays/Evening Swim' 'Holidays/Lake House' 'Holidays/Snow Day' \
  'Trips/Canyon Hike' 'Trips/Harbor Lights' 'Trips/Mountain Pass' 'Trips/River Walk'; do
  video="$media/videos/$name.mkv"
  (( tone += 40 ))
  if [[ ! -f $video ]]; then
    mkdir -p ${video:h}
    ffmpeg -nostdin -loglevel error -f lavfi -i testsrc2=size=426x240:rate=24 -f lavfi -i sine=frequency=$tone:sample_rate=48000 \
      -i $root/sub.srt -map 0 -map 1 -map 2 -t 120 -c:v libx264 -preset ultrafast -b:v 300k -pix_fmt yuv420p \
      -c:a aac -b:a 64k -c:s srt -metadata:s:s:0 language=eng $video || exit 1
  fi
done
# Long enough to resume well into it, with surround audio a phone's
# speaker takes as stereo, so its audio is encoded.
video="$media/videos/Night Shift.mkv"
if [[ ! -f $video ]]; then
  ffmpeg -nostdin -loglevel error -f lavfi -i testsrc2=size=1280x720:rate=24 -f lavfi -i sine=frequency=300:sample_rate=48000 \
    -i $root/sub.srt -map 0 -map 1 -map 2 -t 900 -c:v libx264 -preset ultrafast -b:v 1M -g 48 -pix_fmt yuv420p \
    -c:a ac3 -ac 6 -b:a 384k -c:s srt -metadata:s:s:0 language=eng $video || exit 1
fi
touch $media/.made

(cd $here/../../../../server && go build -o $root/kivo ./cmd/kivo) || exit 1
(cd $here && go build -o $root/proxy proxy.go) || exit 1
cd $root
KIVO_LISTEN=${KIVO_TEST_LISTEN:-127.0.0.1}:$port KIVO_DATA_DIR=$root/data KIVO_CACHE_DIR=$root/cache KIVO_BACKUP_DIR=$root/backups \
  KIVO_FFMPEG=$(command -v ffmpeg) KIVO_FFPROBE=$(command -v ffprobe) KIVO_AUDIBLE=false \
  nohup ./kivo -config /dev/null > $root/kivo.log 2>&1 &!
echo $! > $root/kivo.pid
nohup ./proxy -listen :$proxy_port -backend http://127.0.0.1:$port -pidfile $root/kivo.pid > $root/proxy.log 2>&1 &!
echo $! > $root/proxy.pid

for i in {1..50}; do curl -sf http://127.0.0.1:$port/healthz > /dev/null && break; sleep 0.2; done
api=http://127.0.0.1:$port/api/v1
post() {
  curl -sf -c $root/jar -b $root/jar -H "Origin: http://127.0.0.1:$port" -H 'Content-Type: application/json' -d $2 $api/$1 > /dev/null
}
post setup '{"username":"test","password":"download test"}' || exit 1
post libraries '{"name":"Movies","kind":"movies","folders":["'$media/movies'"]}' || exit 1
post libraries '{"name":"Audiobooks","kind":"audiobooks","folders":["'$media/books'"]}' || exit 1
post libraries '{"name":"Shows","kind":"shows","folders":["'$media/shows'"]}' || exit 1
post libraries '{"name":"Home Videos","kind":"videos","folders":["'$media/videos'"]}' || exit 1
echo "Kivo on http://127.0.0.1:$proxy_port (user test, password \"download test\"); logs in $root"
