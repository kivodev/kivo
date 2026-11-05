package ffmpeg

// escapedName is a file name that needs escaping at both levels of a
// filter graph. Windows names can't hold ':' or '\', but the drive and the
// folders around the name supply both.
const escapedName = `Don't panic [part 1], a;b.srt`
