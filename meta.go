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

var WantSuffixes = map[string]bool {
	".log": true,
}

// From memcached.log...
//   2016-04-14T16:10:09.463447-07:00 WARNING Restarting file logging
// From ns_server.babysitter.log...
//   [error_logger:info,2016-04-14T16:10:05.262-07:00,babysitter_of_ns_1@127.0.0.1

var re_memcached =
	regexp.MustCompile(`^(\d\d\d\d)-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)\.(\d\d\d)`)
var re_ns_server =
	regexp.MustCompile(`^\[\w+,(\d\d\d\d)-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)\.(\d\d\d)`)

// ------------------------------------------------------------

type FileMeta struct {
	Skip       bool
	LineFirst  int
	LineSample string
	EntryStart func(string) bool
}

var FileMetaNS = FileMeta{
	LineFirst:  5,
	LineSample: `[ns_server:debug,2016-04-14T16:10:05.262-07:00,babysitter_of_ns_1@127.0.0.1:<0.65.0>:restartable:start_child:98]Started child process <0.66.0>
  MFA: {supervisor_cushion,start_link,
                           [ns_server,5000,infinity,ns_port_server,start_link,
                            [#Fun<ns_child_ports_sup.2.49698737>]]}`,
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
}

// FileMetas is keyed by file name.
var FileMetas = map[string]FileMeta{
	// TODO: "couchbase.log".

	// TODO: "ddocs.log".

	// TODO: "diag.log".

	// SKIP: "ini.log" -- not a log file.

	// TODO: "master_events.log".

	"memcached.log": {
		LineFirst:  5,
		LineSample: "2016-04-14T16:10:09.463447-07:00 WARNING Restarting file logging",
	},

	"ns_server.babysitter.log": FileMetaNS,

	"ns_server.couchdb.log": FileMetaNS,

	// SKIP: "ns_server.debug.log": FileMetaNS, -- too big for now.

	"ns_server.error.log": FileMetaNS,

	"ns_server.fts.log": {
		LineFirst:  5,
		LineSample: "2016-04-14T16:12:13.293-07:00 [INFO] main: /opt/couchbase/bin/cbft started",
		EntryStart: func(line string) bool {
			return len(line) > 0 && unicode.IsDigit(rune(line[0]))
		},
	},

	"ns_server.goxdcr.log": {
		LineFirst:  5,
		LineSample: "ReplicationManager 2016-04-14T16:10:09.652-07:00 [INFO] GOMAXPROCS=4",
	},

	"ns_server.http_access.log": {
		LineFirst:  5,
		LineSample: `172.23.123.146 - Administrator [14/Apr/2016:16:10:19 -0700] "GET /nodes/self HTTP/1.1" 200 1727 - Python-httplib2/$Rev: 259 $`,
	},

	"ns_server.http_access_internal.log": {
		LineFirst:  5,
		LineSample: `127.0.0.1 - @ [14/Apr/2016:16:10:09 -0700] "RPCCONNECT /saslauthd-saslauthd-port HTTP/1.1" 200 0 - Go-http-client/1.1`,
	},

	// TODO: "ns_server.indexer.log".

	"ns_server.info.log": FileMetaNS,

	// TODO: "ns_server.mapreduce_errors.log".

	"ns_server.metakv.log": FileMetaNS,

	"ns_server.ns_couchdb.log": FileMetaNS,

	"ns_server.projector.log": {
		LineFirst:  5,
		LineSample: `2016-04-14T16:10:30.378-07:00 [Info] Parsing the args`,
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
