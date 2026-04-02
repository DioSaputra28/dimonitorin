package staticassets

import "embed"

//go:embed css/* js/*
var FS embed.FS
