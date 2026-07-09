package project

import (
	"github.com/ShiroDoromoto/crofty/internal/access"
)

// PreflightInit finds every permission wall between crofty and a new project at
// dir, before init writes a byte or asks the author a question. It returns nil
// when there is nothing to ask for, one *access.Denied, or access.Denials when
// crofty would be stopped twice — so the author grants everything in one pass
// instead of clearing one wall only to meet the next (D-1).
//
// The site folder is the wall that stops init. crofty's own state is not: a site
// it cannot register is still a site, and init says so after writing it (#13).
// So the state is only reported here when init is stopping anyway — asking for a
// permission crofty is about to prove it does not need would be dishonest, and
// the place the author reads about it is the end of a successful init.
func PreflightInit(dir string) error {
	err := EnsureCreatable(dir)
	if err == nil {
		return nil
	}
	site, ok := access.From(err)
	if !ok {
		return err // not a wall: a file in the way, a full disk
	}

	walls := access.Denials{site}
	if state, ok := access.From(EnsureStateWritable()); ok {
		walls = append(walls, state)
	}
	if len(walls) == 1 {
		return walls[0]
	}
	return walls
}
