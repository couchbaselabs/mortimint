mortimint - fresh breath for couchbase postmortems

mortimint flattens cbcollectinfo logs into a grep-friendlier format.

Especially, mortimint produces output with date/time-stamps on each line.

= Building & installing

To build...

    $ go get github.com/couchbaselabs/mortimint

After that, mortimint should be built and installed into your go bin
directory.

= Usage

Usage example...

    $ cd myTempDir
    
    $ curl ...                          # Download cbcollect-info zip's.
    
    $ unzip *.zip                       # Unzip them.
    
    $ mortimint * | grep curr_items     # Grep away.

The output should have date/time-stamps, so you can use more of your
favorite cmd-line tools for more analysis and correlations.

For example, pipe output to sort to have all events from multiple log
files ordered by date/time...

    $ mortimint ~/tmp/CBSE-1313/cbcollect* | grep "  2016" | sort

mortimint emits to stdout

1) the original log entry

2) and the expanded, flattend log entries prefixed by two spaces.

As mortimint parses log entries, it makes heuristic guesses when it
sees data that look like NAME=VALUE pairs, and the types of those
values (STRING's or INT's).

    2016-04-25T01:01:11.1111 latest terms [
         {foo,11},
         {bar,{baz,222}}]
    2016-04-25T01:01:22.2222 config for vb 22 was {"state":"active","rsets":44,
         {"flog":{"count":555}}}

mortimint will emit to stdout roughly something like...

    2016-04-25T01:01:11.1111 latest terms [
         {foo,11},
         {bar,{baz,222}}]
      2016-0425T01:01:11.111 cbcollect-172.22.12.10/ns_server_diag.log [latest terms] foo = INT 11
      2016-0425T01:01:11.111 cbcollect-172.22.12.10/ns_server_diag.log [latest terms bar] baz = INT 222
    2016-04-25T01:01:22.2222 config for vb 22 was {"state":"active","rsets":44,
         {"flog":{"count":555}}}
      2016-04-25T01:01:22.2222 cbcollect-172.22.12.10/ns_server_diag.log [config for] vb = INT 22
      2016-04-25T01:01:22.2222 cbcollect-172.22.12.10/ns_server_diag.log [was] state = STRING "active"
      2016-04-25T01:01:22.2222 cbcollect-172.22.12.10/ns_server_diag.log [was] rsets = INT 44
      2016-04-25T01:01:22.2222 cbcollect-172.22.12.10/ns_server_diag.log [was flog] count = INT 555

The "path" that mortimint emits in the brackets ('[' and ']') is the
rough path down the parse tree to reach the leaf name=value
information.

For example, you can grep the output for " 2016" and for "INT" to
filter for numeric data.

NOTE: output format might change!  And, cmd-line params/flags might
change.