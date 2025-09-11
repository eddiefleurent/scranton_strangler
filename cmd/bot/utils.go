package main

// shortID returns a truncated ID string, safely handling IDs shorter than 8 characters
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}