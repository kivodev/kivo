package playback

// Migration (downloads): remove after every client has updated past this release.

// legacyQualities are the download qualities an earlier release offered,
// each by the quality it is now. A download asked for at one, by an
// earlier app or before the update, keeps working: 1080p-high and 1080p
// were 1080p, which High is; 720p and 480p were for saving space, which
// Standard is.
var legacyQualities = map[string]string{"1080p-high": "high", "1080p": "high", "720p": "standard", "480p": "standard"}

// Migration (downloads): remove after every client has updated past this release.

// currentQualityName is a download quality's name as this release knows
// it.
func currentQualityName(name string) string {
	if current, ok := legacyQualities[name]; ok {
		return current
	}
	return name
}

// Migration (downloads): remove after every client has updated past this release.

// currentPlan moves a stored plan asked for at an earlier release's quality
// to the quality it is now. A download that isn't ready yet is decided
// again at that quality. A ready one keeps the decision its files were made
// by.
func currentPlan(p plan, ready bool) plan {
	name := p.Input.Quality.Name
	if _, ok := legacyQualities[name]; !ok {
		return p
	}
	q, _ := DownloadQualityNamed(name)
	if ready {
		p.Input.Quality.Name = q.Name
		return p
	}
	p.Input.Quality = q
	p.Decision = Decide(p.Input)
	return p
}
