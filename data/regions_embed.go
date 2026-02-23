package data

import _ "embed"

// RegionsJSON is the embedded regions source for Riot region choices.
//
//go:embed regions.json
var RegionsJSON []byte
