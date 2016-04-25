mortimint - fresh breath for couchbase postmortems

mortimint flattens cbcollect-info logs into a grep-friendlier format.

Many cbcollect-info log files include tree-like log entries that span
multiple lines, such as erlang terms and JSON.  mortimint will flatten
those tree-like entries into multiple output lines, where every
emitted line will include a date/time-stamp.

= Building & installing

Prerequisites: go

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

The stdout of mortimint will have date/time-stamps on every line, so
you can use more of your favorite cmd-line tools for more analysis and
correlations.

For example, pipe output to sort to have all events from multiple log
files ordered by date/time...

    $ mortimint ~/tmp/CBSE-1313/cbcollect* | sort

As mortimint parses log entries, it makes heuristic guesses on how to
parse tree-like entries and when it encounters log entries that look
like NAME=VALUE pairs.  The mortimint tool also makes heuristic
guesses as to the types of those VALUE's (STRING's or INT's).

As an example, if the cbcollect-info log entries looked like...

    2016-04-25T01:01:11.1111 latest terms [
         {foo,11},
         {bar,{baz,222}}]
    2016-04-25T01:01:22.2222 config for vb 22 was {"state":"active","rsets":44,
         {"flog":{"count":555}}}

Then, mortimint will emit to stdout roughly something like...

      2016-0425T01:01:11.111 cbcollect-172.22.12.10/ns_server_diag.log:100:100 [latest terms] foo = INT 11
      2016-0425T01:01:11.111 cbcollect-172.22.12.10/ns_server_diag.log:100:100 [latest terms bar] baz = INT 222
      2016-04-25T01:01:22.2222 cbcollect-172.22.12.10/ns_server_diag.log:140:103 [config for] vb = INT 22
      2016-04-25T01:01:22.2222 cbcollect-172.22.12.10/ns_server_diag.log:140:103 [was] state = STRING "active"
      2016-04-25T01:01:22.2222 cbcollect-172.22.12.10/ns_server_diag.log:140:103 [was] rsets = INT 44
      2016-04-25T01:01:22.2222 cbcollect-172.22.12.10/ns_server_diag.log:140:103 [was flog] count = INT 555

The part after the timestamp is
"$DIR/$FILENAME:$BYTE_OFFSET:$LINE_NUM", where BYTE_OFFSET is the
offset into file of the byte that starts the log entry.

The "path" that mortimint emits in the brackets ('[' and ']') is the
rough path down the parse tree to reach the specific, leaf name=value
information.

For example, you can grep the output for "INT" to filter for numeric
data.

NOTE: output format might change!  And, cmd-line params/flags might
change.
