package storage

import "errors"

// ErrNoIVReadings is returned when no IV readings are found for a symbol
var ErrNoIVReadings = errors.New("no IV readings found")
