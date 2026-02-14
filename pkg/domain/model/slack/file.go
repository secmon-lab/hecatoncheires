package slack

import (
	libslack "github.com/slack-go/slack"
)

// File represents a Slack file attachment metadata
type File struct {
	id         string
	name       string
	mimetype   string
	filetype   string
	size       int
	urlPrivate string
	permalink  string
	thumbURL   string
}

// NewFileFromSlack creates a File from a slack-go File struct
func NewFileFromSlack(f libslack.File) File {
	return File{
		id:         f.ID,
		name:       f.Name,
		mimetype:   f.Mimetype,
		filetype:   f.Filetype,
		size:       f.Size,
		urlPrivate: f.URLPrivate,
		permalink:  f.Permalink,
		thumbURL:   bestThumbURL(f),
	}
}

// NewFileFromData creates a File from raw data (for repository reconstruction)
func NewFileFromData(id, name, mimetype, filetype string, size int, urlPrivate, permalink, thumbURL string) File {
	return File{
		id:         id,
		name:       name,
		mimetype:   mimetype,
		filetype:   filetype,
		size:       size,
		urlPrivate: urlPrivate,
		permalink:  permalink,
		thumbURL:   thumbURL,
	}
}

// Getters
func (f File) ID() string         { return f.id }
func (f File) Name() string       { return f.name }
func (f File) Mimetype() string   { return f.mimetype }
func (f File) Filetype() string   { return f.filetype }
func (f File) Size() int          { return f.size }
func (f File) URLPrivate() string { return f.urlPrivate }
func (f File) Permalink() string  { return f.permalink }
func (f File) ThumbURL() string   { return f.thumbURL }

// bestThumbURL selects the best available thumbnail URL from a Slack file.
// It prefers larger thumbnails for better display quality.
func bestThumbURL(f libslack.File) string {
	// Prefer larger thumbnails, fall back to smaller ones
	candidates := []string{
		f.Thumb1024,
		f.Thumb960,
		f.Thumb720,
		f.Thumb480,
		f.Thumb360,
		f.Thumb160,
		f.Thumb80,
		f.Thumb64,
	}

	for _, url := range candidates {
		if url != "" {
			return url
		}
	}
	return ""
}
