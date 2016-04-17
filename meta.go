//  Copyright (c) 2016 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the
//  License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing,
//  software distributed under the License is distributed on an "AS
//  IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
//  express or implied. See the License for the specific language
//  governing permissions and limitations under the License.

package main

import (
	"regexp"
	"strings"
	"unicode"
)

var WantSuffixes = map[string]bool{
	".log": true,
}

// From memcached.log...
//   2016-04-14T16:10:09.463447-07:00 WARNING Restarting file logging
//
// From ns_server.fts.log...
//   2016-04-14T17:43:52.164-07:00 [INFO] moss_herder: persistence progess, waiting: 3
//
// From ns_server.babysitter.log...
//   [error_logger:info,2016-04-14T16:10:05.262-07:00,babysitter_of_ns_1@127.0.0.1
//
// From ns_server.goxdcr.log...
//   ReplicationManager 2016-04-14T16:10:09.652-07:00 [INFO] GOMAXPROCS=4
//
// From ns_server.http_access.log...
//   172.23.123.146 - Administrator [14/Apr/2016:16:10:19 -0700] \
//     "GET /nodes/self HTTP/1.1" 200 1727 - Python-httplib2/$Rev: 259 $

var ymd = `(?P<year>\d\d\d\d)-(?P<month>\d\d)-(?P<day>\d\d)`
var hms = `T(?P<HH>\d\d):(?P<MM>\d\d):(?P<SS>\d\d)\.(?P<SSSS>\d+)`

var re_usual = regexp.MustCompile(`^` + ymd + hms + `-\S+\s(?P<level>\S+)\s`)

var re_usual_ex = regexp.MustCompile(`^(?P<module>\w+)\s` + ymd + hms + `-\S+\s(?P<level>\S+)\s`)

var re_ns = regexp.MustCompile(`^\[(?P<module>\w+):(?P<level>\w+),` + ymd + hms + `-[^,]+,`)

// ------------------------------------------------------------

// FileMeta represents metadata about a file that needs to be parsed.
type FileMeta struct {
	Skip       bool
	FirstLine  int // The line number where actual entries start.
	EntryStart func(string) bool
	PrefixRE   *regexp.Regexp
	Cleanser   func([]byte) []byte
}

// ------------------------------------------------------------

var equals_bar_re = regexp.MustCompile(`=======+([^=]+)=======+`)

var equals_bar_replace = []byte(`"$1"`)

// FileMeta represents metadata about a ns-server log file.
var FileMetaNS = FileMeta{
	FirstLine: 5,
	EntryStart: func(line string) bool {
		if len(line) <= 0 ||
			line[0] != '[' {
			return false
		}
		lineParts := strings.Split(line, ",")
		if len(lineParts) < 3 || len(lineParts[1]) <= 0 {
			return false
		}
		return unicode.IsDigit(rune(lineParts[1][0]))
	},
	PrefixRE: re_ns,
	Cleanser: func(s []byte) []byte {
		// Convert `=============PROGRESS REPORT=============`
		// into `"PROGRESS REPORT"`
		return equals_bar_re.ReplaceAll(s, equals_bar_replace)
	},
}

// ------------------------------------------------------------

// FileMetas is keyed by file name and represents metadata for the
// various types of files that we need to parse.
var FileMetas = map[string]FileMeta{
	// Alphabetically...

	// TODO: "couchbase.log".

	// TODO: "ddocs.log".

	// TODO: "diag.log".

	// SKIP: "ini.log" -- not a log file.

	// TODO: "master_events.log".

	"memcached.log": {
		FirstLine: 5,
		PrefixRE:  re_usual,
	},

	"ns_server.babysitter.log": FileMetaNS,

	"ns_server.couchdb.log": FileMetaNS,

	// SKIP: "ns_server.debug.log": FileMetaNS, -- too big for now.

	"ns_server.error.log": FileMetaNS,

	"ns_server.fts.log": {
		FirstLine: 5,
		EntryStart: func(line string) bool {
			return len(line) > 0 && unicode.IsDigit(rune(line[0]))
		},
		PrefixRE: re_usual,
	},

	"ns_server.goxdcr.log": {
		FirstLine: 5,
		PrefixRE:  re_usual_ex,
	},

	"ns_server.http_access.log": {
		Skip:      true,
		FirstLine: 5,
	},

	"ns_server.http_access_internal.log": {
		Skip:      true,
		FirstLine: 5,
	},

	// TODO: "ns_server.indexer.log".

	"ns_server.info.log": FileMetaNS,

	// TODO: "ns_server.mapreduce_errors.log".

	"ns_server.metakv.log": FileMetaNS,

	"ns_server.ns_couchdb.log": FileMetaNS,

	"ns_server.projector.log": {
		FirstLine: 5,
		PrefixRE:  re_usual,
	},

	// TODO: "ns_server.query.log".

	"ns_server.reports.log": FileMetaNS,

	"ns_server.ssl_proxy.log": FileMetaNS,

	"ns_server.stats.log": FileMetaNS,

	// TODO: "ns_server.views.log".

	"ns_server.xdcr.log": FileMetaNS,

	// TODO: "ns_server.xdcr_errors.log".

	// TODO: "ns_server.xdcr_trace.log".

	// TODO: "stats.log".

	// TODO: "stats__archives.json".

	// TODO: "syslog.tar.gz".

	// TODO: "systemd_journal.gz".
}
