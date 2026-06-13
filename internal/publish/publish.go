package publish

import "time"

// Fragment is what may be syndicated: a title, a summary, a canonical link, and
// optionally a hero image for the link card. It has NO body field by design —
// the asset/fragment line (spec 06 §1) is enforced by this type, so a post body
// cannot reach a Publisher even by mistake.
type Fragment struct {
	Title        string
	Summary      string // canonical description, or a per-channel override
	CanonicalURL string // link back to the owned site (always included)
	ImageURL     string // optional hero image for the card
}

// Capabilities declares a channel's constraints so the common layer can adapt a
// Fragment before sending (A4).
type Capabilities struct {
	MaxChars int  // max post text length in runes (0 = unlimited)
	Editable bool // whether a published post can later be edited
	LinkCard bool // whether the channel renders a card from the canonical URL
}

// Preview is the side-effect-free result of Plan: exactly what Execute will
// post. It is the content shown at the confirmation gate (A4 / screen B-3).
type Preview struct {
	Target string
	Text   string   // the exact text that will be posted
	Frag   Fragment // fragment used to build the link card
	Notes  []string // adaptation notes (e.g. trimming) to show the user
}

// Result is the outcome of a successful Execute.
type Result struct {
	Target      string
	PostID      string // stable channel id (e.g. an AT URI)
	URL         string // public URL of the post
	PublishedAt time.Time
}

// Publisher syndicates a Fragment to one channel (A4). Implementations receive
// only the Fragment — never the post body (spec 06 §1). Plan has no side
// effects; Execute performs the post after the user confirms the Preview.
type Publisher interface {
	Name() string
	Capabilities() Capabilities
	Plan(Fragment) (Preview, error)
	Execute(Preview) (Result, error)
}
