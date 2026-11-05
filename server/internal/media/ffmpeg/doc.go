// Package ffmpeg carries out media's requests by running ffmpeg and ffprobe
// as child processes, one per call or job, so a crash or hang costs only
// one job. Every call except Transcode and the accelerator test runs at low
// OS priority, below playback. So a play waiting on such a call waits for
// the job, never for the CPU or disk it shares with playback.
//
// Requests become command lines here and nowhere else. Each is built from a
// fixed sequence of steps that append to argv: the input, then each
// output's streams, then the muxer. The same request with the same
// accelerator test results always gives the same argv.
package ffmpeg
