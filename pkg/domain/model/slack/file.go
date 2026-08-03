package slack

// File represents a Slack file attachment metadata.
//
// Construction from a slack-go File lives in pkg/service/slack
// (FileFromSlack), keeping the Slack wire format out of the domain layer.
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
