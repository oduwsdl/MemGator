package main

import (
	"time"
)

// Response represents a response to a request for momentos
type Response struct {
	// originally requested uri
	OriginalUri string `json:""`
	// request represented as a uri
	Self string `json:"self"`
	// list of returned momentos from different archive services
	Mementos MementoList `json:"mementos"`
	// Object of uris that point to timemap urls in different formats
	TimemapUri TimemapUri `json:"timemap_uri"`
	// Url for timegate
	TimegateUri string `json:"timegate_uri"`
}

// MementoList is a wrapper for presenting a list of momentos
type MementoList struct {
	List []DatetimeUri `json:"list"`
}

// DatetimeUri combines a uri with a datestamp
type DatetimeUri struct {
	Datetime time.Time `json:"datetime"`
	Uri      string    `json:"uri"`
}

// TimemapUri is a list of uris that contains timemap
// uris in different formats
type TimemapUri struct {
	LinkFormat string `json:"link_format"`
	JsonFormat string `json:"json_format"`
	CdxjFormat string `json:"cdxj_format"`
}
