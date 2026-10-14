package supervisor

import "phlo/internal/paths"

// SetLayout points the supervisor at moved projects or database folders.
func (s *Supervisor) SetLayout(layout paths.Layout) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Layout = layout
}
