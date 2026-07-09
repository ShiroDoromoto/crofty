package project

// StateStatus is where crofty keeps its own state and whether it may write
// there. Nothing else answers that question without writing: the only way to
// learn that the registry is behind a wall was to run init and read the warning
// it prints afterwards. An agent wants to know before it acts (D-1).
//
// A wall here is not a failure. The state directory holds the project registry,
// which powers discovery and nothing else, so Err is a fact to report — never a
// reason for the command asking to fail (#13).
type StateStatus struct {
	Dir     string // crofty's per-user state directory
	FromEnv bool   // CROFTY_HOME chose Dir, rather than the OS config dir
	Err     error  // nil when crofty may write there; an *access.Denied on a permission wall
}

// Writable reports whether crofty may write its state directory.
func (s StateStatus) Writable() bool { return s.Err == nil }

// State answers where crofty's state lives and whether it may write there,
// probing the filesystem the same way EnsureStateWritable does — an ACL, a
// read-only mount and a full disk all refuse writes the permission bits allow.
func State() (StateStatus, error) {
	dir, err := GlobalDir()
	if err != nil {
		return StateStatus{}, err
	}
	return StateStatus{Dir: dir, FromEnv: envHome() != "", Err: EnsureStateWritable()}, nil
}
